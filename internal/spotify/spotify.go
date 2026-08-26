package spotify

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"herdr-spotify/internal/config"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const TokenURL = "https://accounts.spotify.com/api/token"
const APIBase = "https://api.spotify.com/v1"

func base64URLEncode(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}

func GeneratePKCE() (verifier, challenge string) {
	buf := make([]byte, 64)
	_, _ = rand.Read(buf)
	verifier = base64URLEncode(buf)
	h := sha256.Sum256([]byte(verifier))
	challenge = base64URLEncode(h[:])
	return
}

func BuildAuthURL(clientID, challenge, state string) string {
	v := url.Values{}
	v.Set("client_id", clientID)
	v.Set("response_type", "code")
	v.Set("redirect_uri", config.RedirectURI)
	v.Set("code_challenge_method", "S256")
	v.Set("code_challenge", challenge)
	v.Set("scope", config.Scopes)
	v.Set("state", state)
	return "https://accounts.spotify.com/authorize?" + v.Encode()
}

func ExchangeCode(code, verifier string) (*config.Token, error) {
	clientID := config.GetClientID()
	if clientID == "" {
		return nil, fmt.Errorf("no client_id")
	}
	data := url.Values{}
	data.Set("client_id", clientID)
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("redirect_uri", config.RedirectURI)
	data.Set("code_verifier", verifier)
	resp, err := http.PostForm(TokenURL, data)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("token exchange %d: %s", resp.StatusCode, string(body))
	}
	var t config.Token
	if err := json.Unmarshal(body, &t); err != nil {
		return nil, err
	}
	t.ExpiresAt = time.Now().UnixMilli() + t.ExpiresIn*1000 - 5000
	if err := config.SaveToken(&t); err != nil {
		return nil, err
	}
	return &t, nil
}

func RefreshIfNeeded() (*config.Token, error) {
	t := config.LoadToken()
	if t == nil {
		return nil, fmt.Errorf("not authenticated — run: herdr plugin action invoke dev.spotify-herdr.auth")
	}
	if t.ExpiresAt != 0 && time.Now().UnixMilli() < t.ExpiresAt {
		return t, nil
	}
	if t.RefreshToken == "" {
		return nil, fmt.Errorf("no refresh_token — re-auth required")
	}
	clientID := config.GetClientID()
	data := url.Values{}
	data.Set("grant_type", "refresh_token")
	data.Set("refresh_token", t.RefreshToken)
	data.Set("client_id", clientID)
	resp, err := http.PostForm(TokenURL, data)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("refresh %d: %s", resp.StatusCode, string(body))
	}
	var fresh config.Token
	if err := json.Unmarshal(body, &fresh); err != nil {
		return nil, err
	}
	if fresh.RefreshToken == "" {
		fresh.RefreshToken = t.RefreshToken
	}
	fresh.ExpiresAt = time.Now().UnixMilli() + fresh.ExpiresIn*1000 - 5000
	_ = config.SaveToken(&fresh)
	return &fresh, nil
}

func APIFetch(path, method string, body io.Reader) (*http.Response, error) {
	t, err := RefreshIfNeeded()
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(method, APIBase+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+t.AccessToken)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	client := &http.Client{Timeout: 15 * time.Second}
	return client.Do(req)
}

type PlaybackState struct {
	IsPlaying   bool   `json:"is_playing"`
	ProgressMs  *int   `json:"progress_ms"`
	Item        *Track `json:"item"`
	Device      *Device `json:"device"`
}
type Device struct {
	Name         string `json:"name"`
	Type         string `json:"type"`
	VolumePercent *int  `json:"volume_percent"`
}
type Track struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	URI        string   `json:"uri"`
	DurationMs int      `json:"duration_ms"`
	Artists    []Artist `json:"artists"`
	Album      struct {
		Name string `json:"name"`
	} `json:"album"`
}
type Artist struct{ Name string `json:"name"` }

type QueueResp struct {
	CurrentlyPlaying *Track  `json:"currently_playing"`
	Queue            []Track `json:"queue"`
}

func FormatTrack(t *Track) string {
	if t == nil {
		return "-"
	}
	var arts []string
	for _, a := range t.Artists {
		arts = append(arts, a.Name)
	}
	return strings.TrimSpace(strings.Join(arts, ", ") + " - " + t.Name)
}

func GetPlaybackState() (*PlaybackState, error) {
	resp, err := APIFetch("/me/player", "GET", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 204 {
		return nil, nil
	}
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("get playback %d: %s", resp.StatusCode, string(body))
	}
	var s PlaybackState
	if err := json.Unmarshal(body, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func GetQueue() (*QueueResp, error) {
	resp, err := APIFetch("/me/player/queue", "GET", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("queue %d: %s", resp.StatusCode, string(body))
	}
	var q QueueResp
	if err := json.Unmarshal(body, &q); err != nil {
		return nil, err
	}
	return &q, nil
}
