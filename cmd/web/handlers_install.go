package web

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
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

// handleInstallUpload accepts a .zip upload, extracts it to a temp directory,
// and installs the resulting folder as a service (mirroring
// `mythic-cli install folder`). Like the GitHub path, this runs third-party
// code, so the template confirms before submit.
func (s *Server) handleInstallUpload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		s.renderer.toast(w, "error", "Upload failed: "+err.Error())
		return
	}
	file, header, err := r.FormFile("archive")
	if err != nil {
		s.renderer.toast(w, "error", "Missing zip file")
		return
	}
	defer file.Close()
	if !strings.HasSuffix(strings.ToLower(header.Filename), ".zip") {
		s.renderer.toast(w, "error", "Please upload a .zip file")
		return
	}
	force := r.FormValue("force") == "on"

	// Stage the upload and extraction in a temp dir we always clean up.
	workDir, err := os.MkdirTemp("", "mythic-ctrl-install-*")
	if err != nil {
		s.renderer.toast(w, "error", "Cannot create temp dir: "+err.Error())
		return
	}
	defer os.RemoveAll(workDir)

	zipPath := filepath.Join(workDir, "upload.zip")
	if err := saveUpload(file, zipPath); err != nil {
		s.renderer.toast(w, "error", "Saving upload failed: "+err.Error())
		return
	}

	extractDir := filepath.Join(workDir, "extracted")
	if err := os.MkdirAll(extractDir, 0o755); err != nil {
		s.renderer.toast(w, "error", err.Error())
		return
	}
	if err := unzip(zipPath, extractDir); err != nil {
		s.renderer.toast(w, "error", "Unzip failed: "+err.Error())
		return
	}
	root, err := installRoot(extractDir)
	if err != nil {
		s.renderer.toast(w, "error", err.Error())
		return
	}

	additional, err := installFromFolder(root, force, true)
	if err != nil {
		s.renderer.toast(w, "error", "Install failed: "+err.Error())
		return
	}
	msg := "Installed from " + header.Filename
	if len(additional) > 0 {
		msg += " (also pulled: " + strings.Join(additional, ", ") + ")"
	}
	s.renderer.toast(w, "ok", msg)
}

// saveUpload streams an uploaded file to dst on disk.
func saveUpload(src io.Reader, dst string) error {
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, src)
	return err
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
