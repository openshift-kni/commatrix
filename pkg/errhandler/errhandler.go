package errhandler

import (
	"fmt"
	"os"
)

// HandleAndExit prints the error to stderr and exits non-zero.
// Upstream callers should wrap errors with %w for context.
func HandleAndExit(err error) {
	if err == nil {
		return
	}
	fmt.Fprintf(os.Stderr, "error: %v\n", err)
	os.Exit(1)
}

// RecoverAndExit recovers from panics in main, logs them, and exits with a generic message.
func RecoverAndExit() {
	if r := recover(); r != nil {
		var err error
		switch v := r.(type) {
		case error:
			err = v
		default:
			err = fmt.Errorf("%v", v)
		}
		HandleAndExit(err)
	}
}
