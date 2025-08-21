package debug

import (
	"fmt"
	"log"
	"time"
)

// DebugHeader prints debug header if debugging is enabled
func DebugHeader(enabled bool) {
	if enabled {
		log.Printf("=== DEBUG START ===")
	}
}

// DebugFooter prints debug footer if debugging is enabled
func DebugFooter(enabled bool) {
	if enabled {
		log.Printf("=== DEBUG END ===")
	}
}

// DebugOutput prints debug output if debugging is enabled
func DebugOutput(enabled bool, format string, args ...interface{}) {
	if enabled {
		timestamp := time.Now().Format("15:04:05.000")
		message := fmt.Sprintf(format, args...)
		log.Printf("[%s] %s", timestamp, message)
	}
}

// DebugTiming measures and logs execution time if debugging is enabled
func DebugTiming(enabled bool, operation string) func() {
	if !enabled {
		return func() {}
	}
	
	start := time.Now()
	DebugOutput(enabled, "Starting: %s", operation)
	
	return func() {
		duration := time.Since(start)
		DebugOutput(enabled, "Completed: %s (took %v)", operation, duration)
	}
}