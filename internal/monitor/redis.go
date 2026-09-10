package monitor

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisMetrics holds point-in-time Redis telemetry
type RedisMetrics struct {
	ConnectedClients int
	UsedMemoryMB     float64
	InstantaneousOps int
	HitRatio         float64
	EvictedKeys      int64
}

// RedisMonitor inspects Redis system stats via standard client INFO commands.
type RedisMonitor struct {
	addr   string
	client *redis.Client
}

var _ DatabaseMonitor[*RedisMetrics] = (*RedisMonitor)(nil)

// NewRedisMonitor establishes a connection to Redis
func NewRedisMonitor(addr string) (*RedisMonitor, error) {
	if addr == "" {
		return nil, nil
	}

	client := redis.NewClient(&redis.Options{
		Addr:        addr,
		DialTimeout: 2 * time.Second,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to redis: %w", err)
	}

	return &RedisMonitor{
		addr:   addr,
		client: client,
	}, nil
}

// Close terminates the Redis client (implements DatabaseMonitor)
func (r *RedisMonitor) Close(ctx context.Context) error {
	if r != nil && r.client != nil {
		return r.client.Close()
	}
	return nil
}

// Harvest parses INFO stats and INFO memory
func (r *RedisMonitor) Harvest(ctx context.Context) (*RedisMetrics, error) {
	if r == nil || r.client == nil {
		return nil, nil
	}

	info, err := r.client.Info(ctx).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to read redis info: %w", err)
	}

	metrics := &RedisMetrics{}
	lines := strings.Split(info, "\r\n")

	var hits, misses int64

	for _, line := range lines {
		if strings.HasPrefix(line, "connected_clients:") {
			metrics.ConnectedClients, _ = strconv.Atoi(strings.TrimPrefix(line, "connected_clients:"))
		} else if strings.HasPrefix(line, "used_memory:") {
			bytes, _ := strconv.ParseInt(strings.TrimPrefix(line, "used_memory:"), 10, 64)
			metrics.UsedMemoryMB = float64(bytes) / (1024 * 1024)
		} else if strings.HasPrefix(line, "instantaneous_ops_per_sec:") {
			metrics.InstantaneousOps, _ = strconv.Atoi(strings.TrimPrefix(line, "instantaneous_ops_per_sec:"))
		} else if strings.HasPrefix(line, "keyspace_hits:") {
			hits, _ = strconv.ParseInt(strings.TrimPrefix(line, "keyspace_hits:"), 10, 64)
		} else if strings.HasPrefix(line, "keyspace_misses:") {
			misses, _ = strconv.ParseInt(strings.TrimPrefix(line, "keyspace_misses:"), 10, 64)
		} else if strings.HasPrefix(line, "evicted_keys:") {
			metrics.EvictedKeys, _ = strconv.ParseInt(strings.TrimPrefix(line, "evicted_keys:"), 10, 64)
		}
	}

	totalKeyspace := hits + misses
	if totalKeyspace > 0 {
		metrics.HitRatio = (float64(hits) / float64(totalKeyspace)) * 100.0
	}

	return metrics, nil
}
