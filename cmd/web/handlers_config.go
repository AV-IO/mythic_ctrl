package web

import (
	"net/http"
	"strings"
)

// handleConfigPage lists every configuration entry from the .env with inline
// edit forms. Sensitive-looking values are masked until revealed client-side.
func (s *Server) handleConfigPage(w http.ResponseWriter, r *http.Request) {
	s.renderer.page(w, "config", data("config",
		"Entries", allConfig(),
	))
}

// handleConfigSet writes a single key/value back to the .env and returns an
// updated row fragment plus a toast.
func (s *Server) handleConfigSet(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.renderer.toast(w, "error", "Bad form")
		return
	}
	key := strings.TrimSpace(r.FormValue("key"))
	value := r.FormValue("value")
	if key == "" {
		s.renderer.toast(w, "error", "Missing config key")
		return
	}

	setConfig(key, value)
	s.renderer.toast(w, "ok", "Saved "+key+" (restart affected services to apply)")
}

// isSensitiveKey reports whether a config value should be masked in the UI.
func isSensitiveKey(key string) bool {
	k := strings.ToLower(key)
	for _, frag := range []string{"password", "secret", "token", "api_key", "apikey"} {
		if strings.Contains(k, frag) {
			return true
		}
	}
	return false
}
