package web

import (
	"fmt"
	"sort"

	"github.com/MythicMeta/Mythic_CLI/cmd/config"
	"github.com/MythicMeta/Mythic_CLI/cmd/internal"
	"github.com/MythicMeta/Mythic_CLI/cmd/manager"
)

// adapter.go is the ONLY file that talks to the upstream Mythic_CLI packages
// (cmd/config and cmd/manager). Every web handler goes through these wrappers,
// so if an upstream signature changes, the fix lives here and nowhere else.
//
// The CLIManager interface (cmd/manager/managerInterface.go) is the primary
// reuse surface. Action methods return error and are called directly. Info
// methods print to stdout (Status, GetHealthCheck, GetLogs, PrintVolumeInformation,
// PrintConnectionInfo, ...) and are wrapped with captureStdout (see capture.go).

// ---- Config -----------------------------------------------------------------

// Viper keys for the Mythic admin account, as stored in the local .env.
// Viper lookups are case-insensitive, so either case works.
const (
	keyAdminUser     = "MYTHIC_ADMIN_USER"
	keyAdminPassword = "MYTHIC_ADMIN_PASSWORD"
)

// keyJWTSecret is the Mythic config key whose value signs our session JWTs.
const keyJWTSecret = "JWT_SECRET"

// jwtSecret returns the signing secret from the Mythic config (.env). Empty if
// unset — callers must treat an empty secret as "auth unavailable".
func jwtSecret() []byte {
	return []byte(config.GetMythicEnv().GetString(keyJWTSecret))
}

// adminCredentials returns the configured admin username and password.
func adminCredentials() (user, pass string) {
	env := config.GetMythicEnv()
	user = env.GetString(keyAdminUser)
	if user == "" {
		user = "mythic_admin" // Mythic's default admin username
	}
	pass = env.GetString(keyAdminPassword)
	return user, pass
}

// ConfigEntry is one configuration key/value with optional help text.
type ConfigEntry struct {
	Key   string
	Value string
	Help  string
}

// allConfig returns every config entry sorted by key.
func allConfig() []ConfigEntry {
	values := config.GetConfigAllStrings()
	help := config.GetConfigHelp(nil) // help for all entries; nil = no filter

	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	out := make([]ConfigEntry, 0, len(keys))
	for _, k := range keys {
		out = append(out, ConfigEntry{Key: k, Value: values[k], Help: help[k]})
	}
	return out
}

// setConfig writes a single key/value back to the .env.
func setConfig(key, value string) {
	config.SetConfigStrings(key, value)
}

// ---- Manager (control / services / images / volumes / backup) --------------

func mgr() manager.CLIManager { return manager.GetManager() }

// runServiceAction dispatches a control action to the internal.Service* function
// — the SAME ones the `mythic-cli start/stop/build` commands call — not the bare
// manager methods. The internal layer expands an empty list to "all services",
// (re)generates the docker-compose file, and resolves dependencies; the manager
// methods do none of that, so calling them directly with an empty slice returns
// instantly without starting anything.
//
// IMPORTANT: these functions can log.Fatal/os.Exit on docker errors (a failed
// compose up, a missing nginx config file, ...). That would kill the whole GUI,
// which recoverPanics cannot prevent. So this never runs inside a request: it is
// invoked only in a CHILD process via RunControlAction (see control.go), where an
// os.Exit terminates the child alone and the parent reports the failure.
func runServiceAction(action string, services []string) error {
	switch action {
	case "start":
		return internal.ServiceStart(services, false)
	case "stop":
		// keepVolume=true so a GUI stop never deletes data volumes.
		return internal.ServiceStop(services, true)
	case "build":
		return internal.ServiceBuild(services, false)
	default:
		return fmt.Errorf("unknown control action %q", action)
	}
}

func removeServices(services []string) error {
	return mgr().RemoveServices(services, true)
}

// statusText returns the human-readable status table the CLI prints, captured
// from stdout.
func statusText(verbose bool) string {
	return captureStdout(func() { mgr().Status(verbose) })
}

// healthText returns the health-check output, captured from stdout.
func healthText(services []string) string {
	return captureStdout(func() { mgr().GetHealthCheck(services) })
}

// connectionInfoText returns the printed connection info (URLs, ports).
func connectionInfoText() string {
	return captureStdout(func() { mgr().PrintConnectionInfo() })
}

// volumeInfoText returns the printed volume table.
func volumeInfoText() string {
	return captureStdout(func() { mgr().PrintVolumeInformation() })
}

func removeVolume(name string) error { return mgr().RemoveVolume(name) }

func saveImages(services []string, outputPath string) error {
	return mgr().SaveImages(services, outputPath)
}

func loadImages(path string) error { return mgr().LoadImages(path) }

func removeImages() error { return mgr().RemoveImages() }

func backupDatabase(path string, useVolume bool) error {
	return mgr().BackupDatabase(path, useVolume)
}

func restoreDatabase(path string, useVolume bool) error {
	return mgr().RestoreDatabase(path, useVolume)
}

func backupFiles(path string, useVolume bool) error {
	return mgr().BackupFiles(path, useVolume)
}

func restoreFiles(path string, useVolume bool) error {
	return mgr().RestoreFiles(path, useVolume)
}

func resetDatabase(useVolume bool) { mgr().ResetDatabase(useVolume) }

// ---- Install (agents / C2 profiles from GitHub) ----------------------------

// installFromGitHub installs a service from a git URL, mirroring the
// `mythic-cli install github <url> [branch]` command. Note the upstream return
// order: (error, map[string]string) where the map describes any additional
// services pulled in (name -> source/path). We flatten it to a name slice.
func installFromGitHub(url, branch string, force, keepVolume bool) ([]string, error) {
	err, additional := internal.InstallService(url, branch, force, keepVolume)
	return additionalNames(additional), err
}

// installFromFolder installs a service from a local folder on disk, mirroring
// `mythic-cli install folder <path>`. The trailing "" matches the CLI call.
func installFromFolder(path string, force, keepVolume bool) ([]string, error) {
	err, additional := internal.InstallFolder(path, force, keepVolume, "")
	return additionalNames(additional), err
}

// additionalNames flattens the (name -> path) map returned by the install
// functions into a sorted slice of service names.
func additionalNames(additional map[string]string) []string {
	names := make([]string, 0, len(additional))
	for name := range additional {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ---- Service inventory ------------------------------------------------------

// serviceNames returns the current Mythic service names plus any installed
// 3rd-party services, for populating the status/control views.
func serviceNames() (mythic []string, thirdParty []string) {
	mythic, _ = mgr().GetCurrentMythicServiceNames()
	thirdParty, _ = mgr().GetInstalled3rdPartyServicesOnDisk()
	sort.Strings(mythic)
	sort.Strings(thirdParty)
	return mythic, thirdParty
}
