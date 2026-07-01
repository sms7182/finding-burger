package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"finding.vendor.com/db"
	"finding.vendor.com/internal/config"
	"finding.vendor.com/internal/events"
	"finding.vendor.com/internal/handlers"
	"github.com/gin-gonic/gin"
)

func main() {
	fmt.Println("do you want burger?")

	cfg := config.Load()
	ctx := context.Background()

	pg, err := db.NewPostgres(ctx, cfg.PostgresDSN)
	if err != nil {
		log.Fatalf("postgres init: %v", err)
	}
	defer pg.Close()

	rl := db.NewRedisLoad(cfg.RedisAddr, cfg.RedisOpTimeout, cfg.RedisCBThreshold, cfg.RedisCBCooldown)
	defer rl.Close()

	if vendors, lerr := pg.ListVendors(ctx); lerr == nil {
		for _, v := range vendors {
			rl.SeedIfAbsent(ctx, v.ID, v.BaseLoad)
		}
		log.Printf("seeded load counters for %d vendors", len(vendors))
	} else {
		log.Printf("could not seed load counters: %v", lerr)
	}
	go startLoadFlusher(ctx, pg, rl, 5*time.Second)

	events.EnsureTopic(cfg.KafkaBrokers, cfg.KafkaTopic, 3)
	prod := events.NewProducer(cfg.KafkaBrokers, cfg.KafkaTopic)
	defer prod.Close()

	h := handlers.New(pg, rl, prod, cfg)
	r := gin.Default()
	h.Register(r)

	srv := &http.Server{
		Addr:    ":" + cfg.HTTPPort,
		Handler: r,
	}

	go func() {
		log.Printf("listening on :%s", cfg.HTTPPort)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http server: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	log.Println("shutting down...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("graceful shutdown failed: %v", err)
	}
	fmt.Println("yohuuuu founded burger....")

}

func startLoadFlusher(ctx context.Context, pg *db.Postgres, rl *db.RedisLoad, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		vendors, err := pg.ListVendors(ctx)
		if err != nil {
			continue
		}
		for _, v := range vendors {
			load, degraded := rl.GetLoad(ctx, v.ID)
			if degraded {
				continue
			}
			if err := pg.SetLoad(ctx, v.ID, load); err != nil {
				log.Printf("load flush for vendor %d failed: %v", v.ID, err)
			}
		}
	}
}
