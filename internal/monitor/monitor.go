package monitor

import "context"

// DatabaseMonitor defines the standard interface for external datastore telemetry harvesters
type DatabaseMonitor[T any] interface {
	Harvest(ctx context.Context) (T, error)
	Close(ctx context.Context) error
}
