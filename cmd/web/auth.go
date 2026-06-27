package web

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"sync"
	"time"
)

const sessionCookie = "mythic_ctrl_session"

// sessionStore is an in-memory session set. Sessions live for the process
// lifetime and expire after sessionTTL of inactivity.
type sessionStore struct {
	mu       sync.Mutex
	sessions map[string]time.Time // id -> expiry
}

const sessionTTL = 8 * time.Hour

func newSessionStore() *sessionStore {
	return &sessionStore{sessions: map[string]time.Time{}}
}

func (s *sessionStore) create() string {
	id := randomToken()
	s.mu.Lock()
	s.sessions[id] = time.Now().Add(sessionTTL)
	s.mu.Unlock()
	return id
}

func (s *sessionStore) valid(id string) bool {
	if id == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	exp, ok := s.sessions[id]
	if !ok {
		return false
	}
	if time.Now().After(exp) {
		delete(s.sessions, id)
		return false
	}
	// Sliding expiry.
	s.sessions[id] = time.Now().Add(sessionTTL)
	return true
}

func (s *sessionStore) destroy(id string) {
	s.mu.Lock()
	delete(s.sessions, id)
	s.mu.Unlock()
}

func randomToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// requireAuth gates protected handlers. Unauthenticated browser requests are
// redirected to /login; htmx requests get a 401 with an HX-Redirect header so
// the client navigates instead of swapping a login page into a fragment.
func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(sessionCookie)
		if err != nil || !s.sessions.valid(c.Value) {
			if r.Header.Get("HX-Request") == "true" {
				w.Header().Set("HX-Redirect", "/login")
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleLoginForm(w http.ResponseWriter, r *http.Request) {
	// Already logged in? Go to dashboard.
	if c, err := r.Cookie(sessionCookie); err == nil && s.sessions.valid(c.Value) {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	s.renderer.page(w, "login", data("login"))
}

func (s *Server) handleLoginSubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	user := r.FormValue("username")
	pass := r.FormValue("password")

	if !validateAdminCredentials(user, pass) {
		w.WriteHeader(http.StatusUnauthorized)
		s.renderer.page(w, "login", data("login", "Error", "Invalid credentials"))
		return
	}

	id := s.sessions.create()
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    id,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   r.TLS != nil,
		MaxAge:   int(sessionTTL.Seconds()),
	})
	http.Redirect(w, r, "/", http.StatusFound)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		s.sessions.destroy(c.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})
	http.Redirect(w, r, "/login", http.StatusFound)
}

// validateAdminCredentials checks the submitted username/password against the
// Mythic admin credentials stored in the local .env (read via the config
// package). Comparison is constant-time. See adapter.go for the config access.
func validateAdminCredentials(user, pass string) bool {
	wantUser, wantPass := adminCredentials()
	if wantPass == "" {
		// No admin password configured: refuse rather than allow blank logins.
		return false
	}
	userOK := subtle.ConstantTimeCompare([]byte(user), []byte(wantUser)) == 1
	passOK := subtle.ConstantTimeCompare([]byte(pass), []byte(wantPass)) == 1
	return userOK && passOK
}
