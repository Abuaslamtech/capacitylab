//go:build windows

package sentinel

import (
	"time"
)

// getProcessCPUTime returns total user + system CPU duration on Windows
func getProcessCPUTime() time.Duration {
	return 0
}
