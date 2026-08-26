# AGENTS — herdr-spotify

## What this is
Single Go binary Herdr plugin `dev.spotify-herdr` (`herdr-plugin.toml:1`, `go.mod:1` module `herdr-spotify`). Controls Spotify: zero-setup local `toggle/next/prev/volume` on macOS/Linux, gated `search/queue/save` via Spotify Web API PKCE. No audio in terminal — controls existing Premium Connect device.

## Entrypoints
- `cmd/spotify/main.go:22` — single binary, subcommands `auth toggle next prev volume save search queue pane nowplaying` (switch at `main.go:30`)
- `internal/config/config.go:9` — `RedirectURI`, `Scopes`, `ConfigDir()` (reads `HERDR_PLUGIN_CONFIG_DIR` or `~/.config/herdr/plugins/dev.spotify-herdr`), `Token` with `ExpiresAt` ms
- `internal/spotify/spotify.go:13` — PKCE, `BuildAuthURL`, `ExchangeCode`, `RefreshIfNeeded`, `APIFetch` (`api.spotify.com/v1`)
- `internal/local/local.go:17` — darwin `osascript` / linux `playerctl` fallbacks, `NowPlayingInfo()`

## Build / verify
```bash
go vet ./...                              # only static check in this repo
go build -o spotify ./cmd/spotify         # must rerun manually — [[build]] only runs on `herdr plugin install`, not `link`
./spotify toggle && ./spotify volume      # zero-setup smoke (needs Spotify.app running on darwin)
./spotify queue 2>&1 | head                # should gate: "requires Spotify auth" when no token
```

## Herdr plugin flow (non-obvious)
- `herdr plugin link /path/to/herdr-spotify` for local dev; `herdr plugin install DeepRuparel/herdr-spotify` clones to Herdr-managed store and runs `[[build]]` (`herdr-plugin.toml:8`). Link does NOT run build.
- Actions/panes run detached with **no stdin** — interactive `auth` will fail via `herdr plugin action invoke` unless `config.json` pre-exists. For prompt, run directly in a Herdr pane: `HERDR_PLUGIN_CONFIG_DIR=$(herdr plugin config-dir dev.spotify-herdr) ./spotify auth`
- `herdr plugin pane open --plugin dev.spotify-herdr --entrypoint player` works from shell, but a `keys.command` popup that runs that CLI flashes (exits instantly). Run binary directly in popup: `command = "/abs/path/spotify pane"` (`~/.config/herdr/config.toml` pattern).
- Config/token live **outside repo** at `$(herdr plugin config-dir dev.spotify-herdr)/{config.json,token.json}` (0600). Never commit; `.gitignore:3` ignores root `/spotify` binary only — not `cmd/spotify/` (was bug at `/spotify` vs `spotify`).

## Secrets / env
- Spotify Dashboard: Redirect URI must be exactly `http://127.0.0.1:43841/callback` (`config.go:9`). Copy **Client ID only** — PKCE public client, `client_secret` never used/sent.
- `SPOTIFY_CLIENT_ID` env overrides `config.json`. `HERDR_PLUGIN_CONFIG_DIR` overrides config dir for tests.
- `token.json` auto-refreshes (`spotify.go:85`); stale scopes require re-`auth`.

## Conventions / gotchas
- `toggle/next/prev` try API first if token exists, fallback to `local` on darwin/linux (`main.go:82` `tryAPI`). `search/queue/save` hard-gate on `hasAuth()` and error immediately.
- `go.mod` is Go 1.27, zero external deps. `herdr-plugin.toml` version bump manually on release.
- No tests/CI/lint config in repo — `go vet` is the check. No `opencode.json` custom instructions beyond `{"$schema":...}`.
- Add `herdr-plugin` topic on GitHub for marketplace discovery.
