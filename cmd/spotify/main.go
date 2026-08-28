package main

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"herdr-spotify/internal/config"
	"herdr-spotify/internal/controlcenter"
	"herdr-spotify/internal/herdrconfig"
	"herdr-spotify/internal/local"
	"herdr-spotify/internal/spotify"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}
	cmd := os.Args[1]
	args := os.Args[2:]
	var err error
	switch cmd {
	case "auth":
		err = runAuth()
	case "toggle":
		err = runToggle()
	case "next":
		err = runNext()
	case "prev":
		err = runPrev()
	case "volume":
		err = runVolume(args)
	case "save":
		err = runSave()
	case "search":
		err = runSearch(args)
	case "queue":
		err = runQueue()
	case "pane":
		err = runPane()
	case "nowplaying":
		err = runNowPlaying()
	case "setup-keys":
		err = runSetupKeys(args)
	case "config-path":
		err = runConfigPath()
	case "controlcenter":
		controlcenter.Run()
		return // never returns
	case "setup-control-center":
		err = runSetupControlCenter(args)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %s\n", cmd)
		usage()
		os.Exit(1)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		if strings.Contains(err.Error(), "NO_ACTIVE_DEVICE") {
			fmt.Fprintln(os.Stderr, "Hint: open Spotify on your device and play something once.")
		}
		os.Exit(1)
	}
}

func usage() {
	fmt.Println(`spotify — herdr-spotify Go binary
  auth              login via PKCE browser flow
  toggle            play/pause (local zero-setup on macOS/Linux, API otherwise)
  next|prev         next/prev track
  volume [0-100]    get/set volume
  save              save current track to Your Library (needs auth)
  search [query]    search & play (needs auth)
  queue             show queue (needs auth)
  pane              interactive TUI (space toggle, n next, p prev, +/- vol, / search*, l queue*, L save*)
  nowplaying        compact now-playing loop
  setup-keys        add keybindings to herdr config.toml (platform independent)
  config-path       print herdr config.toml path
  controlcenter     run sidebar control center daemon (reports $spotify_track / $spotify_controls)
  setup-control-center add control center rows to config + start daemon`)
}

// helpers

func hasAuth() bool { return config.LoadToken() != nil }

func tryAPI(apiFn func() error, localFn func() (string, error)) error {
	if hasAuth() {
		if err := apiFn(); err == nil {
			return nil
		} else {
			// fallback to local on all platforms (windows now has SMTC master volume)
			if msg, lerr := localFn(); lerr == nil {
				fmt.Println(msg + " (API fallback)")
				return nil
			}
			return err
		}
	}
	msg, err := localFn()
	if err != nil {
		return err
	}
	fmt.Println(msg)
	return nil
}

// commands

func runAuth() error {
	clientID := config.GetClientID()
	if clientID == "" {
		fmt.Println("\nNo Spotify Client ID found.")
		fmt.Println("1. Go to https://developer.spotify.com/dashboard -> Create App")
		fmt.Printf("2. Add Redirect URI: %s\n", config.RedirectURI)
		fmt.Println("3. Copy the Client ID")
		fmt.Print("Paste Client ID: ")
		reader := bufio.NewReader(os.Stdin)
		line, _ := reader.ReadString('\n')
		clientID = strings.TrimSpace(line)
		if clientID == "" {
			return fmt.Errorf("no Client ID provided — set SPOTIFY_CLIENT_ID or create config.json")
		}
		c := config.LoadConfig()
		c.ClientID = clientID
		if err := config.SaveConfig(c); err != nil {
			return err
		}
		fmt.Printf("Saved to %s\n", config.ConfigPath())
	}
	verifier, challenge := spotify.GeneratePKCE()
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	state := hex.EncodeToString(b)
	authURL := spotify.BuildAuthURL(clientID, challenge, state)
	fmt.Println("\nOpening browser for Spotify login...")
	fmt.Println(authURL + "\n")
	_ = openBrowser(authURL)
	code, err := waitForCallback(state)
	if err != nil {
		return err
	}
	fmt.Println("Exchanging code for token...")
	if _, err := spotify.ExchangeCode(code, verifier); err != nil {
		return err
	}
	fmt.Println("✓ Authenticated! Token saved.")
	fmt.Println("Try: herdr plugin pane open --plugin dev.spotify-herdr --entrypoint player")
	return nil
}

