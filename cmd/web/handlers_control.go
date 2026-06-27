package web

import (
	"net/http"
	"strings"
)

// handleDashboard renders the main control page: the graphical service status
// card (live run/health state) plus start/stop/restart/build controls.
func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	mythicServices, thirdParty := serviceNames()
	s.renderer.page(w, "dashboard", data("dashboard",
		"MythicServices", mythicServices,
		"ThirdPartyServices", thirdParty,
		"Status", liveStatus(),
		"Connection", connectionInfoText(),
	))
}

// handleStatusPartial is polled by htmx (hx-trigger="every 5s") to refresh the
// status card without a full page reload.
func (s *Server) handleStatusPartial(w http.ResponseWriter, r *http.Request) {
	s.renderer.partial(w, "status_panel", map[string]any{
		"Status": liveStatus(),
	})
}

func (s *Server) handleStart(w http.ResponseWriter, r *http.Request) {
	services := servicesFromForm(r)
	out, err := startServices(services)
	s.actionResult(w, "Start", actionMsg("Started", services), out, err)
}

func (s *Server) handleStop(w http.ResponseWriter, r *http.Request) {
	services := servicesFromForm(r)
	out, err := stopServices(services)
	s.actionResult(w, "Stop", actionMsg("Stopped", services), out, err)
}

func (s *Server) handleRestart(w http.ResponseWriter, r *http.Request) {
	services := servicesFromForm(r)
	stopOut, err := stopServices(services)
	if err != nil {
		s.actionResultRaw(w, "error", "Restart (stop phase) failed", stopOut)
		return
	}
	startOut, err := startServices(services)
	out := stopOut + "\n" + startOut
	if err != nil {
		s.actionResultRaw(w, "error", "Restart (start phase) failed", out)
		return
	}
	s.actionResultRaw(w, "ok", actionMsg("Restarted", services), out)
}

func (s *Server) handleBuild(w http.ResponseWriter, r *http.Request) {
	services := servicesFromForm(r)
	out, err := buildServices(services)
	s.actionResult(w, "Build", actionMsg("Built", services), out, err)
}

// actionResult renders the outcome of a control action: a toast plus the child
// process's captured output (swapped out-of-band into the dashboard's output
// panel). On error the captured output is what tells the operator why it failed.
func (s *Server) actionResult(w http.ResponseWriter, verb, okMsg, output string, err error) {
	if err != nil {
		s.actionResultRaw(w, "error", verb+" failed", output)
		return
	}
	s.actionResultRaw(w, "ok", okMsg, output)
}

func (s *Server) actionResultRaw(w http.ResponseWriter, level, msg, output string) {
	s.renderer.partial(w, "action_result", map[string]any{
		"Level":   level,
		"Message": msg,
		"Output":  strings.TrimSpace(output),
	})
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
