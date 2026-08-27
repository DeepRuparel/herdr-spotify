# Agent setup — Spotify keybindings (platform independent)

This file is the agent entrypoint. End users should not manually edit `config.toml`.

## For AI agents (Claude, Opencode, etc.)

Run **one** of these — they query Herdr's real config path and write idempotently, works on macOS ( `~/.config/herdr/config.toml` ) and Windows ( `%APPDATA%\herdr\config.toml` automatically):

```bash
# via Herdr plugin action (preferred — works even if binary not in PATH)
herdr plugin action invoke dev.spotify-herdr.setup-keys

# or directly via binary (linked or installed)
./spotify setup-keys
# dry-run: ./spotify setup-keys --dry-run
# query path: ./spotify config-path
# or: herdr plugin action invoke dev.spotify-herdr.config-path
```

What it does (`internal/herdrconfig/herdrconfig.go:11`):
1. Discovers `config.toml` path via `HERDR_CONFIG_PATH` env > `herdr --help` `Config:` line > fallback `~/.config` / `%APPDATA%` heuristic
2. Appends missing `[[keys.command]]` blocks for `prefix+ctrl+p/n/o/f/q/l/s` (idempotent — checks `dev.spotify-herdr.toggle` etc.)
3. Runs `herdr server reload-config` via `HERDR_BIN_PATH`

Keys installed:
- `prefix+ctrl+p` toggle, `prefix+ctrl+n` next, `prefix+ctrl+o` prev (zero-setup)
- `prefix+ctrl+f` search*, `prefix+ctrl+q` queue*, `prefix+ctrl+l` save* (* needs `auth`)
- `prefix+ctrl+s` shell → `herdr plugin pane open --plugin dev.spotify-herdr --entrypoint player` (portable TUI, no absolute path)

Verification:
```bash
herdr plugin log list --plugin dev.spotify-herdr --limit 1
cat "$(./spotify config-path)" | grep -A2 spotify-herdr
```

No manual copy needed. Safe to re-run — won't duplicate.
