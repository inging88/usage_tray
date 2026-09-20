package main

import (
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestCodexReachedTypeCompatibility(t *testing.T) {
	for _, value := range []string{`null`, `"weekly"`, `{"type":"weekly"}`, `[]`, `false`} {
		var response codexUsageResp
		data := `{"rate_limit_reached_type":` + value + `,"rate_limit":{"primary_window":{"used_percent":18,"limit_window_seconds":604800,"reset_at":1790424128}}}`
		if err := json.Unmarshal([]byte(data), &response); err != nil {
			t.Fatal(value, err)
		}
		if response.RateLimit.Primary == nil || *response.RateLimit.Primary.UsedPercent != 18 {
			t.Fatal("lost quota")
		}
	}
}
func TestClaudeKeychainFormats(t *testing.T) {
	raw := []byte(`{"claudeAiOauth":{"accessToken":"test-token"}}`)
	for _, input := range [][]byte{raw, []byte(hex.EncodeToString(raw))} {
		if parseClaudeCreds(input) != "test-token" {
			t.Fatal("credential decoding")
		}
	}
	if parseClaudeCreds([]byte(`{"mcpOAuth":{}}`)) != "" {
		t.Fatal("MCP authorization is not a Claude subscription login")
	}
}
func TestAntigravityQuotas(t *testing.T) {
	fixture := `{"userStatus":{"cascadeModelConfigData":{"clientModelConfigs":[{"label":"Gemini","quotaInfo":{"remainingFraction":0.6381429,"resetTime":"2026-09-19T18:39:00Z"}},{"label":"Claude","quotaInfo":{"remainingFraction":1}},{"label":"Exhausted","quotaInfo":{"remainingFraction":0}},{"label":"Unknown"}]}}}`
	u, err := parseAntigravity([]byte(fixture))
	if err != nil {
		t.Fatal(err)
	}
	if !u.OK || len(u.Models) != 3 || u.Short.Left != 0 || u.Week.Left != -1 {
		t.Fatalf("bad quotas: %+v", u)
	}
	for _, m := range u.Models {
		if m.Label == "Gemini" && (m.Left != 64 || m.ResetAt == 0) {
			t.Fatal(m)
		}
	}
	if _, err := parseAntigravity([]byte(`{"userStatus":{}}`)); err == nil {
		t.Fatal("missing quotas reported as success")
	}
}
func TestAntigravityProcessIsolation(t *testing.T) {
	ps := "123 /Applications/An /Applications/Antigravity.app/Contents/Resources/bin/language_server --csrf_token secret\n456 /bin/sh sh -c echo language_server antigravity --csrf_token not-a-server"
	p := parseAGProcesses(ps)
	if len(p) != 1 || p[0].pid != "123" {
		t.Fatal(p)
	}
	ports := parseAGPorts("p123\nn127.0.0.1:12345\nn*:12346\nn127.0.0.1:12345\nn*:70000\n")
	if strings.Join(ports, ",") != "12345,12346" {
		t.Fatal(ports)
	}
}
func TestLastGoodRetainsOriginalAgeAndError(t *testing.T) {
	fetched := time.Now().Add(-20 * time.Minute)
	previous := emptyAgent()
	previous.OK = true
	previous.FetchedAt = fetched
	previous.Short.Left = 64
	current := emptyAgent()
	current.Error = "offline"
	result := keepLastGood(current, previous, time.Now())
	if result.AgeMin < 20 || result.Error != "offline" || result.FetchedAt != fetched {
		t.Fatal(result)
	}
	again := keepLastGood(current, result, time.Now())
	if again.AgeMin < 20 {
		t.Fatal("stale age reset")
	}
}
func TestDashboardDisplaysProviderErrorsAndQuotas(t *testing.T) {
	s := &State{Updated: time.Now(), Claude: emptyAgent(), Codex: emptyAgent(), Antigravity: emptyAgent()}
	s.Claude.Available = true
	s.Claude.Error = "로그인 필요 <test>"
	s.Antigravity.Available = true
	s.Antigravity.Models = []Window{{Label: "Gemini", Left: 64}}
	html := renderPage(s, nil)
	for _, expected := range []string{"Claude · 5시간", "Antigravity · Gemini", "64", "로그인 필요 &lt;test&gt;"} {
		if !strings.Contains(html, expected) {
			t.Fatal("missing", expected)
		}
	}
	if strings.Contains(html, "로그인 필요 <test>") {
		t.Fatal("unescaped error")
	}
}
