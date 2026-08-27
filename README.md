# Spotify for Herdr (Go)

Control Spotify from Herdr — **zero-setup** play/pause/next/prev/volume (master on Windows) on macOS/Linux/Windows (AppleScript / playerctl / SMTC SendInput), plus gated search/queue/save ♥ via Spotify Web API PKCE. Controls your existing Premium Connect device (no audio streamed to terminal). Works on macOS/Linux/Windows.

## Install

```bash
herdr plugin install DeepRuparel/herdr-spotify
# or local dev:
herdr plugin link /Users/deepruparel/Desktop/dev/herdr-spotify
herdr plugin list
```

## Zero-setup (no Spotify app needed)

```bash
herdr plugin action invoke dev.spotify-herdr.toggle
herdr plugin action invoke dev.spotify-herdr.next
herdr plugin action invoke dev.spotify-herdr.prev
# or binary directly:
./spotify toggle && ./spotify next && ./spotify volume 70

# TUI (keys: space toggle, n next, p prev, +/- vol, q quit)
herdr plugin pane open --plugin dev.spotify-herdr --entrypoint player

# Keybindings — add to ~/.config/herdr/config.toml then herdr server reload-config
# [[keys.command]] key="prefix+ctrl+p" type="plugin_action" command="dev.spotify-herdr.toggle"
# [[keys.command]] key="prefix+ctrl+n" type="plugin_action" command="dev.spotify-herdr.next"
# [[keys.command]] key="prefix+ctrl+o" type="plugin_action" command="dev.spotify-herdr.prev"
# [[keys.command]] key="prefix+ctrl+s" type="shell" command="herdr plugin pane open --plugin dev.spotify-herdr --entrypoint player"
```

## Gated features (search / queue / save ♥) — one-time 2-min setup

Requires Spotify Web API auth (PKCE). Only needed for search/queue/save; basics work without it.

1. Create app at https://developer.spotify.com/dashboard
   - Redirect URI: `http://127.0.0.1:43841/callback` (exact)
   - Copy **Client ID** (no secret)

2. Save ID and authenticate:
```bash
echo '{"client_id":"YOUR_CLIENT_ID"}' > "$(herdr plugin config-dir dev.spotify-herdr)/config.json"
herdr plugin action invoke dev.spotify-herdr.auth
# — opens browser → approve → token saved to $(herdr plugin config-dir dev.spotify-herdr)/token.json (0600)
# alternative direct: ./spotify auth  (prompts for ID)
# env override: SPOTIFY_CLIENT_ID=xxx ./spotify auth
```

3. Use gated actions:
```bash
herdr plugin action invoke dev.spotify-herdr.search-play
herdr plugin action invoke dev.spotify-herdr.queue
herdr plugin action invoke dev.spotify-herdr.save
# in TUI: / search*, l queue*, L save* (* = gated)
```

## Config

- `$(herdr plugin config-dir dev.spotify-herdr)/config.json` → `{ "client_id": "..." }`
- `$(herdr plugin config-dir dev.spotify-herdr)/token.json` → access/refresh token (auto-refresh)
- Env: `SPOTIFY_CLIENT_ID`

## Stack

Go 1.22+ single binary (`go build -o spotify ./cmd/spotify`, Windows `spotify.exe` via `[[build]]`). Local controls: macOS `osascript` (Spotify.app), Linux `playerctl`, Windows SMTC `SendInput` `VK_MEDIA_*` / `VK_VOLUME_*` (system master volume). API: `accounts.spotify.com` PKCE + `api.spotify.com/v1`. Herdr host: `herdr-plugin.toml` actions/panes, `HERDR_PLUGIN_CONFIG_DIR` (`%APPDATA%/herdr/plugins/config` on Windows).

## Publish

Push with topic `herdr-plugin` and `herdr-plugin.toml` at root, then `herdr plugin install you/herdr-spotify`.
