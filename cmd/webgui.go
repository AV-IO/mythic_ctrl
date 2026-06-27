package cmd

import (
	"log"

	"github.com/MythicMeta/Mythic_CLI/cmd/manager"
	"github.com/MythicMeta/Mythic_CLI/cmd/web"
	"github.com/spf13/cobra"
)

// webguiCmd launches a server-rendered web dashboard that drives the same
// underlying Mythic_CLI logic the terminal commands use (cmd/internal,
// cmd/manager, cmd/config). It is a thin HTTP front-end, not a reimplementation.
var webguiCmd = &cobra.Command{
	Use:   "webgui",
	Short: "Launch a web GUI for controlling Mythic",
	Long: `Launch a server-rendered web dashboard (htmx) that exposes the mythic-cli
command surface in the browser: start/stop/restart services, view status,
health, and logs, manage configuration, install agents/C2 profiles, and
manage images, volumes, and backups.

Authentication reuses the Mythic admin password from the local .env, so the
GUI works even while Mythic itself is stopped. By default the server binds to
127.0.0.1; exposing it to other interfaces should only be done behind a TLS
reverse proxy.`,
	Run: func(cmd *cobra.Command, args []string) {
		host, _ := cmd.Flags().GetString("host")
		port, _ := cmd.Flags().GetInt("port")

		// Initialize the manager (docker compose) exactly like the other
		// subcommands do, so every reused method has a live manager.
		manager.Initialize()

		if err := web.Serve(host, port); err != nil {
			log.Fatalf("[-] web gui exited: %v\n", err)
		}
	},
}

func init() {
	webguiCmd.Flags().String("host", "127.0.0.1", "Interface to bind the web GUI to")
	webguiCmd.Flags().Int("port", 7444, "Port to serve the web GUI on")
	rootCmd.AddCommand(webguiCmd)
}
