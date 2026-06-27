package web

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// control.go isolates service control (start/stop/build) in a CHILD process.
//
// The upstream internal.Service* functions can call log.Fatal/os.Exit on docker
// errors — e.g. a failed `compose up`, or a missing nginx/postgres config file in
// the Mythic checkout. In a long-running server those would kill the whole GUI,
// and recoverPanics (which only catches panics) cannot stop an os.Exit.
//
// So instead of calling them in-process, we re-exec our own binary in a hidden
// `webgui --exec-action` mode (see cmd/webgui.go) that runs the action and exits.
// The parent captures the child's combined stdout+stderr and its exit status and
// stays alive no matter how the child dies — surfacing the output to the browser.

// controlActionTimeout bounds a single start/stop/build. Builds can be slow
// (image pulls + compilation), so this is generous.
const controlActionTimeout = 30 * time.Minute

// controlAction re-execs this binary to run a service action out-of-process and
// returns the combined stdout+stderr. A non-nil error means the action failed
// (non-zero child exit or the child could not be launched); the returned output
// explains why.
func controlAction(action string, services []string) (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locating binary: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), controlActionTimeout)
	defer cancel()

	args := []string{"webgui", "--exec-action", action}
	if len(services) > 0 {
		args = append(args, "--exec-services", strings.Join(services, ","))
	}

	// The child inherits our working directory (the Mythic root, where .env
	// resolves) and environment, so it sees the same config the GUI does. We add
	// COMPOSE_PROGRESS=plain / NO_COLOR so docker compose emits plain line output
	// for a non-TTY instead of an animated, color-coded spinner.
	cmd := exec.CommandContext(ctx, exe, args...)
	cmd.Env = append(os.Environ(), "COMPOSE_PROGRESS=plain", "NO_COLOR=1")

	out, err := cmd.CombinedOutput()
	clean := renderTerminal(string(out))
	if ctx.Err() == context.DeadlineExceeded {
		return clean, fmt.Errorf("timed out after %s", controlActionTimeout)
	}
	return clean, err
}

// renderTerminal collapses the in-place progress animation docker compose emits
// — carriage returns, cursor-up moves, line erases, and reprints of the same
// block, plus color codes — into the final text a terminal would actually show.
// Without this, merely stripping the control codes leaves every animation frame
// stacked as duplicate lines (the garbled "Last action output").
//
// It implements just enough of a VT100: CR, LF, cursor up/down/forward/back,
// absolute column (G), and erase-to-end-of-line (K). SGR colors, cursor
// show/hide, and other sequences are parsed and ignored.
func renderTerminal(s string) string {
	var screen [][]rune
	row, col := 0, 0
	grow := func(r int) {
		for len(screen) <= r {
			screen = append(screen, nil)
		}
	}
	grow(0)

	r := []rune(s)
	for i := 0; i < len(r); {
		switch c := r[i]; c {
		case '\x1b':
			// Only CSI ("ESC [") is interpreted; anything else is dropped.
			if i+1 < len(r) && r[i+1] == '[' {
				j := i + 2
				for j < len(r) && (r[j] < '@' || r[j] > '~') { // scan to final byte
					j++
				}
				if j < len(r) {
					params := string(r[i+2 : j])
					switch r[j] {
					case 'A':
						if row -= firstParam(params, 1); row < 0 {
							row = 0
						}
					case 'B':
						row += firstParam(params, 1)
						grow(row)
					case 'C':
						col += firstParam(params, 1)
					case 'D':
						if col -= firstParam(params, 1); col < 0 {
							col = 0
						}
					case 'G':
						if col = firstParam(params, 1) - 1; col < 0 {
							col = 0
						}
					case 'K':
						if firstParam(params, 0) == 0 && row < len(screen) && col < len(screen[row]) {
							screen[row] = screen[row][:col] // erase to end of line
						}
					}
					i = j + 1
					continue
				}
			}
			i++
		case '\r':
			col = 0
			i++
		case '\n':
			row++
			col = 0
			grow(row)
			i++
		default:
			grow(row)
			line := screen[row]
			for len(line) < col {
				line = append(line, ' ')
			}
			if col < len(line) {
				line[col] = c
			} else {
				line = append(line, c)
			}
			screen[row] = line
			col++
			i++
		}
	}

	lines := make([]string, len(screen))
	for i, line := range screen {
		lines[i] = strings.TrimRight(string(line), " ")
	}
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1] // drop trailing blank lines
	}
	return strings.Join(lines, "\n")
}

// firstParam returns the leading integer of a CSI parameter string (e.g. "1",
// "0", "?25", "1;2"), or def when there is none.
func firstParam(params string, def int) int {
	start := 0
	for start < len(params) && (params[start] < '0' || params[start] > '9') {
		if params[start] == ';' {
			return def
		}
		start++
	}
	end := start
	for end < len(params) && params[end] >= '0' && params[end] <= '9' {
		end++
	}
	if start == end {
		return def
	}
	n, err := strconv.Atoi(params[start:end])
	if err != nil {
		return def
	}
	return n
}

func startServices(services []string) (string, error) { return controlAction("start", services) }
func stopServices(services []string) (string, error)  { return controlAction("stop", services) }
func buildServices(services []string) (string, error) { return controlAction("build", services) }

// RunControlAction is the CHILD-side entry point invoked by the hidden
// `webgui --exec-action` mode. It runs the requested action against the upstream
// internal.Service* function and terminates the process with status 0 on success
// or 1 on failure. It never returns — any os.Exit the upstream code performs is
// equivalent. The manager must already be initialized by the caller.
func RunControlAction(action, servicesCSV string) {
	var services []string
	for _, s := range strings.Split(servicesCSV, ",") {
		if s = strings.TrimSpace(s); s != "" {
			services = append(services, s)
		}
	}
	if err := runServiceAction(action, services); err != nil {
		fmt.Fprintf(os.Stderr, "[-] %s failed: %v\n", action, err)
		os.Exit(1)
	}
	os.Exit(0)
}
