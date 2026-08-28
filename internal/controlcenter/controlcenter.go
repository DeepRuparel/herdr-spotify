package controlcenter

import (
	"encoding/json"
	"fmt"
	"herdr-spotify/internal/config"
	"herdr-spotify/internal/local"
	"herdr-spotify/internal/spotify"
	"os"
	"os/exec"
	"strings"
	"time"
)

func herdrBin() string {
	if p := os.Getenv("HERDR_BIN_PATH"); p != "" {
		return p
	}
	if p, err := exec.LookPath("herdr"); err == nil {
		return p
	}
	return "herdr"
}

type snapshot struct {
	Result struct {
		Snapshot struct {
			FocusedWorkspaceID string `json:"focused_workspace_id"`
			Workspaces         []struct {
				WorkspaceID string `json:"workspace_id"`
			} `json:"workspaces"`
			Panes []struct {
				PaneID string `json:"pane_id"`
			} `json:"panes"`
			FocusedPaneID string `json:"focused_pane_id"`
		} `json:"snapshot"`
	} `json:"result"`
}

func getSnapshot() (*snapshot, error) {
	cmd := exec.Command(herdrBin(), "api", "snapshot")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var s snapshot
	if err := json.Unmarshal(out, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func getTrack() (track string, isPlaying bool, hasTrack bool) {
	// Prefer API if authenticated
	if config.LoadToken() != nil {
		if st, err := spotify.GetPlaybackState(); err == nil && st != nil && st.Item != nil {
			track = spotify.FormatTrack(st.Item)
			isPlaying = st.IsPlaying
			hasTrack = true
			return
		}
	}
	// Fallback local (macOS SMTC / Windows SMTC etc.)
	if np := local.NowPlayingInfo(); np != nil && np.Text != "" {
		track = np.Text
		isPlaying = np.IsPlaying
		hasTrack = true
		return
	}
	track = "♫ No track"
	isPlaying = false
	hasTrack = false
	return
}

func reportOnce() {
	track, isPlaying, hasTrack := getTrack()
	// Herdr caps token at 80 chars, trim
	if len(track) > 70 {
		track = track[:67] + "..."
	}
	controls := "⏮  ⏯  ⏭"
	if hasTrack {
		if isPlaying {
			controls = "⏮  ⏸  ⏭"
		} else {
			controls = "⏮  ▶  ⏭"
		}
	}
	status := "⏸"
	if isPlaying {
		status = "▶"
	}
	// Discover snapshot
	snap, err := getSnapshot()
	if err != nil {
		// No Herdr server running? Skip
		return
	}
	bin := herdrBin()
	// Report workspace metadata for all workspaces (so track appears regardless of focused workspace)
	// Correct order: herdr workspace report-metadata <WORKSPACE_ID> --source ... --token ...
	for _, ws := range snap.Result.Snapshot.Workspaces {
		cmd := exec.Command(bin, "workspace", "report-metadata", ws.WorkspaceID,
			"--source", "herdr-spotify",
			"--token", fmt.Sprintf("spotify_track=%s", track),
			"--token", fmt.Sprintf("spotify_controls=%s", controls),
			"--token", fmt.Sprintf("spotify_status=%s", status),
			"--ttl-ms", "5000")
		_ = cmd.Run()
	}
	// Report pane metadata to ALL panes so $spotify_track is visible under every agent row
	// (bottom corner under agents pane = last agent's expanded rows). Correct order: <PANE_ID> before --source.
	for _, p := range snap.Result.Snapshot.Panes {
		cmd := exec.Command(bin, "pane", "report-metadata", p.PaneID,
			"--source", "herdr-spotify",
			"--token", fmt.Sprintf("spotify_track=%s", track),
			"--token", fmt.Sprintf("spotify_controls=%s", controls),
			"--token", fmt.Sprintf("spotify_status=%s", status),
			"--ttl-ms", "5000")
		_ = cmd.Run()
	}
	// Also ensure the track is truncated for display; Herdr will cap at 80
	_ = strings.TrimSpace
}

// Run loops forever, reporting every 2s. Blocks until interrupted.
func Run() {
	// Initial report quickly
	reportOnce()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		reportOnce()
	}
}
