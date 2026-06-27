package web

import (
	"net/http"
	"strings"
)

// handleImagesSave exports the given services' docker images to a tar at the
// provided output path on the Mythic host.
func (s *Server) handleImagesSave(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	services := servicesFromForm(r)
	out := strings.TrimSpace(r.FormValue("output"))
	if out == "" {
		s.renderer.toast(w, "error", "Missing output path")
		return
	}
	if err := saveImages(services, out); err != nil {
		s.renderer.toast(w, "error", "Save failed: "+err.Error())
		return
	}
	s.renderer.toast(w, "ok", "Saved images to "+out)
}

// handleImagesLoad imports docker images from a tar at the provided path.
func (s *Server) handleImagesLoad(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	path := strings.TrimSpace(r.FormValue("path"))
	if path == "" {
		s.renderer.toast(w, "error", "Missing input path")
		return
	}
	if err := loadImages(path); err != nil {
		s.renderer.toast(w, "error", "Load failed: "+err.Error())
		return
	}
	s.renderer.toast(w, "ok", "Loaded images from "+path)
}

// handleImagesRemove removes dangling/unused Mythic images.
func (s *Server) handleImagesRemove(w http.ResponseWriter, r *http.Request) {
	if err := removeImages(); err != nil {
		s.renderer.toast(w, "error", "Remove failed: "+err.Error())
		return
	}
	s.renderer.toast(w, "ok", "Removed unused images")
}
