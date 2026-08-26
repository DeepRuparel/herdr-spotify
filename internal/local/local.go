package local

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

func execOut(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).CombinedOutput()
	s := strings.TrimSpace(string(out))
	if err != nil {
		return s, fmt.Errorf("%s", s)
	}
	return s, nil
}

func hasCommand(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func osa(script string) (string, error) {
	return execOut("osascript", "-e", script)
}

func macRunning() bool {
	s, err := osa(`tell application "System Events" to (name of processes) contains "Spotify"`)
	if err != nil {
		return false
	}
	return s == "true"
}

func Toggle() (string, error) {
	switch runtime.GOOS {
	case "darwin":
		if !macRunning() {
			return "", fmt.Errorf("Spotify.app not running — open Spotify and play something once")
		}
		if _, err := osa(`tell application "Spotify" to playpause`); err != nil {
			return "", err
		}
		return "▶/⏸ toggled (Spotify.app)", nil
	case "linux":
		if !hasCommand("playerctl") {
			return "", fmt.Errorf("playerctl not found — install it or use Spotify API auth")
		}
		if _, err := execOut("playerctl", "-p", "spotify", "play-pause"); err != nil {
			if _, err2 := execOut("playerctl", "play-pause"); err2 != nil {
				return "", err
			}
		}
		return "▶/⏸ toggled (playerctl)", nil
	default:
		return "", fmt.Errorf("local control not supported on Windows — run: herdr plugin action invoke dev.spotify-herdr.auth")
	}
}

func Next() (string, error) {
	switch runtime.GOOS {
	case "darwin":
		if !macRunning() {
			return "", fmt.Errorf("Spotify.app not running")
		}
		_, err := osa(`tell application "Spotify" to next track`)
		return "⏭ Next (Spotify.app)", err
	case "linux":
		if !hasCommand("playerctl") {
			return "", fmt.Errorf("playerctl not found")
		}
		if _, err := execOut("playerctl", "-p", "spotify", "next"); err != nil {
			_, _ = execOut("playerctl", "next")
		}
		return "⏭ Next (playerctl)", nil
	default:
		return "", fmt.Errorf("local control not supported on Windows — use Spotify API")
	}
}

func Prev() (string, error) {
	switch runtime.GOOS {
	case "darwin":
		if !macRunning() {
			return "", fmt.Errorf("Spotify.app not running")
		}
		_, err := osa(`tell application "Spotify" to previous track`)
		return "⏮ Prev (Spotify.app)", err
	case "linux":
		if !hasCommand("playerctl") {
			return "", fmt.Errorf("playerctl not found")
		}
		if _, err := execOut("playerctl", "-p", "spotify", "previous"); err != nil {
			_, _ = execOut("playerctl", "previous")
		}
		return "⏮ Prev (playerctl)", nil
	default:
		return "", fmt.Errorf("local control not supported on Windows — use Spotify API")
	}
}

func Volume(pct *int) (int, error) {
	if pct == nil {
		switch runtime.GOOS {
		case "darwin":
			if !macRunning() {
				return 0, fmt.Errorf("Spotify.app not running")
			}
			s, err := osa(`tell application "Spotify" to sound volume`)
			if err != nil {
				return 0, err
			}
			var v int
			_, _ = fmt.Sscan(s, &v)
			return v, nil
		case "linux":
			if hasCommand("playerctl") {
				s, err := execOut("playerctl", "volume")
				if err == nil {
					var f float64
					_, _ = fmt.Sscan(s, &f)
					return int(f * 100), nil
				}
			}
			return 0, fmt.Errorf("volume query needs API or playerctl")
		default:
			return 0, fmt.Errorf("volume query needs Spotify API")
		}
	}
	v := *pct
	if v < 0 {
		v = 0
	}
	if v > 100 {
		v = 100
	}
	switch runtime.GOOS {
	case "darwin":
		if !macRunning() {
			return 0, fmt.Errorf("Spotify.app not running")
		}
		_, err := osa(fmt.Sprintf(`tell application "Spotify" to set sound volume to %d`, v))
		return v, err
	case "linux":
		if hasCommand("playerctl") {
			_, err := execOut("playerctl", "volume", fmt.Sprintf("%.2f", float64(v)/100))
			return v, err
		}
		return 0, fmt.Errorf("local volume not supported — use API")
	default:
		return 0, fmt.Errorf("local volume not supported — use API")
	}
}

type NowPlaying struct {
	Text      string
	IsPlaying bool
	Source    string
}

func NowPlayingInfo() *NowPlaying {
	switch runtime.GOOS {
	case "darwin":
		if !macRunning() {
			return nil
		}
		artist, _ := osa(`tell application "Spotify" to artist of current track`)
		name, _ := osa(`tell application "Spotify" to name of current track`)
		if artist == "" && name == "" {
			return nil
		}
		state, _ := osa(`tell application "Spotify" to player state`)
		return &NowPlaying{Text: strings.TrimSpace(artist + " - " + name), IsPlaying: state == "playing", Source: "Spotify.app"}
	case "linux":
		if !hasCommand("playerctl") {
			return nil
		}
		s, _ := execOut("playerctl", "-p", "spotify", "metadata", "--format", "{{artist}} - {{title}}")
		s = strings.TrimSpace(s)
		if s == "" || s == " - " {
			return nil
		}
		st, _ := execOut("playerctl", "-p", "spotify", "status")
		return &NowPlaying{Text: s, IsPlaying: strings.TrimSpace(st) == "Playing", Source: "playerctl"}
	}
	return nil
}
