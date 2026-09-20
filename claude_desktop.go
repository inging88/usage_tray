package main

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/pbkdf2"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Claude Desktop persists OAuth caches with Electron safeStorage. Read only these
// specific encrypted fields and the app's own Keychain item, never browser cookies.
// Plaintext credentials stay in memory; the desktop app remains responsible for renewal.
func decryptClaudeDesktop(encoded, password string) ([]byte, error) {
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(data) < 19 || !bytes.HasPrefix(data, []byte("v10")) {
		return nil, fmt.Errorf("unsupported desktop encryption")
	}
	ciphertext := data[3:]
	if len(ciphertext)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("invalid ciphertext length")
	}
	key, err := pbkdf2.Key(sha1.New, password, []byte("saltysalt"), 1003, 16)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	plaintext := make([]byte, len(ciphertext))
	cipher.NewCBCDecrypter(block, bytes.Repeat([]byte(" "), aes.BlockSize)).CryptBlocks(plaintext, ciphertext)
	padding := int(plaintext[len(plaintext)-1])
	if padding < 1 || padding > aes.BlockSize || padding > len(plaintext) {
		return nil, fmt.Errorf("invalid padding")
	}
	for _, v := range plaintext[len(plaintext)-padding:] {
		if int(v) != padding {
			return nil, fmt.Errorf("invalid padding")
		}
	}
	return plaintext[:len(plaintext)-padding], nil
}

type desktopCredential struct {
	Token     string  `json:"token"`
	ExpiresAt float64 `json:"expiresAt"`
}

func selectDesktopToken(data []byte, now time.Time) (string, float64) {
	var cache map[string]*desktopCredential
	if json.Unmarshal(data, &cache) != nil {
		return "", 0
	}
	var selected string
	var expiry float64
	for key, entry := range cache {
		// Exclude staging, other hosts, inference-only and plugin credentials.
		if entry == nil || !strings.Contains(key, ":https://api.anthropic.com:") || !strings.Contains(key, "user:profile") {
			continue
		}
		if entry.Token != "" && entry.ExpiresAt > float64(now.UnixMilli()) && entry.ExpiresAt > expiry {
			selected, expiry = entry.Token, entry.ExpiresAt
		}
	}
	return selected, expiry
}
func claudeDesktopToken() string {
	if runtime.GOOS != "darwin" {
		return ""
	}
	p := filepath.Join(homeDir(), "Library", "Application Support", "Claude", "config.json")
	data, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	var config map[string]json.RawMessage
	if json.Unmarshal(data, &config) != nil {
		return ""
	}
	var encrypted []string
	for _, key := range []string{"oauth:tokenCacheV2", "oauth:tokenCache"} {
		var value string
		if json.Unmarshal(config[key], &value) == nil && value != "" {
			encrypted = append(encrypted, value)
		}
	}
	if len(encrypted) == 0 {
		return ""
	}
	password := keychainSecret("Claude Safe Storage")
	if password == "" {
		return ""
	}
	var token string
	var latest float64
	for _, encoded := range encrypted {
		plaintext, err := decryptClaudeDesktop(encoded, password)
		if err != nil {
			continue
		}
		selected, expiry := selectDesktopToken(plaintext, time.Now())
		if expiry > latest {
			token, latest = selected, expiry
		}
	}
	return token
}

// Desktop's local chart history contains percentages, but no reset timestamps.
// Keep that distinction visible rather than inventing a reset time or showing zero.
func claudeDesktopHistory() (AgentUsage, bool) {
	u := emptyAgent()
	u.Available = true
	p := filepath.Join(homeDir(), "Library", "Application Support", "Claude", "plan-usage-history.json")
	data, err := os.ReadFile(p)
	if err != nil {
		return u, false
	}
	return parseClaudeDesktopHistory(data, time.Now())
}
func parseClaudeDesktopHistory(data []byte, now time.Time) (AgentUsage, bool) {
	u := emptyAgent()
	u.Available = true
	var history struct {
		Version int `json:"version"`
		Samples []struct {
			T int64 `json:"t"`
			U struct {
				FH *float64 `json:"fh"`
				SD *float64 `json:"sd"`
			} `json:"u"`
		} `json:"samples"`
	}
	if json.Unmarshal(data, &history) != nil || history.Version != 2 {
		return u, false
	}
	latest := -1
	for i, s := range history.Samples {
		if s.T <= now.UnixMilli() && (s.U.FH != nil || s.U.SD != nil) && (latest < 0 || s.T > history.Samples[latest].T) {
			latest = i
		}
	}
	if latest < 0 {
		return u, false
	}
	s := history.Samples[latest]
	at := time.UnixMilli(s.T)
	if now.Sub(at) > 24*time.Hour {
		return u, false
	}
	u.Short.Left = leftFrom(s.U.FH)
	u.Week.Left = leftFrom(s.U.SD)
	u.OK = true
	u.Src = "앱 기록"
	u.FetchedAt = at
	u.AgeMin = int(now.Sub(at).Minutes())
	u.Error = "Claude 앱에 저장된 사용량입니다. 실시간 한도·리셋 시각 연결에는 키체인 접근이 필요합니다"
	return u, true
}
