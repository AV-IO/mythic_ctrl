package web

import (
	"net/http"
	"strings"
)

// handleInstallPage lists installed 3rd-party services and offers a form to
// install a new agent / C2 profile from a GitHub (or GitLab) URL.
func (s *Server) handleInstallPage(w http.ResponseWriter, r *http.Request) {
	_, thirdParty := serviceNames()
	s.renderer.page(w, "install", data("install",
		"Installed", thirdParty,
	))
}

// handleInstallGitHub installs a service from a git URL. This runs third-party
// code from the internet — the template warns the operator before submit.
func (s *Server) handleInstallGitHub(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.renderer.toast(w, "error", "Bad form")
		return
	}
	url := strings.TrimSpace(r.FormValue("url"))
	branch := strings.TrimSpace(r.FormValue("branch"))
	force := r.FormValue("force") == "on"
	if url == "" {
		s.renderer.toast(w, "error", "Missing repository URL")
		return
	}

	additional, err := installFromGitHub(url, branch, force, true)
	if err != nil {
		s.renderer.toast(w, "error", "Install failed: "+err.Error())
		return
	}
	msg := "Installed from " + url
	if len(additional) > 0 {
		msg += " (also pulled: " + strings.Join(additional, ", ") + ")"
	}
	s.renderer.toast(w, "ok", msg)
}

// handleServiceRemove stops and removes the containers for a service. (Full
// on-disk uninstall is intentionally separate; this is the container-level
// removal exposed by the manager interface.)
func (s *Server) handleServiceRemove(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.renderer.toast(w, "error", "Bad form")
		return
	}
	name := strings.TrimSpace(r.FormValue("service"))
	if name == "" {
		s.renderer.toast(w, "error", "Missing service name")
		return
	}
	if err := removeServices([]string{name}); err != nil {
		s.renderer.toast(w, "error", "Remove failed: "+err.Error())
		return
	}
	s.renderer.toast(w, "ok", "Removed containers for "+name)
}
