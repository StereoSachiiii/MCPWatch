package utils

import (
	"runtime"
)

// GetActiveGoroutineCount returns the number of currently active goroutines.
func GetActiveGoroutineCount() int {
	return runtime.NumGoroutine()
}
