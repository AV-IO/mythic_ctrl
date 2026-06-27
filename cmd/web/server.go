package web

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

//go:embed templates static
var embedded embed.FS

// Server holds the shared state for the web GUI: parsed templates, the static
// file handler, and the in-memory session store.
type Server struct {
	host     string
	port     int
	renderer *renderer
	sessions *sessionStore
}

// Serve builds the server, wires routes, and blocks until interrupted.
func Serve(host string, port int) error {
	r, err := newRenderer(embedded)
	if err != nil {
		return fmt.Errorf("parsing templates: %w", err)
	}

	s := &Server{
		host:     host,
		port:     port,
		renderer: r,
		sessions: newSessionStore(),
	}

	mux := http.NewServeMux()
	s.routes(mux)

	addr := net.JoinHostPort(host, fmt.Sprintf("%d", port))
	httpServer := &http.Server{
		Addr:              addr,
		Handler:           logRequests(mux),
		ReadHeaderTimeout: 10 * time.Second,
		// No write timeout: SSE log streams are long-lived.
	}

	if !isLoopback(host) {
		log.Printf("[!] WARNING: binding to %s exposes the Mythic control panel beyond localhost.\n"+
			"[!] Only do this behind a TLS reverse proxy on a trusted network.\n", host)
	}

	// Graceful shutdown on Ctrl-C / SIGTERM.
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
		<-sigCh
		log.Println("[*] Shutting down web GUI...")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(ctx)
	}()

	log.Printf("[+] Mythic web GUI listening on http://%s\n", addr)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// routes registers every endpoint. Static assets and the login page are public;
// everything else passes through requireAuth.
func (s *Server) routes(mux *http.ServeMux) {
	staticFS, _ := fs.Sub(embedded, "static")
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))

	// Auth (public).
	mux.HandleFunc("GET /login", s.handleLoginForm)
	mux.HandleFunc("POST /login", s.handleLoginSubmit)
	mux.HandleFunc("POST /logout", s.handleLogout)

	// Pages (protected).
	mux.Handle("GET /{$}", s.requireAuth(http.HandlerFunc(s.handleDashboard)))
	mux.Handle("GET /config", s.requireAuth(http.HandlerFunc(s.handleConfigPage)))
	mux.Handle("GET /logs", s.requireAuth(http.HandlerFunc(s.handleLogsPage)))
	mux.Handle("GET /install", s.requireAuth(http.HandlerFunc(s.handleInstallPage)))
	mux.Handle("GET /volumes", s.requireAuth(http.HandlerFunc(s.handleVolumesPage)))
	mux.Handle("GET /backup", s.requireAuth(http.HandlerFunc(s.handleBackupPage)))

	// Control actions (protected). Each returns an htmx fragment / toast.
	mux.Handle("GET /partials/status", s.requireAuth(http.HandlerFunc(s.handleStatusPartial)))
	mux.Handle("GET /partials/health", s.requireAuth(http.HandlerFunc(s.handleHealthPartial)))
	mux.Handle("POST /control/start", s.requireAuth(http.HandlerFunc(s.handleStart)))
	mux.Handle("POST /control/stop", s.requireAuth(http.HandlerFunc(s.handleStop)))
	mux.Handle("POST /control/restart", s.requireAuth(http.HandlerFunc(s.handleRestart)))
	mux.Handle("POST /control/build", s.requireAuth(http.HandlerFunc(s.handleBuild)))

	// Config actions (protected).
	mux.Handle("POST /config/set", s.requireAuth(http.HandlerFunc(s.handleConfigSet)))

	// Logs SSE stream (protected).
	mux.Handle("GET /logs/stream", s.requireAuth(http.HandlerFunc(s.handleLogsStream)))

	// Install / services (protected).
	mux.Handle("POST /install/github", s.requireAuth(http.HandlerFunc(s.handleInstallGitHub)))
	mux.Handle("POST /install/upload", s.requireAuth(http.HandlerFunc(s.handleInstallUpload)))
	mux.Handle("POST /install/remove", s.requireAuth(http.HandlerFunc(s.handleServiceRemove)))

	// Images (protected).
	mux.Handle("POST /images/save", s.requireAuth(http.HandlerFunc(s.handleImagesSave)))
	mux.Handle("POST /images/load", s.requireAuth(http.HandlerFunc(s.handleImagesLoad)))
	mux.Handle("POST /images/remove", s.requireAuth(http.HandlerFunc(s.handleImagesRemove)))

	// Volumes (protected).
	mux.Handle("POST /volumes/remove", s.requireAuth(http.HandlerFunc(s.handleVolumeRemove)))

	// Backup / restore (protected, destructive ones confirmed client-side).
	mux.Handle("POST /backup/database", s.requireAuth(http.HandlerFunc(s.handleBackupDatabase)))
	mux.Handle("POST /backup/files", s.requireAuth(http.HandlerFunc(s.handleBackupFiles)))
	mux.Handle("POST /backup/restore-database", s.requireAuth(http.HandlerFunc(s.handleRestoreDatabase)))
	mux.Handle("POST /backup/restore-files", s.requireAuth(http.HandlerFunc(s.handleRestoreFiles)))
	mux.Handle("POST /backup/reset-database", s.requireAuth(http.HandlerFunc(s.handleResetDatabase)))
}

// logRequests is a tiny access logger.
func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("[web] %s %s %s", r.Method, r.URL.Path, time.Since(start).Round(time.Millisecond))
	})
}

func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
