package web

import (
	"net/http"
	"strings"
)

// handleVolumesPage shows the docker volumes used by Mythic (captured from the
// CLI's volume table) and offers a remove action per volume.
func (s *Server) handleVolumesPage(w http.ResponseWriter, r *http.Request) {
	s.renderer.page(w, "volumes", data("volumes",
		"VolumeInfo", volumeInfoText(),
	))
}

// handleVolumeRemove deletes a named docker volume. Destructive — confirmed in
// the template before submit.
func (s *Server) handleVolumeRemove(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.renderer.toast(w, "error", "Bad form")
		return
	}
	name := strings.TrimSpace(r.FormValue("volume"))
	if name == "" {
		s.renderer.toast(w, "error", "Missing volume name")
		return
	}
	if err := removeVolume(name); err != nil {
		s.renderer.toast(w, "error", "Remove failed: "+err.Error())
		return
	}
	s.renderer.toast(w, "ok", "Removed volume "+name)
}
