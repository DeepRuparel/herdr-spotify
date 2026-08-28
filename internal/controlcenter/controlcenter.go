package controlcenter

import (
	"encoding/json"
	"fmt"
	"herdr-spotify/internal/config"
	"herdr-spotify/internal/local"
	"herdr-spotify/internal/spotify"
	"net"
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
				PaneID      string `json:"pane_id"`
				AgentStatus string `json:"agent_status"`
				Agent       *string `json:"agent"`
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
	if config.LoadToken() != nil {
		if st, err := spotify.GetPlaybackState(); err == nil && st != nil && st.Item != nil {
			track = spotify.FormatTrack(st.Item)
			isPlaying = st.IsPlaying
			hasTrack = true
			return
		}
	}
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

var spotifyPaneID string
var agentReported bool

func findSpotifyPane(snap *snapshot) string {
	if spotifyPaneID != "" {
		for _, p := range snap.Result.Snapshot.Panes {
			if p.PaneID == spotifyPaneID {
				return spotifyPaneID
			}
		}
	}
	for i := len(snap.Result.Snapshot.Panes) - 1; i >= 0; i-- {
		p := snap.Result.Snapshot.Panes[i]
		if p.AgentStatus == "unknown" {
			spotifyPaneID = p.PaneID
			return spotifyPaneID
		}
	}
	if len(snap.Result.Snapshot.Panes) > 0 {
		spotifyPaneID = snap.Result.Snapshot.Panes[len(snap.Result.Snapshot.Panes)-1].PaneID
		return spotifyPaneID
	}
	return ""
}

func ensureSpotifyAgent(spotifyPane string) {
	if agentReported {
		return
	}
	bin := herdrBin()
	cmd := exec.Command(bin, "pane", "report-agent", spotifyPane, "--source", "herdr-spotify", "--agent", "spotify", "--state", "unknown")
	_ = cmd.Run()
	agentReported = true
	ensureBottomView()
}

func ensureBottomView() {
	sock := os.Getenv("HERDR_SOCKET_PATH")
	if sock == "" {
		return
	}
	// Only for HERDR_ENV=1 panes (has socket)
	if os.Getenv("HERDR_ENV") != "1" {
		return
	}
	// Payload: sort by agent asc so claude < opencode < spotify (bottom)
	payload := `{"id":"spotify-view","method":"agent.view.set","params":{"source":"plugin:dev.spotify-herdr","label":"spotify-bottom","sort":[{"field":"agent","order":"asc"}]}}` + "\n"
	conn, err := net.Dial("unix", sock)
	if err != nil {
		return
	}
	defer conn.Close()
	_ = conn.SetWriteDeadline(time.Now().Add(500 * time.Millisecond))
	_, _ = conn.Write([]byte(payload))
	_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	buf := make([]byte, 4096)
	_, _ = conn.Read(buf)
}

func reportOnce() {
	track, isPlaying, hasTrack := getTrack()
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
	snap, err := getSnapshot()
	if err != nil {
		return
	}
	bin := herdrBin()
	spotifyPane := findSpotifyPane(snap)
	if spotifyPane != "" {
		ensureSpotifyAgent(spotifyPane)
		cmd := exec.Command(bin, "pane", "report-metadata", spotifyPane,
			"--source", "herdr-spotify",
			"--token", fmt.Sprintf("spotify_track=%s", track),
			"--token", fmt.Sprintf("spotify_controls=%s", controls),
			"--token", fmt.Sprintf("spotify_status=%s", status),
			"--ttl-ms", "5000")
		_ = cmd.Run()
		for _, p := range snap.Result.Snapshot.Panes {
			if p.PaneID == spotifyPane {
				continue
			}
			cmd := exec.Command(bin, "pane", "report-metadata", p.PaneID,
				"--source", "herdr-spotify",
				"--clear-token", "spotify_track",
				"--clear-token", "spotify_controls",
				"--clear-token", "spotify_status")
			_ = cmd.Run()
		}
	}
	for _, ws := range snap.Result.Snapshot.Workspaces {
		cmd := exec.Command(bin, "workspace", "report-metadata", ws.WorkspaceID,
			"--source", "herdr-spotify",
			"--token", fmt.Sprintf("spotify_track=%s", track),
			"--token", fmt.Sprintf("spotify_controls=%s", controls),
			"--token", fmt.Sprintf("spotify_status=%s", status),
			"--ttl-ms", "5000")
		_ = cmd.Run()
	}
	_ = strings.TrimSpace
}

func Run() {
	reportOnce()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		reportOnce()
	}
}