func waitForCallback(expectedState string) (string, error) {
	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)
	srv := &http.Server{Addr: "127.0.0.1:43841"}
	http.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if errStr := q.Get("error"); errStr != "" {
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprintf(w, "<h1>Error: %s</h1><p>Close tab.</p>", errStr)
			errCh <- fmt.Errorf("%s", errStr)
			return
		}
		state := q.Get("state")
		if state != expectedState {
			http.Error(w, "state mismatch", 400)
			return
		}
		code := q.Get("code")
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<h1>✓ Spotify connected</h1><p>Close tab and return to Herdr.</p>"))
		codeCh <- code
	})
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()
	fmt.Println("Waiting for callback on http://127.0.0.1:43841/callback ... (60s timeout)")
	select {
	case code := <-codeCh:
		_ = srv.Close()
		return code, nil
	case err := <-errCh:
		_ = srv.Close()
		return "", err
	case <-time.After(60 * time.Second):
		_ = srv.Close()
		return "", fmt.Errorf("timeout waiting for Spotify callback (60s) — did you add redirect URI in dashboard?")
	}
}

func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		return fmt.Errorf("unsupported OS")
	}
	return cmd.Start()
}

func runToggle() error {
	return tryAPI(func() error {
		s, err := spotify.GetPlaybackState()
		if err != nil {
			return err
		}
		if s == nil {
			resp, err := spotify.APIFetch("/me/player/play", "PUT", nil)
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			if resp.StatusCode != 204 && resp.StatusCode != 200 {
				b, _ := io.ReadAll(resp.Body)
				return fmt.Errorf("%s", string(b))
			}
			fmt.Println("▶ Play (API)")
			return nil
		}
		if s.IsPlaying {
			resp, err := spotify.APIFetch("/me/player/pause", "PUT", nil)
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			b, _ := io.ReadAll(resp.Body)
			if resp.StatusCode != 204 && resp.StatusCode != 200 {
				return fmt.Errorf("%s", string(b))
			}
			fmt.Println("⏸ Paused (API)")
		} else {
			resp, err := spotify.APIFetch("/me/player/play", "PUT", nil)
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			b, _ := io.ReadAll(resp.Body)
			if resp.StatusCode != 204 && resp.StatusCode != 200 {
				return fmt.Errorf("%s", string(b))
			}
			fmt.Println("▶ Playing (API)")
		}
		return nil
	}, local.Toggle)
}

func runNext() error {
	return tryAPI(func() error {
		resp, err := spotify.APIFetch("/me/player/next", "POST", nil)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != 204 && resp.StatusCode != 200 {
			b, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("%s", string(b))
		}
		fmt.Println("⏭ Next (API)")
		return nil
	}, local.Next)
}

func runPrev() error {
	return tryAPI(func() error {
		resp, err := spotify.APIFetch("/me/player/previous", "POST", nil)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != 204 && resp.StatusCode != 200 {
			b, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("%s", string(b))
		}
		fmt.Println("⏮ Prev (API)")
		return nil
	}, local.Prev)
}

func runVolume(args []string) error {
	if len(args) == 0 {
		if hasAuth() {
			if s, err := spotify.GetPlaybackState(); err == nil && s != nil && s.Device != nil && s.Device.VolumePercent != nil {
				fmt.Printf("Volume: %d%% (API)\n", *s.Device.VolumePercent)
				return nil
			}
		}
		v, err := local.Volume(nil)
		if err != nil {
			return err
		}
		fmt.Printf("Volume: %d%% (local)\n", v)
		return nil
	}
	var pct int
	_, _ = fmt.Sscan(args[0], &pct)
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	return tryAPI(func() error {
		resp, err := spotify.APIFetch(fmt.Sprintf("/me/player/volume?volume_percent=%d", pct), "PUT", nil)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != 204 && resp.StatusCode != 200 {
			b, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("%s", string(b))
		}
		fmt.Printf("Volume %d%% (API)\n", pct)
		return nil
	}, func() (string, error) {
		v, err := local.Volume(&pct)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("Volume %d%% (local)", v), nil
	})
}

