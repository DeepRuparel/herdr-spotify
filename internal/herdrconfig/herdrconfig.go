package herdrconfig

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// HerdrConfigPath returns the Herdr config.toml path, platform independent.
// Priority: HERDR_CONFIG_PATH env > parsing `herdr --help` (Config: ...) > fallback heuristic.
func HerdrConfigPath() (string, error) {
	if p := os.Getenv("HERDR_CONFIG_PATH"); p != "" {
		return p, nil
	}
	// Try HERDR_BIN_PATH then PATH
	herdrBin := os.Getenv("HERDR_BIN_PATH")
	if herdrBin == "" {
		if p, err := exec.LookPath("herdr"); err == nil {
			herdrBin = p
		} else {
			herdrBin = "herdr"
		}
	}
	// herdr --help prints "Config: /path/to/config.toml"
	cmd := exec.Command(herdrBin, "--help")
	out, err := cmd.CombinedOutput()
	if err == nil {
		scanner := bufio.NewScanner(strings.NewReader(string(out)))
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if strings.HasPrefix(line, "Config:") {
				p := strings.TrimSpace(strings.TrimPrefix(line, "Config:"))
				if p != "" {
					return p, nil
				}
			}
		}
	}
	// Fallback heuristic (matches herdr's own logic)
	if runtime.GOOS == "windows" {
		if dir, err := os.UserConfigDir(); err == nil && dir != "" {
			return filepath.Join(dir, "herdr", "config.toml"), nil
		}
		if appdata := os.Getenv("APPDATA"); appdata != "" {
			return filepath.Join(appdata, "herdr", "config.toml"), nil
		}
	}
	// Unix: respect XDG_CONFIG_HOME, else ~/.config
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "herdr", "config.toml"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home dir: %w", err)
	}
	return filepath.Join(home, ".config", "herdr", "config.toml"), nil
}

var keybindings = []struct {
	Key     string
	Type    string
	Command string
	Desc    string
}{
	{"prefix+ctrl+p", "plugin_action", "dev.spotify-herdr.toggle", "Spotify: Play/Pause (zero-setup)"},
	{"prefix+ctrl+n", "plugin_action", "dev.spotify-herdr.next", "Spotify: Next track"},
	{"prefix+ctrl+o", "plugin_action", "dev.spotify-herdr.prev", "Spotify: Prev track"},
	{"prefix+ctrl+f", "plugin_action", "dev.spotify-herdr.search-play", "Spotify: Search & Play* (needs auth)"},
	{"prefix+ctrl+q", "plugin_action", "dev.spotify-herdr.queue", "Spotify: Show queue*"},
	{"prefix+ctrl+l", "plugin_action", "dev.spotify-herdr.save", "Spotify: Save ♥ to library*"},
	{"prefix+ctrl+s", "shell", "herdr plugin pane open --plugin dev.spotify-herdr --entrypoint player", "Spotify: Open player TUI"},
}

func blockFor(k struct {
	Key     string
	Type    string
	Command string
	Desc    string
}) string {
	return fmt.Sprintf("[[keys.command]]\nkey = \"%s\"\ntype = \"%s\"\ncommand = \"%s\"\ndescription = \"%s\"\n", k.Key, k.Type, k.Command, k.Desc)
}

// SetupKeys idempotently appends missing Spotify keybindings to herdr config.toml.
// Returns path, added count, and reload result.
func SetupKeys() (path string, added int, err error) {
	path, err = HerdrConfigPath()
	if err != nil {
		return "", 0, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return path, 0, err
	}
	var existing string
	if b, err := os.ReadFile(path); err == nil {
		existing = string(b)
	} else if !os.IsNotExist(err) {
		return path, 0, err
	}
	var toAdd []string
	for _, k := range keybindings {
		// idempotency: check if command already present
		if strings.Contains(existing, k.Command) {
			continue
		}
		toAdd = append(toAdd, blockFor(k))
	}
	if len(toAdd) == 0 {
		return path, 0, nil
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return path, 0, err
	}
	defer f.Close()
	if existing != "" && !strings.HasSuffix(strings.TrimSpace(existing), "") {
		// ensure newline
		if !strings.HasSuffix(existing, "\n") {
			if _, err := f.WriteString("\n"); err != nil {
				return path, 0, err
			}
		}
	}
	header := "\n# Spotify for Herdr — auto-added by herdr-spotify setup-keys (prefix=ctrl+b)\n"
	if _, err := f.WriteString(header); err != nil {
		return path, 0, err
	}
	for _, b := range toAdd {
		if _, err := f.WriteString(b + "\n"); err != nil {
			return path, 0, err
		}
	}
	return path, len(toAdd), nil
}

func ReloadConfig() error {
	herdrBin := os.Getenv("HERDR_BIN_PATH")
	if herdrBin == "" {
		if p, err := exec.LookPath("herdr"); err == nil {
			herdrBin = p
		} else {
			herdrBin = "herdr"
		}
	}
	cmd := exec.Command(herdrBin, "server", "reload-config")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("herdr server reload-config failed: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// SetupControlCenter adds sidebar rows for the bottom-corner control center.
// It is idempotent: if $spotify_track is already in the file, nothing is added.
// For dedicated single space, we use rows_by_agent so only the spotify agent shows track/controls.
func SetupControlCenter() (path string, added bool, err error) {
	path, err = HerdrConfigPath()
	if err != nil {
		return "", false, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return path, false, err
	}
	b, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return path, false, err
	}
	existing := ""
	if b != nil {
		existing = string(b)
	}
	if strings.Contains(existing, "$spotify_track") {
		return path, false, nil
	}
	// Dedicated bottom-corner: all agents show 4 rows, but only the dedicated Spotify pane has tokens
	// So the bottom-most agent (Spotify, sorted last via agent.view.set) shows the track/controls.
	// We avoid rows_by_agent with custom id (invalid) and rely on single-pane tokens + view sorting.
	block := `
# Spotify control center — dedicated bottom space under agents pane (added by setup-control-center)
# All agents have extra rows for $spotify_track/$spotify_controls, but only the dedicated Spotify agent (bottom) has tokens
[ui.sidebar.agents]
rows = [["state_icon", "workspace", "tab"], ["agent"], ["$spotify_track"], ["$spotify_controls"]]
row_gap = 0

[ui.sidebar.spaces]
rows = [["state_icon", "workspace"], ["branch", "git_status"], ["$spotify_track"]]
`
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return path, false, err
	}
	defer f.Close()
	if existing != "" && !strings.HasSuffix(existing, "\n") {
		f.WriteString("\n")
	}
	if _, err := f.WriteString(block); err != nil {
		return path, false, err
	}
	return path, true, nil
}
