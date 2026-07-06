package errhandler

import (
	"fmt"
	"io"
	"os"
)

var (
	osExit           = os.Exit
	stderr io.Writer = os.Stderr
)

// HandleAndExit prints the error to stderr and exits non-zero.
// Upstream callers should wrap errors with %w for context.
func HandleAndExit(err error) {
	if err == nil {
		return
	}
	fmt.Fprintf(stderr, "error: %v\n", err)
	osExit(1)
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
