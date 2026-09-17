package main

// 한도 조회. Claude 와 Codex 둘 다 실시간 엔드포인트를 쓰고, 실패하면 로컬 기록으로 폴백한다.
//
//	Claude : GET https://api.anthropic.com/api/oauth/usage       (OAuth 토큰)
//	Codex  : GET https://chatgpt.com/backend-api/codex/usage     (ChatGPT OAuth 토큰 + account-id)
//
// Codex 쪽은 Cloudflare 가 TLS 핑거프린트로 막을 수 있다(.NET 스택은 실제로 403 이 온다).
// Go 의 net/http 가 통과하는지는 환경에 따라 다르므로, 403/1010 을 받으면 curl 로 한 번 더 시도한다
// (curl 은 Windows 10 1803+ · macOS 기본 탑재다).

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

type Window struct {
	Left    int    `json:"left"`    // 남은 % (-1 = 값 없음)
	ResetAt int64  `json:"resetAt"` // unix 초, 0 = 모름
	WindowS int64  `json:"windowS"` // 창 길이(초)
	Label   string `json:"label"`   // 5h / 7d 등 사람이 읽는 이름
}

type AgentUsage struct {
	Available bool   `json:"available"`
	OK        bool   `json:"ok"`
	Src       string `json:"src"` // live / api / rollout / cache
	Short     Window `json:"short"`
	Week      Window `json:"week"`
	Limit     string `json:"limit"`
	Plan      string `json:"plan"`
	Model     string `json:"model"`
	Effort    string `json:"effort"`
	AgeMin    int    `json:"ageMin"` // -1 = 모름, 0 = 방금
	Extra     string `json:"extra"`
}

func emptyAgent() AgentUsage {
	return AgentUsage{
		Short:  Window{Left: -1, Label: "5h"},
		Week:   Window{Left: -1, Label: "7d"},
		AgeMin: -1,
	}
}

var httpClient = &http.Client{Timeout: 10 * time.Second}

func getJSON(url string, headers map[string]string, out interface{}) (int, error) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return resp.StatusCode, err
	}
	if resp.StatusCode != 200 {
		return resp.StatusCode, fmt.Errorf("http %d", resp.StatusCode)
	}
	return 200, json.Unmarshal(body, out)
}

// curl 폴백 — 토큰은 명령줄이 아니라 설정을 stdin 으로 넘겨 프로세스 목록에 노출되지 않게 한다.
func curlJSON(url string, headers map[string]string, ua string, out interface{}) error {
	curl, err := exec.LookPath("curl")
	if err != nil {
		return fmt.Errorf("curl 없음: %w", err)
	}
	var cfg strings.Builder
	fmt.Fprintf(&cfg, "url = \"%s\"\nsilent\nmax-time = 10\n", url)
	for k, v := range headers {
		fmt.Fprintf(&cfg, "header = \"%s: %s\"\n", k, v)
	}
	if ua != "" {
		fmt.Fprintf(&cfg, "user-agent = \"%s\"\n", ua)
	}
	cmd := exec.Command(curl, "-K", "-")
	cmd.Stdin = strings.NewReader(cfg.String())
	hideWindow(cmd) // Windows 에서 콘솔 창이 번쩍이지 않게
	body, err := cmd.Output()
	if err != nil {
		return err
	}
	b := strings.TrimSpace(string(body))
	if !strings.HasPrefix(b, "{") {
		if len(b) > 80 {
			b = b[:80]
		}
		return fmt.Errorf("json 아님: %s", b)
	}
	return json.Unmarshal([]byte(b), out)
}

// ---------------------------------------------------------------- Claude

type claudeUsageResp struct {
	FiveHour struct {
		Utilization *float64 `json:"utilization"`
		ResetsAt    string   `json:"resets_at"`
	} `json:"five_hour"`
	SevenDay struct {
		Utilization *float64 `json:"utilization"`
		ResetsAt    string   `json:"resets_at"`
	} `json:"seven_day"`
	ExtraUsage struct {
		IsEnabled   bool     `json:"is_enabled"`
		Utilization *float64 `json:"utilization"`
	} `json:"extra_usage"`
}

func parseISO(s string) int64 {
	if s == "" {
		return 0
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05Z0700", "2006-01-02T15:04:05"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.Unix()
		}
	}
	return 0
}

func leftFrom(util *float64) int {
	if util == nil {
		return -1
	}
	l := 100 - int(math.Round(*util))
	if l < 0 {
		l = 0
	}
	return l
}

