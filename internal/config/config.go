package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const RedirectURI = "http://127.0.0.1:43841/callback"

var Scopes = "user-read-playback-state user-modify-playback-state user-read-currently-playing user-library-modify user-library-read playlist-read-private"

type FileConfig struct {
	ClientID string `json:"client_id"`
}

type Token struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
	ExpiresAt    int64  `json:"expires_at"` // unix ms
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
}

func ConfigDir() string {
	if d := os.Getenv("HERDR_PLUGIN_CONFIG_DIR"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "herdr", "plugins", "dev.spotify-herdr")
}

func ConfigPath() string { return filepath.Join(ConfigDir(), "config.json") }
func TokenPath() string  { return filepath.Join(ConfigDir(), "token.json") }

func EnsureConfigDir() error { return os.MkdirAll(ConfigDir(), 0755) }

func LoadConfig() FileConfig {
	var c FileConfig
	b, err := os.ReadFile(ConfigPath())
	if err != nil {
		return c
	}
	_ = json.Unmarshal(b, &c)
	// also try clientId camel
	if c.ClientID == "" {
		var m map[string]string
		_ = json.Unmarshal(b, &m)
		c.ClientID = m["clientId"]
	}
	return c
}

func SaveConfig(c FileConfig) error {
	if err := EnsureConfigDir(); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(c, "", "  ")
	return os.WriteFile(ConfigPath(), b, 0600)
}

func LoadToken() *Token {
	b, err := os.ReadFile(TokenPath())
	if err != nil {
		return nil
	}
	var t Token
	if err := json.Unmarshal(b, &t); err != nil {
		return nil
	}
	return &t
}

func SaveToken(t *Token) error {
	if err := EnsureConfigDir(); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(t, "", "  ")
	if err := os.WriteFile(TokenPath(), b, 0600); err != nil {
		return err
	}
	_ = os.Chmod(TokenPath(), 0600)
	return nil
}

func GetClientID() string {
	if v := os.Getenv("SPOTIFY_CLIENT_ID"); v != "" {
		return v
	}
	c := LoadConfig()
	return c.ClientID
}
