package web

import (
	"net/http"
	"strings"
)

// useVolumeFromForm reports whether the operation should run against the docker
// volume (true) rather than a bind path. Defaults to true, matching the common
// CLI usage.
func useVolumeFromForm(r *http.Request) bool {
	v := strings.ToLower(strings.TrimSpace(r.FormValue("use_volume")))
	return v == "" || v == "on" || v == "true" || v == "1"
}

func (s *Server) handleBackupPage(w http.ResponseWriter, r *http.Request) {
	s.renderer.page(w, "backup", data("backup"))
}

func (s *Server) handleBackupDatabase(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	path := strings.TrimSpace(r.FormValue("path"))
	if path == "" {
		s.renderer.toast(w, "error", "Missing backup path")
		return
	}
	if err := backupDatabase(path, useVolumeFromForm(r)); err != nil {
		s.renderer.toast(w, "error", "Database backup failed: "+err.Error())
		return
	}
	s.renderer.toast(w, "ok", "Database backed up to "+path)
}

func (s *Server) handleBackupFiles(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	path := strings.TrimSpace(r.FormValue("path"))
	if path == "" {
		s.renderer.toast(w, "error", "Missing backup path")
		return
	}
	if err := backupFiles(path, useVolumeFromForm(r)); err != nil {
		s.renderer.toast(w, "error", "Files backup failed: "+err.Error())
		return
	}
	s.renderer.toast(w, "ok", "Files backed up to "+path)
}

func (s *Server) handleRestoreDatabase(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	path := strings.TrimSpace(r.FormValue("path"))
	if path == "" {
		s.renderer.toast(w, "error", "Missing restore path")
		return
	}
	if err := restoreDatabase(path, useVolumeFromForm(r)); err != nil {
		s.renderer.toast(w, "error", "Database restore failed: "+err.Error())
		return
	}
	s.renderer.toast(w, "ok", "Database restored from "+path)
}

func (s *Server) handleRestoreFiles(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	path := strings.TrimSpace(r.FormValue("path"))
	if path == "" {
		s.renderer.toast(w, "error", "Missing restore path")
		return
	}
	if err := restoreFiles(path, useVolumeFromForm(r)); err != nil {
		s.renderer.toast(w, "error", "Files restore failed: "+err.Error())
		return
	}
	s.renderer.toast(w, "ok", "Files restored from "+path)
}

// handleResetDatabase wipes the Mythic database. Highly destructive — the
// template requires an explicit typed confirmation before this is called.
func (s *Server) handleResetDatabase(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	if r.FormValue("confirm") != "RESET" {
		s.renderer.toast(w, "warn", "Type RESET to confirm database reset")
		return
	}
	// ResetDatabase has no error return; run it under capture so any printed
	// output is swallowed and a panic can't take down the server.
	_ = captureStdout(func() { resetDatabase(useVolumeFromForm(r)) })
	s.renderer.toast(w, "ok", "Database reset")
}