func fetchClaude() AgentUsage {
	u := emptyAgent()
	u.Available = true
	tok := claudeToken()
	if tok == "" {
		logf("claude: 토큰 없음")
		return u
	}
	var r claudeUsageResp
	_, err := getJSON("https://api.anthropic.com/api/oauth/usage", map[string]string{
		"Accept":         "application/json",
		"Authorization":  "Bearer " + tok,
		"anthropic-beta": "oauth-2025-04-20",
		"User-Agent":     "usage-tray/1.0",
	}, &r)
	if err != nil {
		logf("claude api 실패: %v", err)
		return u
	}
	u.Src = "api"
	u.Short = Window{Left: leftFrom(r.FiveHour.Utilization), ResetAt: parseISO(r.FiveHour.ResetsAt), WindowS: 5 * 3600, Label: "5h"}
	u.Week = Window{Left: leftFrom(r.SevenDay.Utilization), ResetAt: parseISO(r.SevenDay.ResetsAt), WindowS: 7 * 86400, Label: "7d"}
	if r.ExtraUsage.IsEnabled && r.ExtraUsage.Utilization != nil {
		u.Extra = fmt.Sprintf("extra %d%%", int(*r.ExtraUsage.Utilization))
	}
	u.OK = u.Short.Left >= 0 || u.Week.Left >= 0
	u.AgeMin = 0
	return u
}

// ---------------------------------------------------------------- Codex

type codexWindow struct {
	UsedPercent   *float64 `json:"used_percent"`
	WindowSeconds int64    `json:"limit_window_seconds"`
	ResetAt       int64    `json:"reset_at"`
}

type codexUsageResp struct {
	PlanType  string `json:"plan_type"`
	RateLimit struct {
		Allowed      bool         `json:"allowed"`
		LimitReached bool         `json:"limit_reached"`
		Primary      *codexWindow `json:"primary_window"`
		Secondary    *codexWindow `json:"secondary_window"`
	} `json:"rate_limit"`
	ReachedType *string `json:"rate_limit_reached_type"`
}

func fetchCodex() AgentUsage {
	u := emptyAgent()
	u.Available = true
	tok, acct := codexToken()
	if tok == "" {
		logf("codex: 토큰 없음")
		return u
	}
	const url = "https://chatgpt.com/backend-api/codex/usage"
	const ua = "codex_cli_rs/0.0.0"
	headers := map[string]string{
		"Accept":             "application/json",
		"Authorization":      "Bearer " + tok,
		"chatgpt-account-id": acct,
		"originator":         "codex_cli_rs",
		"User-Agent":         ua,
	}
	var r codexUsageResp
	code, err := getJSON(url, headers, &r)
	if err != nil {
		// Cloudflare 가 막는 경우가 있다 — 완전히 다른 스택(curl)으로 한 번 더.
		logf("codex api %d: %v — curl 로 재시도", code, err)
		if cerr := curlJSON(url, headers, ua, &r); cerr != nil {
			logf("codex curl 도 실패: %v", cerr)
			return codexFromRollout(u)
		}
	}
	u.Src = "live"
	u.Plan = r.PlanType
	if r.RateLimit.LimitReached {
		u.Limit = "rate_limit"
	}
	if r.ReachedType != nil && *r.ReachedType != "" {
		u.Limit = *r.ReachedType
	}
	// 창은 primary/secondary 이름이 아니라 길이로 분류한다 — 계정마다 뜻이 다르다.
	for _, w := range []*codexWindow{r.RateLimit.Primary, r.RateLimit.Secondary} {
		if w == nil || w.UsedPercent == nil {
			continue
		}
		win := Window{Left: leftFrom(w.UsedPercent), ResetAt: w.ResetAt, WindowS: w.WindowSeconds}
		if w.WindowSeconds >= 2*86400 {
			win.Label = "7d"
			u.Week = win
		} else {
			win.Label = "5h"
			u.Short = win
		}
	}
	u.OK = u.Week.Left >= 0 || u.Short.Left >= 0 || u.Limit != ""
	u.AgeMin = 0
	u.Model, u.Effort = codexConfigDefaults()
	return u
}

func codexConfigDefaults() (model, effort string) {
	b, err := os.ReadFile(codexConfigPath())
	if err != nil {
		return "", ""
	}
	for _, line := range strings.Split(string(b), "\n") {
		s := strings.TrimSpace(line)
		if strings.HasPrefix(s, "[") {
			break
		}
		if v, ok := tomlString(s, "model"); ok && model == "" {
			model = v
		}
		if v, ok := tomlString(s, "model_reasoning_effort"); ok && effort == "" {
			effort = v
		}
	}
	return model, effort
}

func tomlString(line, key string) (string, bool) {
	if !strings.HasPrefix(line, key) {
		return "", false
	}
	rest := strings.TrimSpace(strings.TrimPrefix(line, key))
	if !strings.HasPrefix(rest, "=") {
		return "", false
	}
	rest = strings.TrimSpace(strings.TrimPrefix(rest, "="))
	if len(rest) < 2 || rest[0] != '"' {
		return "", false
	}
	if i := strings.IndexByte(rest[1:], '"'); i >= 0 {
		return rest[1 : 1+i], true
	}
	return "", false
}
