package web

import (
	"bytes"
	"io"
	"log"
	"os"
	"sync"
)

// captureMu serializes stdout/stderr redirection. Redirecting the process-wide
// os.Stdout/os.Stderr is global state, so only one capture can run at a time.
var captureMu sync.Mutex

// captureStdout runs fn while redirecting os.Stdout, os.Stderr, and the default
// logger to a pipe, and returns everything written. This lets the GUI reuse the
// upstream "print to the terminal" info functions (Status, GetHealthCheck,
// PrintConnectionInfo, PrintVolumeInformation, ...) by capturing their output.
//
// It does NOT make functions that call os.Exit safe — those must be avoided in
// handlers. The action methods we call instead all return error.
func captureStdout(fn func()) string {
	captureMu.Lock()
	defer captureMu.Unlock()

	origOut, origErr := os.Stdout, os.Stderr
	origLogOut := log.Writer()
	origLogFlags := log.Flags()

	r, w, err := os.Pipe()
	if err != nil {
		// If we cannot make a pipe, run fn without capture so behavior is at
		// least correct, even if we return nothing.
		fn()
		return ""
	}

	os.Stdout = w
	os.Stderr = w
	log.SetOutput(w)
	log.SetFlags(0)

	// Drain the pipe concurrently so a large amount of output cannot deadlock
	// on the pipe buffer.
	var buf bytes.Buffer
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(&buf, r)
		close(done)
	}()

	func() {
		defer func() { _ = recover() }() // contain panics from reused code
		fn()
	}()

	// Restore, close the writer, wait for the drain to finish.
	os.Stdout = origOut
	os.Stderr = origErr
	log.SetOutput(origLogOut)
	log.SetFlags(origLogFlags)
	_ = w.Close()
	<-done
	_ = r.Close()

	return buf.String()
}
