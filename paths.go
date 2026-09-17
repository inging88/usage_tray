package main

// OS 별 경로와 자격증명 읽기. Windows 와 macOS 가 다른 곳은 전부 여기 모은다.
//
// macOS 는 Claude Code 자격증명을 Keychain 에 넣는다고 알려져 있다(파일이 아니다).
// 실기 확인을 못 했으므로 후보 서비스명 몇 개를 차례로 시도하고, 그래도 안 되면
// USAGE_TRAY_CLAUDE_KEYCHAIN_SERVICE 환경변수로 사용자가 지정할 수 있게 뒀다.
// Codex 는 양쪽 다 ~/.codex/auth.json 이므로 분기가 없다.

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func homeDir() string {
	h, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return h
}

// Claude Code 트랜스크립트 루트 — 양쪽 다 ~/.claude/projects 다.
func claudeProjectsDir() string { return filepath.Join(homeDir(), ".claude", "projects") }

func codexSessionsDir() string { return filepath.Join(homeDir(), ".codex", "sessions") }

func codexAuthPath() string { return filepath.Join(homeDir(), ".codex", "auth.json") }

func codexConfigPath() string { return filepath.Join(homeDir(), ".codex", "config.toml") }

// 데이터·로그를 두는 곳. Windows 는 %LOCALAPPDATA%, macOS 는 ~/Library/Application Support.
func dataDir() string {
	var base string
	switch runtime.GOOS {
	case "windows":
		base = os.Getenv("LOCALAPPDATA")
		if base == "" {
			base = filepath.Join(homeDir(), "AppData", "Local")
		}
	case "darwin":
		base = filepath.Join(homeDir(), "Library", "Application Support")
	default:
		base = os.Getenv("XDG_STATE_HOME")
		if base == "" {
			base = filepath.Join(homeDir(), ".local", "state")
		}
	}
	d := filepath.Join(base, "usage-tray")
	_ = os.MkdirAll(d, 0o755)
	return d
}

type claudeCreds struct {
	ClaudeAiOauth struct {
		AccessToken string `json:"accessToken"`
	} `json:"claudeAiOauth"`
}

func parseClaudeCreds(b []byte) string {
	var c claudeCreds
	if err := json.Unmarshal(b, &c); err != nil {
		return ""
	}
	t := strings.TrimSpace(c.ClaudeAiOauth.AccessToken)
	if t == "null" {
		return ""
	}
	return t
}

// 파일 후보 — 양쪽 OS 에서 쓰이는 자리를 모두 본다.
func claudeCredFiles() []string {
	h := homeDir()
	files := []string{filepath.Join(h, ".claude", ".credentials.json")}
	if runtime.GOOS == "windows" {
		if la := os.Getenv("LOCALAPPDATA"); la != "" {
			files = append([]string{filepath.Join(la, "Claude Code", "credentials.json")}, files...)
		}
	} else {
		files = append(files,
			filepath.Join(h, "Library", "Application Support", "Claude Code", "credentials.json"))
	}
	return files
}

// macOS Keychain 후보. `security` 는 OS 기본 탑재다.
// 잠긴 키체인이면 사용자에게 잠금 해제 창이 뜰 수 있으므로, 실패는 조용히 넘긴다.
func claudeKeychainServices() []string {
	if s := os.Getenv("USAGE_TRAY_CLAUDE_KEYCHAIN_SERVICE"); s != "" {
		return []string{s}
	}
	return []string{"Claude Code-credentials", "Claude Code", "claude-code"}
}

func keychainSecret(service string) string {
	if runtime.GOOS != "darwin" {
		return ""
	}
	out, err := exec.Command("security", "find-generic-password", "-s", service, "-w").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// Claude OAuth 토큰. 환경변수 → 파일 → (macOS) Keychain 순서.
func claudeToken() string {
	if t := strings.TrimSpace(os.Getenv("CLAUDE_CODE_OAUTH_TOKEN")); t != "" {
		return t
	}
	for _, p := range claudeCredFiles() {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		if t := parseClaudeCreds(b); t != "" {
			return t
		}
	}
	if runtime.GOOS == "darwin" {
		for _, svc := range claudeKeychainServices() {
			raw := keychainSecret(svc)
			if raw == "" {
				continue
			}
			// Keychain 에 JSON 이 통째로 들어 있는 경우와 토큰만 든 경우 둘 다 받는다.
			if t := parseClaudeCreds([]byte(raw)); t != "" {
				return t
			}
			if strings.HasPrefix(raw, "sk-") || len(raw) > 40 && !strings.HasPrefix(raw, "{") {
				return raw
			}
		}
	}
	return ""
}

type codexAuth struct {
	Tokens struct {
		AccessToken string `json:"access_token"`
		AccountID   string `json:"account_id"`
	} `json:"tokens"`
}

func codexToken() (token, account string) {
	b, err := os.ReadFile(codexAuthPath())
	if err != nil {
		return "", ""
	}
	var a codexAuth
	if json.Unmarshal(b, &a) != nil {
		return "", ""
	}
	return strings.TrimSpace(a.Tokens.AccessToken), strings.TrimSpace(a.Tokens.AccountID)
}

// 자격증명이 있으면 '쓴다' 로 본다. 조회 실패로는 뒤집지 않는다(끈적한 판정).
func detectAgents() (claude, codex bool) {
	return claudeToken() != "", func() bool { t, _ := codexToken(); return t != "" }()
}