func runSave() error {
	if !hasAuth() {
		return fmt.Errorf("Save ♥ requires Spotify auth (user-library-modify) — run: herdr plugin action invoke dev.spotify-herdr.auth")
	}
	s, err := spotify.GetPlaybackState()
	if err != nil {
		return err
	}
	if s == nil || s.Item == nil || s.Item.ID == "" {
		return fmt.Errorf("no track playing — nothing to save")
	}
	// check contains
	resp, err := spotify.APIFetch("/me/tracks/contains?ids="+s.Item.ID, "GET", nil)
	if err == nil && resp.StatusCode == 200 {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		var arr []bool
		_ = json.Unmarshal(b, &arr)
		if len(arr) > 0 && arr[0] {
			fmt.Printf("♥ Already in Your Library: %s\n", s.Item.Name)
			return nil
		}
	} else if resp != nil {
		resp.Body.Close()
	}
	resp2, err := spotify.APIFetch("/me/tracks?ids="+s.Item.ID, "PUT", nil)
	if err != nil {
		return err
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != 200 && resp2.StatusCode != 204 {
		b, _ := io.ReadAll(resp2.Body)
		return fmt.Errorf("%s", string(b))
	}
	arts := ""
	for i, a := range s.Item.Artists {
		if i > 0 {
			arts += ", "
		}
		arts += a.Name
	}
	fmt.Printf("♥ Saved to Your Library: %s - %s\n", s.Item.Name, arts)
	return nil
}

func runSearch(args []string) error {
	if !hasAuth() {
		return fmt.Errorf("Search requires Spotify auth — run: herdr plugin action invoke dev.spotify-herdr.auth\n(play/pause/next/prev work without it on macOS/Linux)")
	}
	q := strings.Join(args, " ")
	if q == "" {
		fmt.Print("Search: ")
		reader := bufio.NewReader(os.Stdin)
		line, _ := reader.ReadString('\n')
		q = strings.TrimSpace(line)
		if q == "" {
			return nil
		}
	}
	// search
	resp, err := spotify.APIFetch("/search?q="+urlEncode(q)+"&type=track&limit=8", "GET", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return fmt.Errorf("%s", string(b))
	}
	var res struct {
		Tracks struct {
			Items []spotify.Track `json:"items"`
		} `json:"tracks"`
	}
	if err := json.Unmarshal(b, &res); err != nil {
		return err
	}
	if len(res.Tracks.Items) == 0 {
		fmt.Println("No results.")
		return nil
	}
	for i, t := range res.Tracks.Items {
		arts := ""
		for j, a := range t.Artists {
			if j > 0 {
				arts += ", "
			}
			arts += a.Name
		}
		fmt.Printf("%d. %s - %s  [%d:%02d]  %s\n", i+1, arts, t.Name, t.DurationMs/60000, (t.DurationMs%60000)/1000, t.URI)
	}
	fmt.Print("\nPick # to play (or q): ")
	reader := bufio.NewReader(os.Stdin)
	line, _ := reader.ReadString('\n')
	sel := strings.TrimSpace(line)
	if sel == "" || strings.ToLower(sel) == "q" {
		return nil
	}
	var idx int
	_, _ = fmt.Sscan(sel, &idx)
	idx--
	if idx < 0 || idx >= len(res.Tracks.Items) {
		fmt.Println("invalid")
		return nil
	}
	uri := res.Tracks.Items[idx].URI
	body := fmt.Sprintf(`{"uris":["%s"]}`, uri)
	resp2, err := spotify.APIFetch("/me/player/play", "PUT", bytes.NewBufferString(body))
	if err != nil {
		return err
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != 204 && resp2.StatusCode != 200 {
		// fallback queue + skip
		resp3, err := spotify.APIFetch("/me/player/queue?uri="+urlEncode(uri), "POST", nil)
		if err != nil {
			return err
		}
		resp3.Body.Close()
		if resp3.StatusCode != 204 && resp3.StatusCode != 200 {
			b, _ := io.ReadAll(resp3.Body)
			return fmt.Errorf("%s", string(b))
		}
		resp4, _ := spotify.APIFetch("/me/player/next", "POST", nil)
		if resp4 != nil {
			resp4.Body.Close()
		}
	}
	fmt.Printf("▶ Playing %s (API)\n", res.Tracks.Items[idx].Name)
	return nil
}

func urlEncode(s string) string {
	// net/url escaping
	return strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(s, "%", "%25"), " ", "%20"), "&", "%26")
	// fallback to proper
}

