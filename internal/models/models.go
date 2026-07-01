package models

import "encoding/json"

type OrderRequest struct {
	CustomerLat *float64 `json:"customer_lat" binding:"required"`
	CustomerLon *float64 `json:"customer_lon" binding:"required"`
	CartTotal   *float64 `json:"cart_total"   binding:"required"`
}

type OrderResponse struct {
	OrderID         string  `json:"order_id"`
	VendorID        int     `json:"vendor_id"`
	VendorName      string  `json:"vendor_name"`
	CartTotal       float64 `json:"cart_total"`
	DiscountApplied bool    `json:"discount_applied"`
	DiscountAmount  float64 `json:"discount_amount"`
	FinalTotal      float64 `json:"final_total"`
	VendorLoad      int     `json:"vendor_load_at_routing"`
	EligibleCount   int     `json:"eligible_vendor_count"`
	DegradedMode    bool    `json:"degraded_mode"`
	RoutedAt        string  `json:"routed_at"`
}

type Vendor struct {
	ID          int             `json:"id"`
	Name        string          `json:"name"`
	ServiceZone json.RawMessage `json:"service_zone"`
	CurrentLoad int             `json:"current_load"`
	BaseLoad    int             `json:"base_load"`
}

type ErrorResponse struct {
	Error  string `json:"error"`
	Code   string `json:"code"`
	Detail string `json:"detail,omitempty"`
}
