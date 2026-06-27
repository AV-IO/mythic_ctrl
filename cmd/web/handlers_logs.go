package web

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
)

// handleLogsPage renders the live-logs view with a service picker. The browser
// opens an EventSource against /logs/stream?service=NAME to follow output.
func (s *Server) handleLogsPage(w http.ResponseWriter, r *http.Request) {
	mythicServices, thirdParty := serviceNames()
	all := append([]string{}, mythicServices...)
	all = append(all, thirdParty...)
	s.renderer.page(w, "logs", data("logs",
		"Services", all,
		"Selected", r.URL.Query().Get("service"),
	))
}

// handleLogsStream streams a container's logs as Server-Sent Events using the
// docker SDK directly. We avoid the manager's GetLogs (which prints to the
// process-wide stdout) so concurrent viewers don't clash.
func (s *Server) handleLogsStream(w http.ResponseWriter, r *http.Request) {
	service := strings.TrimSpace(r.URL.Query().Get("service"))
	if service == "" {
		http.Error(w, "missing service", http.StatusBadRequest)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		writeSSE(w, flusher, "event: error\ndata: cannot reach docker: "+err.Error())
		return
	}
	defer cli.Close()

	ctx := r.Context()
	reader, err := cli.ContainerLogs(ctx, service, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
		Tail:       "200",
		Timestamps: false,
	})
	if err != nil {
		writeSSE(w, flusher, "event: error\ndata: "+err.Error())
		return
	}
	defer reader.Close()

	// Docker multiplexes stdout/stderr; demux through a pipe so we can read
	// clean lines. (Containers without a TTY use this framed stream.)
	pr, pw := io.Pipe()
	go func() {
		_, copyErr := stdcopy.StdCopy(pw, pw, reader)
		_ = pw.CloseWithError(copyErr)
	}()

	scanner := bufio.NewScanner(pr)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return
		default:
		}
		writeSSE(w, flusher, "data: "+scanner.Text())
	}
}

// writeSSE writes one SSE message (caller supplies "data: ..." / "event: ...")
// and flushes. A blank line terminates the event.
func writeSSE(w http.ResponseWriter, f http.Flusher, payload string) {
	fmt.Fprintf(w, "%s\n\n", payload)
	f.Flush()
}