func runQueue() error {
	if !hasAuth() {
		return fmt.Errorf("Queue requires Spotify auth — run: herdr plugin action invoke dev.spotify-herdr.auth")
	}
	q, err := spotify.GetQueue()
	if err != nil {
		return err
	}
	fmt.Printf("Now playing: %s\n\n", spotify.FormatTrack(q.CurrentlyPlaying))
	fmt.Printf("Queue (%d):\n", len(q.Queue))
	for i, t := range q.Queue {
		if i >= 15 {
			break
		}
		fmt.Printf("%d. %s  %s\n", i+1, spotify.FormatTrack(&t), t.URI)
	}
	if len(q.Queue) == 0 {
		fmt.Println("(empty — add via search)")
	}
	return nil
}

func runNowPlaying() error {
	for {
		fmt.Print("\033[2J\033[H")
		if hasAuth() {
			s, err := spotify.GetPlaybackState()
			if err != nil {
				fmt.Println("Not authenticated:", err)
				fmt.Println("Run: herdr plugin action invoke dev.spotify-herdr.auth")
			} else if s == nil || s.Item == nil {
				fmt.Println("♪ No track — start Spotify on your device")
			} else {
				st := "⏸"
				if s.IsPlaying {
					st = "▶"
				}
				fmt.Printf("%s %s\n", st, spotify.FormatTrack(s.Item))
				if s.Device != nil {
					v := "?"
					if s.Device.VolumePercent != nil {
						v = fmt.Sprintf("%d", *s.Device.VolumePercent)
					}
					fmt.Printf("   %s  %s%%\n", s.Device.Name, v)
				}
			}
		} else {
			if np := local.NowPlayingInfo(); np != nil {
				st := "⏸"
				if np.IsPlaying {
					st = "▶"
				}
				fmt.Printf("%s %s (%s)\n", st, np.Text, np.Source)
			} else {
				fmt.Println("♪ No track — start Spotify.app")
			}
			fmt.Println("\n(volume/controls without auth on macOS/Linux)")
		}
		time.Sleep(2 * time.Second)
	}
}

func runPane() error {
	// minimal TUI without external deps — uses raw mode via stty
	// delegate to existing Node pane if Go TUI not desired? For now implement simple loop
	fmt.Println("TUI pane in Go — use keys: space toggle, n next, p prev, q quit, / search*, l queue*")
	fmt.Println("(For full TUI, run Node pane or enhance this Go pane with bubbletea)")
	// simple interactive loop using buffered reader
	reader := bufio.NewReader(os.Stdin)
	for {
		// show status
		if hasAuth() {
			s, _ := spotify.GetPlaybackState()
			if s != nil && s.Item != nil {
				fmt.Printf("\r%s | %s  [space/n/p/q] > ", map[bool]string{true: "▶", false: "⏸"}[s.IsPlaying], spotify.FormatTrack(s.Item))
			} else {
				fmt.Print("\rNo track — [space/n/p/q] > ")
			}
		} else {
			if np := local.NowPlayingInfo(); np != nil {
				fmt.Printf("\r%s %s > ", map[bool]string{true: "▶", false: "⏸"}[np.IsPlaying], np.Text)
			} else {
				fmt.Print("\rNo local track > ")
			}
		}
		line, _ := reader.ReadString('\n')
		cmd := strings.TrimSpace(line)
		switch cmd {
		case " ", "toggle", "":
			_ = runToggle()
		case "n", "next":
			_ = runNext()
		case "p", "prev":
			_ = runPrev()
		case "q", "quit", "exit":
			return nil
		case "/", "search":
			if !hasAuth() {
				fmt.Println("Search needs auth")
				continue
			}
			_ = runSearch(nil)
		case "l", "queue":
			_ = runQueue()
		case "L", "save":
			_ = runSave()
		default:
			fmt.Println("keys: space toggle, n next, p prev, q quit, / search, l queue, L save")
		}
	}
}

