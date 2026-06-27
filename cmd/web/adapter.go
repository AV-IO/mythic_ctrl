package web

import (
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

// startServices starts the given services (empty = all). rebuildOnStart=false.
func startServices(services []string) error {
	return mgr().StartServices(services, false)
}

func stopServices(services []string) error {
	// deleteImages=false, keepVolume=true: a GUI "stop" should not be destructive.
	return mgr().StopServices(services, false, true)
}

func buildServices(services []string) error {
	return mgr().BuildServices(services, true)
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
// order: (error, []string) where the slice is any additional services pulled in.
func installFromGitHub(url, branch string, force, keepVolume bool) ([]string, error) {
	err, additional := internal.InstallService(url, branch, force, keepVolume)
	return additional, err
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
