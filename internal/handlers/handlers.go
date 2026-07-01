package handlers

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"finding.vendor.com/db"
	"finding.vendor.com/internal/config"
	"finding.vendor.com/internal/events"
	"finding.vendor.com/internal/models"
	"finding.vendor.com/routing"
)

type Handler struct {
	pg   *db.Postgres
	rl   *db.RedisLoad
	prod *events.Producer
	cfg  config.Config
}

func New(pg *db.Postgres, rl *db.RedisLoad, prod *events.Producer, cfg config.Config) *Handler {
	return &Handler{pg: pg, rl: rl, prod: prod, cfg: cfg}
}

func (h *Handler) Register(r *gin.Engine) {
	r.GET("/health", h.Health)
	r.POST("/order", h.CreateOrder)
	r.GET("/vendors", h.ListVendors)
	r.POST("/vendors/:id/zone", h.UpdateZone)
}

func (h *Handler) Health(c *gin.Context) {
	redisOK := h.rl.Ping(c.Request.Context()) == nil
	c.JSON(http.StatusOK, gin.H{
		"status":    "ok",
		"redis":     redisOK,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

func (h *Handler) CreateOrder(c *gin.Context) {
	var req models.OrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:  "invalid request body",
			Code:   "INVALID_INPUT",
			Detail: err.Error(),
		})
		return
	}

	lat, lon, cart := *req.CustomerLat, *req.CustomerLon, *req.CartTotal
	if lat < -90 || lat > 90 || lon < -180 || lon > 180 {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: "coordinates out of range",
			Code:  "INVALID_COORDINATES",
		})
		return
	}
	if cart < 0 {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: "cart_total must be non-negative",
			Code:  "INVALID_CART_TOTAL",
		})
		return
	}

	ctx := c.Request.Context()

	eligibles, err := h.pg.EligibleVendors(ctx, lat, lon)
	if err != nil {
		log.Printf("eligibility query failed: %v", err)
		c.JSON(http.StatusServiceUnavailable, models.ErrorResponse{
			Error: "routing backend unavailable",
			Code:  "ROUTING_BACKEND_UNAVAILABLE",
		})
		return
	}
	if len(eligibles) == 0 {
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Error: "no vendor serves this location",
			Code:  "NO_ELIGIBLE_VENDOR",
		})
		return
	}
	loads := make(map[int]int, len(eligibles))
	degraded := false
	for _, v := range eligibles {
		l, deg := h.rl.GetLoad(ctx, v.ID)
		if deg {
			l = v.DBLoad
		}
		loads[v.ID] = l
		degraded = degraded || deg
	}

	chosen := routing.ChooseVendor(eligibles, loads)
	newLoad, deg := h.rl.IncrLoad(ctx, chosen.ID)
	if deg {
		degraded = true
	} else {
		loads[chosen.ID] = newLoad
	}

	applied, amount, final := routing.ApplyDiscount(cart, h.cfg.DiscountThreshold, h.cfg.DiscountRate)

	orderID := uuid.NewString()
	routedAt := time.Now().UTC()

	ev := events.OrderRoutedEvent{
		EventID:         uuid.NewString(),
		OrderID:         orderID,
		VendorID:        chosen.ID,
		VendorName:      chosen.Name,
		CustomerLat:     lat,
		CustomerLon:     lon,
		CartTotal:       cart,
		DiscountApplied: applied,
		DiscountAmount:  amount,
		FinalTotal:      final,
		VendorLoad:      loads[chosen.ID],
		EligibleCount:   len(eligibles),
		DegradedMode:    degraded,
		RoutedAt:        routedAt,
		SchemaVersion:   1,
	}
	if err := h.prod.PublishOrderRouted(ctx, ev); err != nil {
		log.Printf("kafka publish failed for order %s: %v", orderID, err)
	}

	c.JSON(http.StatusOK, models.OrderResponse{
		OrderID:         orderID,
		VendorID:        chosen.ID,
		VendorName:      chosen.Name,
		CartTotal:       cart,
		DiscountApplied: applied,
		DiscountAmount:  amount,
		FinalTotal:      final,
		VendorLoad:      loads[chosen.ID],
		EligibleCount:   len(eligibles),
		DegradedMode:    degraded,
		RoutedAt:        routedAt.Format(time.RFC3339),
	})
}

func (h *Handler) ListVendors(c *gin.Context) {
	ctx := c.Request.Context()
	rows, err := h.pg.ListVendors(ctx)
	if err != nil {
		log.Printf("list vendors failed: %v", err)
		c.JSON(http.StatusServiceUnavailable, models.ErrorResponse{
			Error: "could not load vendors",
			Code:  "ROUTING_BACKEND_UNAVAILABLE",
		})
		return
	}

	out := make([]models.Vendor, 0, len(rows))
	for _, r := range rows {
		l, _ := h.rl.GetLoad(ctx, r.ID)
		out = append(out, models.Vendor{
			ID:          r.ID,
			Name:        r.Name,
			ServiceZone: json.RawMessage(r.ZoneJSON),
			CurrentLoad: l,
			BaseLoad:    r.BaseLoad,
		})
	}
	c.JSON(http.StatusOK, gin.H{"vendors": out})
}

func (h *Handler) UpdateZone(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: "vendor id must be an integer",
			Code:  "INVALID_VENDOR_ID",
		})
		return
	}

	var body struct {
		Zone json.RawMessage `json:"zone"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || len(body.Zone) == 0 {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: "body must be {\"zone\": <GeoJSON geometry>}",
			Code:  "INVALID_INPUT",
		})
		return
	}

	row, err := h.pg.UpdateZone(c.Request.Context(), id, body.Zone)
	if errors.Is(err, db.ErrNoRows) {
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Error: "vendor not found",
			Code:  "VENDOR_NOT_FOUND",
		})
		return
	}
	if err != nil {
		// Most commonly: malformed GeoJSON rejected by ST_GeomFromGeoJSON.
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:  "could not update zone (invalid geometry?)",
			Code:   "INVALID_GEOMETRY",
			Detail: err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, models.Vendor{
		ID:          row.ID,
		Name:        row.Name,
		ServiceZone: json.RawMessage(row.ZoneJSON),
		BaseLoad:    row.BaseLoad,
	})
}
