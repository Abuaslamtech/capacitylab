package monitor

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// PostgresMetrics holds point-in-time PostgreSQL database telemetry
type PostgresMetrics struct {
	ActiveConnections int
	IdleConnections   int
	MaxConnections    int
	CacheHitRatio     float64
	WaitingLocks      int
}

// PostgresMonitor queries PostgreSQL system catalogs directly
type PostgresMonitor struct {
	dsn  string
	conn *pgx.Conn
}

// NewPostgresMonitor establishes a lightweight telemetry connection to PostgreSQL
func NewPostgresMonitor(dsn string) (*PostgresMonitor, error) {
	if dsn == "" {
		return nil, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to postgres: %w", err)
	}

	return &PostgresMonitor{
		dsn:  dsn,
		conn: conn,
	}, nil
}

// Close terminates the PostgreSQL connection
func (p *PostgresMonitor) Close(ctx context.Context) {
	if p.conn != nil {
		_ = p.conn.Close(ctx)
	}
}

// Harvest collects live database telemetry from pg_stat_activity and pg_stat_database
func (p *PostgresMonitor) Harvest(ctx context.Context) (*PostgresMetrics, error) {
	if p.conn == nil {
		return nil, nil
	}

	metrics := &PostgresMetrics{}

	// 1. Get max_connections
	var maxConn int
	err := p.conn.QueryRow(ctx, "SHOW max_connections").Scan(&maxConn)
	if err == nil {
		metrics.MaxConnections = maxConn
	}

	// 2. Get active vs idle connections from pg_stat_activity
	rows, err := p.conn.Query(ctx, "SELECT state, count(*) FROM pg_stat_activity WHERE state IS NOT NULL GROUP BY state")
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var state string
			var count int
			if err := rows.Scan(&state, &count); err == nil {
				if state == "active" {
					metrics.ActiveConnections = count
				} else if state == "idle" {
					metrics.IdleConnections = count
				}
			}
		}
	}

	// 3. Get buffer cache hit ratio from pg_stat_database
	var cacheRatio *float64
	err = p.conn.QueryRow(ctx, `
		SELECT 
			ROUND((blks_hit * 100.0 / NULLIF(blks_hit + blks_read, 0))::numeric, 2)
		FROM pg_stat_database 
		WHERE datname = current_database()
	`).Scan(&cacheRatio)
	if err == nil && cacheRatio != nil {
		metrics.CacheHitRatio = *cacheRatio
	}

	// 4. Check lock contention from pg_locks
	var lockCount int
	err = p.conn.QueryRow(ctx, "SELECT count(*) FROM pg_locks WHERE NOT granted").Scan(&lockCount)
	if err == nil {
		metrics.WaitingLocks = lockCount
	}

	return metrics, nil
}
