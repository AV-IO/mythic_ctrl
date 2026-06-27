package web

import (
	"net/http"
	"strings"
)

// handleDashboard renders the main control page: service status, health, and
// start/stop/restart/build controls.
func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	mythicServices, thirdParty := serviceNames()
	s.renderer.page(w, "dashboard", data("dashboard",
		"MythicServices", mythicServices,
		"ThirdPartyServices", thirdParty,
		"Status", statusText(false),
		"Connection", connectionInfoText(),
	))
}

// handleStatusPartial is polled by htmx (hx-trigger="every 5s") to refresh the
// status panel without a full page reload.
func (s *Server) handleStatusPartial(w http.ResponseWriter, r *http.Request) {
	s.renderer.partial(w, "status_panel", map[string]any{
		"Status": statusText(false),
	})
}

// handleHealthPartial refreshes the health panel on demand / on a timer.
func (s *Server) handleHealthPartial(w http.ResponseWriter, r *http.Request) {
	services := servicesFromForm(r)
	s.renderer.partial(w, "health_panel", map[string]any{
		"Health": healthText(services),
	})
}

func (s *Server) handleStart(w http.ResponseWriter, r *http.Request) {
	services := servicesFromForm(r)
	if err := startServices(services); err != nil {
		s.renderer.toast(w, "error", "Start failed: "+err.Error())
		return
	}
	s.renderer.toast(w, "ok", actionMsg("Started", services))
}

func (s *Server) handleStop(w http.ResponseWriter, r *http.Request) {
	services := servicesFromForm(r)
	if err := stopServices(services); err != nil {
		s.renderer.toast(w, "error", "Stop failed: "+err.Error())
		return
	}
	s.renderer.toast(w, "ok", actionMsg("Stopped", services))
}

func (s *Server) handleRestart(w http.ResponseWriter, r *http.Request) {
	services := servicesFromForm(r)
	if err := stopServices(services); err != nil {
		s.renderer.toast(w, "error", "Restart (stop phase) failed: "+err.Error())
		return
	}
	if err := startServices(services); err != nil {
		s.renderer.toast(w, "error", "Restart (start phase) failed: "+err.Error())
		return
	}
	s.renderer.toast(w, "ok", actionMsg("Restarted", services))
}

func (s *Server) handleBuild(w http.ResponseWriter, r *http.Request) {
	services := servicesFromForm(r)
	if err := buildServices(services); err != nil {
		s.renderer.toast(w, "error", "Build failed: "+err.Error())
		return
	}
	s.renderer.toast(w, "ok", actionMsg("Built", services))
}

// servicesFromForm extracts a list of service names from the "services" form
// field. Whitespace/comma separated. An empty result means "all services",
// matching the CLI behaviour where no args targets everything.
func servicesFromForm(r *http.Request) []string {
	_ = r.ParseForm()
	raw := r.FormValue("services")
	if strings.TrimSpace(raw) == "" {
		return []string{}
	}
	fields := strings.FieldsFunc(raw, func(c rune) bool {
		return c == ',' || c == ' ' || c == '\n' || c == '\t'
	})
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		if f = strings.TrimSpace(f); f != "" {
			out = append(out, f)
		}
	}
	return out
}

func actionMsg(verb string, services []string) string {
	if len(services) == 0 {
		return verb + " all services"
	}
	return verb + " " + strings.Join(services, ", ")
}
