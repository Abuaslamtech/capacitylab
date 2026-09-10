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
	XactCommitRate    float64
	XactRollbackRate  float64
}

// PostgresMonitor queries PostgreSQL system catalogs directly.
type PostgresMonitor struct {
	dsn          string
	conn         *pgx.Conn
	lastHarvest  time.Time
	lastCommit   int64
	lastRollback int64
}

var _ DatabaseMonitor[*PostgresMetrics] = (*PostgresMonitor)(nil)

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
func (p *PostgresMonitor) Close(ctx context.Context) error {
	if p != nil && p.conn != nil {
		return p.conn.Close(ctx)
	}
	return nil
}

// Harvest collects live database telemetry from pg_stat_activity and pg_stat_database
func (p *PostgresMonitor) Harvest(ctx context.Context) (*PostgresMetrics, error) {
	if p == nil || p.conn == nil {
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
		_ = rows.Err()
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

	// 5. Transaction commit / rollback rates from pg_stat_database (README Section 3 & 4)
	var xactCommit, xactRollback int64
	err = p.conn.QueryRow(ctx, `
		SELECT coalesce(sum(xact_commit), 0), coalesce(sum(xact_rollback), 0)
		FROM pg_stat_database 
		WHERE datname = current_database()
	`).Scan(&xactCommit, &xactRollback)
	if err == nil {
		now := time.Now()
		if !p.lastHarvest.IsZero() {
			deltaSec := now.Sub(p.lastHarvest).Seconds()
			if deltaSec > 0 {
				metrics.XactCommitRate = float64(xactCommit-p.lastCommit) / deltaSec
				metrics.XactRollbackRate = float64(xactRollback-p.lastRollback) / deltaSec
				if metrics.XactCommitRate < 0 {
					metrics.XactCommitRate = 0
				}
				if metrics.XactRollbackRate < 0 {
					metrics.XactRollbackRate = 0
				}
			}
		}
		p.lastHarvest = now
		p.lastCommit = xactCommit
		p.lastRollback = xactRollback
	}

	return metrics, nil
}
