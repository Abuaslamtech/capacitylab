package monitor

import (
	"context"
	"testing"
)

func TestDatabaseMonitorInterface(t *testing.T) {
	// Static interface compliance verification
	var _ DatabaseMonitor[*PostgresMetrics] = (*PostgresMonitor)(nil)
	var _ DatabaseMonitor[*RedisMetrics] = (*RedisMonitor)(nil)

	t.Run("Nil monitors handle harvest and close safely", func(t *testing.T) {
		ctx := context.Background()

		var pg *PostgresMonitor
		res, err := pg.Harvest(ctx)
		if err != nil || res != nil {
			t.Errorf("Expected nil result and no error from nil PostgresMonitor")
		}
		if err := pg.Close(ctx); err != nil {
			t.Errorf("Expected no error from nil close")
		}

		var redis *RedisMonitor
		rRes, rErr := redis.Harvest(ctx)
		if rErr != nil || rRes != nil {
			t.Errorf("Expected nil result and no error from nil RedisMonitor")
		}
		if err := redis.Close(ctx); err != nil {
			t.Errorf("Expected no error from nil close")
		}
	})
}
