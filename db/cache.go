package db

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

const FallbackLoad = 0

type RedisLoad struct {
	client    *redis.Client
	opTimeout time.Duration

	mu        sync.Mutex
	failures  int
	threshold int
	cooldown  time.Duration
	openUntil time.Time
}

func NewRedisLoad(addr string, opTimeout time.Duration, threshold int, cooldown time.Duration) *RedisLoad {
	return &RedisLoad{
		client:    redis.NewClient(&redis.Options{Addr: addr}),
		opTimeout: opTimeout,
		threshold: threshold,
		cooldown:  cooldown,
	}
}

func loadKey(id int) string { return fmt.Sprintf("vendor:load:%d", id) }

func (r *RedisLoad) circuitOpen() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return time.Now().Before(r.openUntil)
}

func (r *RedisLoad) recordSuccess() {
	r.mu.Lock()
	r.failures = 0
	r.mu.Unlock()
}

func (r *RedisLoad) recordFailure() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.failures++
	if r.failures >= r.threshold {
		r.openUntil = time.Now().Add(r.cooldown)
		r.failures = 0
		log.Printf("redis circuit OPEN for %s after %d consecutive failures; serving fallback load",
			r.cooldown, r.threshold)
	}
}

func (r *RedisLoad) GetLoad(ctx context.Context, id int) (load int, degraded bool) {
	if r.circuitOpen() {
		return FallbackLoad, true
	}

	cctx, cancel := context.WithTimeout(ctx, r.opTimeout)
	defer cancel()

	v, err := r.client.Get(cctx, loadKey(id)).Int()
	switch {
	case err == redis.Nil:
		// No counter yet means zero in-flight orders — a normal, healthy case.
		r.recordSuccess()
		return 0, false
	case err != nil:
		r.recordFailure()
		log.Printf("redis GetLoad(%d) failed, using fallback: %v", id, err)
		return FallbackLoad, true
	default:
		r.recordSuccess()
		return v, false
	}
}

func (r *RedisLoad) IncrLoad(ctx context.Context, id int) (load int, degraded bool) {
	if r.circuitOpen() {
		return FallbackLoad, true
	}

	cctx, cancel := context.WithTimeout(ctx, r.opTimeout)
	defer cancel()

	v, err := r.client.Incr(cctx, loadKey(id)).Result()
	if err != nil {
		r.recordFailure()
		log.Printf("redis IncrLoad(%d) failed (order still routed): %v", id, err)
		return FallbackLoad, true
	}
	r.recordSuccess()
	return int(v), false
}

func (r *RedisLoad) SeedIfAbsent(ctx context.Context, id, base int) {
	cctx, cancel := context.WithTimeout(ctx, r.opTimeout)
	defer cancel()
	if err := r.client.SetNX(cctx, loadKey(id), base, 0).Err(); err != nil {
		log.Printf("redis seed for vendor %d failed (non-fatal): %v", id, err)
	}
}

func (r *RedisLoad) Ping(ctx context.Context) error {
	cctx, cancel := context.WithTimeout(ctx, r.opTimeout)
	defer cancel()
	return r.client.Ping(cctx).Err()
}

func (r *RedisLoad) Close() error { return r.client.Close() }
