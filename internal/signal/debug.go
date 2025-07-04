package signal

import (
	"fmt"
	"os"
	"time"
)

// debugLog writes debug messages to a file for troubleshooting.
func debugLog(format string, args ...any) {
	// Only log if DEBUG_SIGNAL env var is set
	if os.Getenv("DEBUG_SIGNAL") == "" {
		return
	}

	f, err := os.OpenFile("/tmp/mentat-debug.log", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()

	msg := fmt.Sprintf("[%s] %s\n", time.Now().Format(time.RFC3339), fmt.Sprintf(format, args...))
	_, _ = f.WriteString(msg)
}
