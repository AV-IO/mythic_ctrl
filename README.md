# mythic-ctrl — Web GUI for `mythic-cli`

A server-rendered web dashboard that drives the **same** Go logic the `mythic-cli` terminal tool uses. It adds a `webgui` subcommand and an HTTP server that imports Mythic_CLI's own packages (`cmd/internal`, `cmd/manager`, `cmd/config`) — it is a thin front-end, not a reimplementation.

Features mirror the CLI surface:

- Start / stop / restart / build services (all or specific containers). Each action runs out-of-process and its full docker output is captured into an on-page panel, terminal-rendered so compose's progress animation collapses to its final state instead of stacking. Buttons show an animated spinner while the action runs.
- Live graphical status card — per-service run/health state, Mythic containers and installed services separated (htmx polling of live docker state)
- Live log streaming per service (Server-Sent Events, via the docker SDK)
- View and edit configuration (`.env`); password fields unmask on focus
- Install agents / C2 profiles from GitHub/GitLab, or by uploading a `.zip`
- Manage images, volumes, and database/file backup & restore

## Build & run

### Recommended: `deploy.sh`

The `deploy.sh` script copies the files into a Mythic checkout and (optionally)
builds the binary using Mythic's own `make local` target — so the GUI is
compiled with the project's toolchain and output conventions:

```sh
./deploy.sh                       # copy into ../Mythic (the default)
./deploy.sh -b                    # copy, then build (make local)
./deploy.sh -d /path/to/Mythic -b # custom Mythic dir, copy + build
```

| Flag                        | Effect                                                                      |
| --------------------------- | --------------------------------------------------------------------------- |
| `-d`, `--mythic-dir <path>` | Path to the Mythic directory (default: `../Mythic`, relative to the script) |
| `-b`, `--build`             | Run `make local` after copying (otherwise files are only copied)            |
| `-h`, `--help`              | Show usage                                                                  |

Then run from the Mythic root (see step 4 below).

### Manual

1. Clone Mythic (which contains `Mythic_CLI/src`).
2. Copy the files in:
   ```sh
   cp    path/to/mythic-ctrl/cmd/webgui.go   <Mythic>/Mythic_CLI/src/cmd/webgui.go
   cp -r path/to/mythic-ctrl/cmd/web         <Mythic>/Mythic_CLI/src/cmd/web
   ```
3. Build (Go 1.25; no new module dependencies — only stdlib plus the already vendored `cobra`, `viper`, and `docker` client):
   ```sh
   cd <Mythic>/Mythic_CLI/src
   go build -o mythic-cli .
   ```
4. Run from the Mythic root so the `.env` resolves:
   ```sh
   ./mythic-cli webgui --host 127.0.0.1 --port 7444
   ```
5. Open `http://127.0.0.1:7444` and sign in with the Mythic admin credentials (`MYTHIC_ADMIN_USER` / `MYTHIC_ADMIN_PASSWORD` from the `.env`). For convenience the server prints those credentials to its log on startup.

### Why it's a "drop-in" and not its own repo

Go's `internal/` import rule means `github.com/MythicMeta/Mythic_CLI/cmd/internal` can only be imported by code **inside that same module**. So the web code cannot live in a separate module that imports Mythic_CLI — it must compile *as part of* the Mythic_CLI module to reduce replicating the existing codebase.

## Security note
**Auth** validates the submitted password against `MYTHIC_ADMIN_PASSWORD` in the local `.env`, and uses `JWT_SECRET` to give the client a JWT. If either of these are missing - you did something weird.
