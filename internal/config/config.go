package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	HTTPPort string

	PostgresDSN string

	RedisAddr string

	KafkaBrokers []string
	KafkaTopic   string

	DiscountThreshold float64
	DiscountRate      float64

	RedisOpTimeout   time.Duration
	RedisCBThreshold int
	RedisCBCooldown  time.Duration
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func Load() Config {
	host := getenv("POSTGRES_HOST", "localhost")
	port := getenv("POSTGRES_PORT", "5432")
	user := getenv("POSTGRES_USER", "postgres")
	pass := getenv("POSTGRES_PASSWORD", "postgres")
	db := getenv("POSTGRES_DB", "vendor")

	dsn := fmt.Sprintf(
		"postgres://%s:%s@%s:%s/%s?sslmode=disable",
		user, pass, host, port, db,
	)

	threshold, err := strconv.ParseFloat(getenv("CART_DISCOUNT_THRESHOLD", "100"), 64)
	if err != nil {
		threshold = 100
	}
	rate, err := strconv.ParseFloat(getenv("DISCOUNT_RATE", "0.10"), 64)
	if err != nil {
		rate = 0.10
	}

	return Config{
		HTTPPort:          getenv("HTTP_PORT", "8080"),
		PostgresDSN:       dsn,
		RedisAddr:         getenv("REDIS_ADDR", "localhost:6379"),
		KafkaBrokers:      splitComma(getenv("KAFKA_BROKERS", "localhost:9092")),
		KafkaTopic:        getenv("KAFKA_TOPIC", "order.routed"),
		DiscountThreshold: threshold,
		DiscountRate:      rate,
		RedisOpTimeout:    150 * time.Millisecond,
		RedisCBThreshold:  3,
		RedisCBCooldown:   5 * time.Second,
	}
}

func splitComma(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
