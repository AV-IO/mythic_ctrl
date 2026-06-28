package web

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

// status.go builds the dashboard's graphical status card from LIVE docker state,
// queried through the docker SDK directly (the same client used for log
// streaming) instead of parsing the upstream Status() stdout table. Working from
// structured container data lets us colour each service by its real run/health
// state and keeps the Mythic core "Containers" separate from installed
// "Services" — the two groups the operator thinks in.

// dockerQueryTimeout bounds a single status poll so a wedged docker socket can't
// stall the 5s-interval refresh.
const dockerQueryTimeout = 5 * time.Second

// ServiceStatus is the live state of one Mythic service / container.
type ServiceStatus struct {
	Name   string // container / service name
	Label  string // short human state ("running", "unhealthy", "stopped", ...)
	Detail string // docker's full status line, for the hover tooltip
	Class  string // CSS state class driving the dot colour: ok | warn | error | idle
}

// StatusGroup is one labelled column of services (e.g. "Containers").
type StatusGroup struct {
	Title string
	Items []ServiceStatus
}

// StatusModel is the whole status card: the separated groups plus a roll-up
// count, or an Error string when docker is unreachable.
type StatusModel struct {
	Groups  []StatusGroup
	Total   int
	Running int
	Error   string
	// Overall drives the status card's glowing border, and reflects only the
	// core "Containers" group — third-party Services may be stopped without
	// affecting it. ok (green) only when every core container is running;
	// error (red) as soon as any core container is stopped or unhealthy; warn
	// (yellow) while a core container is transitioning; idle (grey) when there
	// are no core containers.
	Overall string
}

// rawState is the slice of a docker container summary we actually use, copied
// out so no docker-SDK type appears in a function signature (the exact summary
// type name has churned across SDK versions).
type rawState struct {
	state  string // "running", "exited", "created", ...
	status string // "Up 2 hours (healthy)", "Exited (0) 3 minutes ago", ...
}

// liveStatus polls docker and assembles the status card, splitting Mythic core
// services from installed 3rd-party services.
func liveStatus() StatusModel {
	mythic, thirdParty := serviceNames()

	states, err := containerStates()
	model := StatusModel{
		Groups: []StatusGroup{
			{Title: "Containers", Items: statusFor(mythic, states)},
			{Title: "Services", Items: statusFor(thirdParty, states)},
		},
	}
	if err != nil {
		model.Error = err.Error()
	}
	for _, g := range model.Groups {
		for _, s := range g.Items {
			model.Total++
			if s.Class == "ok" || s.Class == "warn" || s.Class == "error" {
				model.Running++ // anything docker still has "up" counts as running
			}
		}
	}
	// The border tracks only the core "Containers" group (index 0); third-party
	// Services are allowed to be down.
	model.Overall = coreOverall(model.Groups[0].Items, model.Error)
	return model
}

// coreOverall rolls the core containers up into the border colour:
//   - green  — every core container is running & healthy
//   - grey   — every core container is off (cleanly stopped), or there are no
//     core containers at all
//   - red    — a partial outage: some core containers up while others are
//     stopped, any core container unhealthy, or docker is unreachable
//   - yellow — converging: some core containers transitioning and none down
//
// (Start/Stop/Restart in progress is forced yellow separately, in CSS.)
func coreOverall(core []ServiceStatus, dockerErr string) string {
	if dockerErr != "" {
		return "error" // can't reach docker -> treat as a fault
	}
	if len(core) == 0 {
		return "idle"
	}
	var ok, bad, off int
	for _, s := range core {
		switch s.Class {
		case "ok":
			ok++
		case "error":
			bad++
		case "warn":
			// transitioning; reflected via the totals below
		default: // "idle": stopped / not started
			off++
		}
	}
	total := len(core)
	switch {
	case bad > 0:
		return "error" // anything unhealthy is a problem
	case off == total:
		return "idle" // cleanly all-off -> grey, not an alarm
	case ok == total:
		return "ok" // all up & healthy
	case off == 0:
		return "warn" // only running + transitioning, none down -> converging
	default:
		return "error" // partial outage: some up, some down
	}
}

// statusFor maps each known service name to its live state, preserving the
// caller's ordering.
func statusFor(names []string, states map[string]rawState) []ServiceStatus {
	out := make([]ServiceStatus, 0, len(names))
	for _, name := range names {
		rs, found := states[name]
		label, class := classify(rs, found)
		out = append(out, ServiceStatus{
			Name:   name,
			Label:  label,
			Detail: rs.status,
			Class:  class,
		})
	}
	return out
}

// classify turns a container's raw state into a display label and CSS class.
// The colour model is deliberately simple: green = running & healthy, yellow =
// transitioning (starting/restarting), red = up but unhealthy, gray = not
// running.
func classify(rs rawState, found bool) (label, class string) {
	if !found {
		return "not started", "idle"
	}

	status := strings.ToLower(rs.status)
	health := ""
	switch {
	case strings.Contains(status, "(unhealthy)"):
		health = "unhealthy"
	case strings.Contains(status, "(healthy)"):
		health = "healthy"
	case strings.Contains(status, "health: starting"):
		health = "starting"
	}

	switch rs.state {
	case "running":
		switch health {
		case "unhealthy":
			return "unhealthy", "error"
		case "starting":
			return "starting", "warn"
		default:
			return "running", "ok"
		}
	case "restarting":
		return "restarting", "warn"
	case "paused":
		return "paused", "warn"
	case "created":
		return "created", "idle"
	case "exited", "dead":
		return "stopped", "idle"
	default:
		if rs.state == "" {
			return "not started", "idle"
		}
		return rs.state, "idle"
	}
}

// containerStates returns every container (running or not) keyed by name, with
// the leading "/" docker adds stripped off.
func containerStates() (map[string]rawState, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("cannot reach docker: %w", err)
	}
	defer cli.Close()

	ctx, cancel := context.WithTimeout(context.Background(), dockerQueryTimeout)
	defer cancel()

	list, err := cli.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return nil, fmt.Errorf("listing containers: %w", err)
	}

	states := make(map[string]rawState, len(list))
	for _, c := range list {
		rs := rawState{state: c.State, status: c.Status}
		for _, n := range c.Names {
			states[strings.TrimPrefix(n, "/")] = rs
		}
	}
	return states, nil
}
