# mythic-ctrl — Web GUI for `mythic-cli`

A server-rendered web dashboard that drives the **same** Go logic the
`mythic-cli` terminal tool uses. It adds a `webgui` subcommand and an HTTP
server that imports Mythic_CLI's own packages (`cmd/internal`, `cmd/manager`,
`cmd/config`) — it is a thin front-end, not a reimplementation.

Features mirror the CLI surface:

- Start / stop / restart / build services (all or specific containers). Each
  action runs out-of-process and its full docker output is captured into an
  on-page panel, terminal-rendered so compose's progress animation collapses to
  its final state instead of stacking. Buttons show an animated spinner while
  the action runs.
- Live graphical status card — per-service run/health state, Mythic containers
  and installed services separated (htmx polling of live docker state)
- Live log streaming per service (Server-Sent Events, via the docker SDK)
- View and edit configuration (`.env`); password fields unmask on focus
- Install agents / C2 profiles from GitHub/GitLab, or by uploading a `.zip`
- Manage images, volumes, and database/file backup & restore

## Why it's a "drop-in" and not its own repo

Go's `internal/` import rule means
`github.com/MythicMeta/Mythic_CLI/cmd/internal` can only be imported by code
**inside that same module**. So the web code cannot live in a separate module
that imports Mythic_CLI — it must compile *as part of* the Mythic_CLI module.
These files therefore use import paths under
`github.com/MythicMeta/Mythic_CLI/...` and are copied into the Mythic_CLI
source tree to build.

> The IDE will show "cannot find package …/cmd/internal" while editing this
> repo on its own. That is expected — those packages only resolve once the
> files sit inside a Mythic checkout.

## Layout (what gets copied where)

```
this repo                     ->  <Mythic>/Mythic_CLI/src/
  cmd/webgui.go               ->    cmd/webgui.go          (package cmd; registers the subcommand)
  cmd/web/                    ->    cmd/web/               (the web server package)
```

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
3. Build (Go 1.25; no new module dependencies — only stdlib plus the already
   vendored `cobra`, `viper`, and `docker` client):
   ```sh
   cd <Mythic>/Mythic_CLI/src
   go build -o mythic-cli .
   ```
4. Run from the Mythic root so the `.env` resolves:
   ```sh
   ./mythic-cli webgui --host 127.0.0.1 --port 7444
   ```
5. Open `http://127.0.0.1:7444` and sign in with the Mythic admin credentials
   (`MYTHIC_ADMIN_USER` / `MYTHIC_ADMIN_PASSWORD` from the `.env`). For
   convenience the server prints those credentials to its log on startup.

> Login requires `JWT_SECRET` to be set in the Mythic config — sessions are
> signed with it. If it is missing, login refuses with a clear error.

## Security notes

- **Auth** validates the submitted password against `MYTHIC_ADMIN_PASSWORD` in
  the local `.env` using a constant-time comparison, then issues a **stateless
  HS256 JWT** (hand-rolled with the standard library — no new dependency) signed
  with the `JWT_SECRET` config value and stored in an HttpOnly, SameSite=Strict
  cookie (8h TTL). Every protected request re-verifies the signature and expiry;
  there is no server-side session store, so sessions survive a GUI restart.
  Because it reads the password from config (not the live Mythic API), the GUI
  works even while Mythic is stopped — useful, since the GUI is how you start it.
- **Credentials in the log**: the admin user/password are printed to stdout on
  startup as a convenience. Anyone who can read that terminal could already read
  the `.env`, but be mindful if you ship the GUI's stdout to a central log.
- **Binding**: defaults to `127.0.0.1`. Binding to any other interface prints a
  startup warning; only do so behind a TLS reverse proxy on a trusted network.
- **Installing services** runs third-party code from the internet on the host.
  The install form requires an explicit confirmation.
- Destructive actions (volume removal, restore, database reset) require an
  htmx confirmation; the database reset additionally requires typing `RESET`.

## How the CLI logic is reused

All coupling to upstream lives in **`cmd/web/adapter.go`** — if an upstream
signature changes, that's the only file to touch.

| GUI action | Reused upstream call |
|---|---|
| start / stop / build | `internal.ServiceStart / ServiceStop / ServiceBuild` — the same functions the `mythic-cli start/stop/build` commands call (they expand "blank = all", regenerate the compose file, and resolve deps). Run in a child process — see below. |
| remove service | `manager.CLIManager.RemoveServices` |
| status (live, graphical) | docker SDK `ContainerList` (structured run/health state, instead of parsing the stdout-printing `Status`) |
| connection info (graphical) | `config.GetMythicEnv()` — same hosts/ports/SSL/bind keys `PrintConnectionInfo` prints, read as structured data (`connection.go`) and rendered as one-line rows (click the name to copy the URI) instead of a stdout table |
| volume info | `PrintVolumeInformation` (captured from stdout via `capture.go`) |
| config list / set | `config.GetConfigAllStrings`, `config.GetConfigHelp`, `config.SetConfigStrings` |
| install from GitHub | `internal.InstallService(url, branch, force, keepVolume)` |
| install from uploaded `.zip` | unzipped to a temp dir (zip-slip / zip-bomb guarded) then `internal.InstallFolder` |
| images / volumes / backup | corresponding `CLIManager` methods |
| logs | docker SDK `ContainerLogs` (concurrency-safe streaming, instead of the stdout-printing `GetLogs`) |

### The stdout / `os.Exit` hazard
Several upstream info functions print to stdout rather than returning data, and
some code paths call `log.Fatal`/`os.Exit`. In a long-running server `os.Exit`
would kill the GUI (and `recoverPanics` only catches panics, not `os.Exit`).
Mitigations:
1. Prefer the `error`-returning `CLIManager` action methods (called directly).
2. For print-only info functions, `capture.go` redirects stdout/stderr around
   the call (serialized by a mutex) and returns the text.
3. **Control actions run out-of-process.** `internal.ServiceStart/ServiceStop/
   ServiceBuild` *can* `os.Exit` on a docker error (a failed compose up, a
   missing config file in the checkout) — which once took the whole GUI down. So
   instead of calling them in-request, the `webgui` binary re-execs *itself* in a
   hidden `--exec-action` mode (`cmd/web/control.go`) that performs one action
   and exits. The parent captures the child's combined stdout+stderr — with
   `COMPOSE_PROGRESS=plain` and a small VT100 reducer (`renderTerminal`) so
   docker's in-place progress animation collapses to its final text — and stays
   alive no matter how the child dies, surfacing the output (and any failure) in
   the browser.