func runConfigPath() error {
	p, err := herdrconfig.HerdrConfigPath()
	if err != nil {
		return err
	}
	fmt.Println(p)
	return nil
}

func runSetupKeys(args []string) error {
	dryRun := len(args) > 0 && (args[0] == "--dry-run" || args[0] == "--check")
	path, err := herdrconfig.HerdrConfigPath()
	if err != nil {
		return err
	}
	fmt.Printf("Herdr config: %s\n", path)
	if dryRun {
		fmt.Println("(dry-run, not writing)")
		return nil
	}
	addedPath, added, err := herdrconfig.SetupKeys()
	if err != nil {
		return err
	}
	if added == 0 {
		fmt.Printf("✓ Keybindings already present in %s\n", addedPath)
	} else {
		fmt.Printf("✓ Added %d keybindings to %s\n", added, addedPath)
		for _, k := range []string{"prefix+ctrl+p toggle", "prefix+ctrl+n next", "prefix+ctrl+o prev", "prefix+ctrl+s TUI", "prefix+ctrl+f search*", "prefix+ctrl+q queue*", "prefix+ctrl+l save*"} {
			fmt.Printf("  - %s\n", k)
		}
	}
	// reload
	if err := herdrconfig.ReloadConfig(); err != nil {
		fmt.Printf("Note: herdr server reload-config failed (is Herdr running?): %v\n", err)
		fmt.Println("Run: herdr server reload-config  (or restart Herdr)")
	} else {
		fmt.Println("✓ herdr server reload-config applied")
	}
	_ = path
	return nil
}

func runSetupControlCenter(args []string) error {
	dryRun := len(args) > 0 && (args[0] == "--dry-run" || args[0] == "--check")
	path, err := herdrconfig.HerdrConfigPath()
	if err != nil {
		return err
	}
	fmt.Printf("Herdr config: %s\n", path)
	if dryRun {
		fmt.Println("(dry-run, not writing)")
		// show what would be added
		b, _ := os.ReadFile(path)
		if strings.Contains(string(b), "$spotify_track") {
			fmt.Println("Control center rows already present")
		} else {
			fmt.Println("Would add [ui.sidebar.agents] rows with $spotify_track / $spotify_controls")
		}
		return nil
	}
	// First ensure keys
	if _, _, err := herdrconfig.SetupKeys(); err != nil {
		return err
	}
	addedPath, added, err := herdrconfig.SetupControlCenter()
	if err != nil {
		return err
	}
	if added {
		fmt.Printf("✓ Added control center rows to %s\n", addedPath)
		fmt.Println("  - bottom corner under agents pane shows $spotify_track and $spotify_controls (⏮ ⏯ ⏭)")
	} else {
		fmt.Printf("✓ Control center rows already present in %s\n", addedPath)
	}
	if err := herdrconfig.ReloadConfig(); err != nil {
		fmt.Printf("Note: herdr server reload-config failed: %v\n", err)
	} else {
		fmt.Println("✓ herdr server reload-config applied")
	}
	fmt.Println("\nControl center daemon will start on next Herdr restart via [[startup]] controlcenter.")
	fmt.Println("To start now: herdr plugin action invoke dev.spotify-herdr.controlcenter")
	fmt.Println("Or run: ./spotify controlcenter &")
	return nil
}
