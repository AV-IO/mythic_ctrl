# mythic-ctrl — Web GUI for `mythic-cli`

A server-rendered web dashboard that drives the **same** Go logic the
`mythic-cli` terminal tool uses. It adds a `webgui` subcommand and an HTTP
server that imports Mythic_CLI's own packages (`cmd/internal`, `cmd/manager`,
`cmd/config`) — it is a thin front-end, not a reimplementation.

Features mirror the CLI surface:

- Start / stop / restart / build services (all or specific containers)
- Live status + health panels (htmx polling)
- Live log streaming per service (Server-Sent Events, via the docker SDK)
- View and edit configuration (`.env`)
- Install agents / C2 profiles from GitHub/GitLab
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
   (`MYTHIC_ADMIN_USER` / `MYTHIC_ADMIN_PASSWORD` from the `.env`).

## Security notes

- **Auth** validates the submitted password against `MYTHIC_ADMIN_PASSWORD` in
  the local `.env` using a constant-time comparison, then issues an HttpOnly,
  SameSite=Strict session cookie. Because it reads the password from config (not
  the live Mythic API), the GUI works even while Mythic is stopped — useful,
  since the GUI is how you start it.
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
| start / stop / build / remove | `manager.CLIManager.StartServices / StopServices / BuildServices / RemoveServices` |
| status / health / connection / volume info | `Status` / `GetHealthCheck` / `PrintConnectionInfo` / `PrintVolumeInformation` (captured from stdout via `capture.go`) |
| config list / set | `config.GetConfigAllStrings`, `config.GetConfigHelp`, `config.SetConfigStrings` |
| install from GitHub | `internal.InstallService(url, branch, force, keepVolume)` |
| images / volumes / backup | corresponding `CLIManager` methods |
| logs | docker SDK `ContainerLogs` (concurrency-safe streaming, instead of the stdout-printing `GetLogs`) |

### The stdout / `os.Exit` hazard
Several upstream info functions print to stdout rather than returning data, and
some code paths call `log.Fatal`/`os.Exit`. In a long-running server `os.Exit`
would kill the GUI. Mitigations:
1. Prefer the `error`-returning `CLIManager` action methods (called directly).
2. For print-only info functions, `capture.go` redirects stdout/stderr around
   the call (serialized by a mutex) and returns the text.
3. Never call an `os.Exit` path from a handler; use the manager equivalent.
