package main

// usage-tray — Claude Code · Codex 사용량을 트레이(Windows)·메뉴바(macOS)에서 본다.
//
//	아이콘  : 링 게이지. 링 길이 = 주간 남은 %, 색 = 브랜드, 20% 미만이면 빨강.
//	툴팁    : 5h/7d 남은량과 리셋 시각, 출처.
//	상세    : 메뉴 → 상세 보기. 127.0.0.1 의 HTML 한 장을 브라우저로 연다.
//	알림    : 임계값을 처음 넘을 때 한 번. 회복하면 다시 무장한다.
//
// 자격증명이 있는 에이전트만 낸다. 둘 다 없으면 아무것도 띄우지 않고 끝낸다.
//
// 단일 인스턴스는 고정 포트 점유로 판정한다 — 뮤텍스보다 이식성이 좋고, 그 포트가 곧 상세 화면이다.

import (
	"encoding/json"
	"flag"
	"fmt"
	"image/color"
	"maps"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"fyne.io/systray"
)

const (
	tickInterval = 60 * time.Second
	tokenEvery   = 5 * time.Minute
)

// 에이전트마다 프로세스를 하나씩 띄운다 — 트레이 API 가 프로세스당 아이콘 1개만 주기 때문이다.
// 포트도 상태 파일도 에이전트별로 갈라야 서로 밟지 않는다.
var ports = map[string]string{"claude": "127.0.0.1:47113", "codex": "127.0.0.1:47114", "antigravity": "127.0.0.1:47115", "": "127.0.0.1:47113"}

// 이 프로세스가 맡은 에이전트("claude" / "codex" / "" = 둘 다 한 창에).
func tag() string {
	if only != nil {
		return *only
	}
	return ""
}

func suffix() string {
	if s := tag(); s != "" {
		return "-" + s
	}
	return ""
}

func listenAddr() string { return ports[tag()] }

type State struct {
	Updated     time.Time  `json:"updated"`
	Antigravity AgentUsage `json:"antigravity"`
	Claude      AgentUsage `json:"claude"`
	Codex       AgentUsage `json:"codex"`
	Summary     string     `json:"summary"`
	Agents      struct {
		Antigravity bool `json:"antigravity"`
		Claude      bool `json:"claude"`
		Codex       bool `json:"codex"`
	} `json:"agents"`
	Alerts map[string]bool `json:"alerts"`
}

var (
	only    *string
	mu      sync.RWMutex
	state   = &State{Alerts: map[string]bool{}}
	tokens  *TokenStats
	agents  struct{ claude, codex, antigravity bool }
	logFile = filepath.Join(dataDir(), "usage-tray.log")
)

func statePath() string { return filepath.Join(dataDir(), "state"+suffix()+".json") }
func textPath() string  { return filepath.Join(dataDir(), "status"+suffix()+".txt") }

func logf(format string, a ...interface{}) {
	line := time.Now().Format("2006-01-02T15:04:05") + " " + fmt.Sprintf(format, a...) + "\n"
	f, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(line)
}

func main() {
	once := flag.Bool("once", false, "한 번 수집해 한 줄 출력하고 끝낸다")
	scan := flag.Bool("tokens", false, "토큰만 스캔해 결과를 출력하고 끝낸다")
	// 둘 다 쓰지만 한쪽만 보고 싶을 때, 그리고 단일 에이전트 화면을 확인할 때.
	only = flag.String("only", "", "claude, codex 또는 antigravity 만 다룬다")
	icons := flag.String("icons", "", "아이콘 견본을 이 폴더에 PNG/ICO 로 떨어뜨리고 끝낸다(확인용)")
	testNotify := flag.Bool("notify", false, "알림을 한 번 띄워 보고 끝낸다(확인용)")
	promote := flag.Bool("promote", false, "트레이 아이콘을 작업표시줄에 항상 보이게 하고 끝낸다(윈도우)")
	restartExplorer := flag.Bool("restart-explorer", false, "-promote 와 함께 쓰면 explorer 를 재시작해 바로 적용한다")
	install := flag.Bool("install", false, "로그인 자동 실행을 걸고, 아이콘을 꺼내고, 지금 띄운다")
	uninstall := flag.Bool("uninstall", false, "-install 로 건 자동 실행을 뗀다")
	flag.Parse()
	if _, ok := ports[tag()]; !ok {
		fmt.Fprintln(os.Stderr, "지원하지 않는 -only 값")
		os.Exit(2)
	}

	if *install {
		installStartup()
		return
	}
	if *uninstall {
		uninstallStartup()
		return
	}
	if *promote {
		promoteTrayIcons(*restartExplorer)
		return
	}

	if *icons != "" {
		dumpIcons(*icons)
		return
	}
	if *testNotify {
		notify("usage-tray 알림 확인", "이렇게 보인다면 임계값 알림도 뜬다")
		time.Sleep(3 * time.Second) // 알림을 띄우는 자식 프로세스가 시작할 틈을 준다
		return
	}

	agents.claude, agents.codex = detectAgents()
	agents.antigravity = detectAntigravity()
	switch *only {
	case "claude":
		agents.antigravity = false
		agents.codex = false
	case "antigravity":
		agents.claude, agents.codex = false, false
	case "codex":
		agents.antigravity = false
		agents.claude = false
	}
	logf("agents: claude=%v codex=%v", agents.claude, agents.codex)

	if *scan {
		t := scanTokens(agents.claude, agents.codex)
		b, _ := json.MarshalIndent(t, "", " ")
		fmt.Println(string(b))
		return
	}

	if !agents.claude && !agents.codex && !agents.antigravity {
		fmt.Println("쓸 수 있는 에이전트가 없다 (Claude Code · Codex 자격증명 없음)")
		logf("no agent credentials found; exit")
		return
	}

	// 여럿을 쓰는데 -only 가 없으면, 에이전트별로 자식을 하나씩 띄우고 이 프로세스는 빠진다.
	// 아이콘이 갈라지는 지점이 여기다. 맥은 이렇게 하면 메뉴바에 아무것도 안 뜨므로 한
	// 프로세스로 간다 — splitPerAgent 주석에 이유가 있다.
	// 수집(refresh)보다 먼저 해야 부모가 상태 파일을 남기지 않는다 — 접미사 없는 state.json
	// 은 한쪽만 쓰는 사람의 파일이다.
	if tag() == "" && !*once && splitPerAgent() {
		if kids := activeAgents(); len(kids) > 1 {
			spawnChildren(kids)
			return
		}
	}

	refresh()
	if *once {
		mu.RLock()
		s := *state
		mu.RUnlock()
		fmt.Println(s.Summary)
		// 메뉴에 뜨는 그대로 — 눈으로 확인할 때 쓴다.
		for _, l := range ownLines(&s) {
			fmt.Println("  " + l)
		}
		// 트레이 툴팁은 63자를 못 넘어 메뉴보다 짧다. 실제로 뜨는 문자열과 길이를 같이 낸다.
		tip := tooltip(&s)
		fmt.Printf("툴팁 %d자 (한계 %d)\n", len([]rune(tip)), tipLimit)
		for _, l := range strings.Split(tip, "\n") {
			fmt.Println("  | " + l)
		}
		return
	}

	ln, err := net.Listen("tcp", listenAddr())
	if err != nil {
		fmt.Println("이미 실행 중이다 —", listenAddr(), "사용 중")
		logf("already running (%s); exit", listenAddr())
		return
	}
	go serve(ln)
	go func(claude, codex bool) {
		t := scanTokens(claude, codex)
		mu.Lock()
		defer mu.Unlock()
		tokens = t
	}(agents.claude, agents.codex)

	systray.Run(onReady, func() { logf("exit") })
}

func serve(ln net.Listener) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		s := mergedState()
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, renderPage(&s, mergedTokens()))
	})
	mux.HandleFunc("/state.json", func(w http.ResponseWriter, r *http.Request) {
		s := mergedState()
		b, _ := json.MarshalIndent(s, "", " ")
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write(b)
	})
	_ = http.Serve(ln, mux)
}

// ---------------------------------------------------------------- 수집

// 조회가 실패했을 때 화면을 비우지 않는다 — 직전 정상값을 그대로 두고 나이만 붙인다.
// 간헐적인 429·네트워크 오류에 아이콘이 회색 '값 없음' 으로 깜빡이는 걸 막는 장치다.
func cachedUsage(u AgentUsage) AgentUsage {
	if !u.FetchedAt.IsZero() {
		u.AgeMin = int(time.Since(u.FetchedAt).Minutes())
	}
	return u
}
func keepLastGood(cur, prev AgentUsage, prevAt time.Time) AgentUsage {
	if cur.OK || !prev.OK || prevAt.IsZero() {
		return cur
	}
	out := prev
	out.Src = "직전값"
	if prev.FetchedAt.IsZero() {
		prev.FetchedAt = prevAt
	}
	out.FetchedAt = prev.FetchedAt
	out.AgeMin = int(time.Since(prev.FetchedAt).Minutes())
	out.Error = cur.Error
	return out
}

var lastFetch struct{ claude, codex, antigravity time.Time }

func refresh() {
	mu.RLock()
	prevState := *state
	mu.RUnlock()

	c, x, a := cachedUsage(prevState.Claude), cachedUsage(prevState.Codex), cachedUsage(prevState.Antigravity)
	if agents.claude {
		// 최소 간격을 지킨다. 그 사이에는 직전값을 그대로 쓴다(아래 keepLastGood).
		if time.Since(lastFetch.claude) >= claudePollMin {
			lastFetch.claude = time.Now()
			c = fetchClaude()
		}
		c = keepLastGood(c, prevState.Claude, prevState.Updated)
	}
	if agents.codex {
		if time.Since(lastFetch.codex) >= codexPollMin {
			lastFetch.codex = time.Now()
			x = fetchCodex()
		}
		x = keepLastGood(x, prevState.Codex, prevState.Updated)
	}
	if agents.antigravity {
		if time.Since(lastFetch.antigravity) >= 2*time.Minute {
			lastFetch.antigravity = time.Now()
			a = fetchAntigravity()
		}
		a = keepLastGood(a, prevState.Antigravity, prevState.Updated)
	}
	mu.Lock()
	prev := state.Alerts
	if prev == nil {
		prev = map[string]bool{}
	}
	state = &State{Updated: time.Now(), Claude: c, Codex: x, Antigravity: a, Alerts: prev}
	state.Agents.Antigravity = agents.antigravity
	state.Agents.Claude = agents.claude
	state.Agents.Codex = agents.codex
	state.Summary = summarize(state)
	snapshot := *state
	mu.Unlock()

	writeState(&snapshot)
	checkAlerts(&snapshot)
}

func summarize(s *State) string {
	var parts []string
	if s.Claude.Available {
		var p []string
		if s.Claude.Week.Left >= 0 {
			p = append(p, fmt.Sprintf("7d %d%%", s.Claude.Week.Left))
		}
		if s.Claude.Short.Left >= 0 {
			p = append(p, fmt.Sprintf("5h %d%%", s.Claude.Short.Left))
		}
		if len(p) > 0 {
			parts = append(parts, "claude "+strings.Join(p, " "))
		}
	}
	if s.Codex.Available {
		var p []string
		if s.Codex.Week.Left >= 0 {
			p = append(p, fmt.Sprintf("7d %d%%", s.Codex.Week.Left))
		}
		if s.Codex.Short.Left >= 0 {
			p = append(p, fmt.Sprintf("5h %d%%", s.Codex.Short.Left))
		}
		if s.Codex.Src != "live" && s.Codex.AgeMin >= 10 {
			p = append(p, "~"+fmtAge(s.Codex.AgeMin))
		}
		if s.Codex.Limit != "" {
			p = append(p, "!limit")
		}
		if len(p) > 0 {
			parts = append(parts, "codex "+strings.Join(p, " "))
		}
	}
	if s.Antigravity.Available {
		if s.Antigravity.Short.Left >= 0 {
			parts = append(parts, fmt.Sprintf("antigravity 최소 %d%%", s.Antigravity.Short.Left))
		} else {
			parts = append(parts, "antigravity 조회 대기")
		}
	}
	return strings.Join(parts, " | ")
}

func writeState(s *State) {
	if b, err := json.MarshalIndent(s, "", " "); err == nil {
		_ = os.WriteFile(statePath(), b, 0o644)
	}
	_ = os.WriteFile(textPath(), []byte(s.Summary), 0o644)
}

// 상세 화면은 두 프로세스의 결과를 합쳐 보여 준다 — 어느 아이콘을 눌러도 같은 화면이 뜨게.
// 내 몫은 메모리에서(가장 신선하다), 옆 프로세스 몫은 그쪽이 쓴 파일에서 읽는다.
func mergedState() State {
	mu.RLock()
	s := *state
	mu.RUnlock()
	for _, other := range []string{"claude", "codex"} {
		if tag() == "" {
			break
		}
		if other == tag() {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dataDir(), "state-"+other+".json"))
		if err != nil {
			continue
		}
		var o State
		if json.Unmarshal(b, &o) != nil {
			continue
		}
		// 30분 넘게 갱신이 없으면 그 프로세스는 죽은 것으로 본다.
		if time.Since(o.Updated) > 30*time.Minute {
			continue
		}
		if other == "claude" && o.Claude.Available {
			s.Claude = o.Claude
			s.Agents.Claude = true
		}
		if other == "codex" && o.Codex.Available {
			s.Codex = o.Codex
			s.Agents.Codex = true
		}
	}
	// 요약도 합친 값으로 다시 만든다 — 안 그러면 /state.json 이 제 몫만 말한다.
	s.Summary = summarize(&s)
	return s
}

func mergedTokens() *TokenStats {
	mu.RLock()
	t := tokens
	mu.RUnlock()
	out := &TokenStats{}
	if t != nil {
		*out = *t
	}
	for _, other := range []string{"claude", "codex"} {
		if tag() == "" {
			break
		}
		if other == tag() {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dataDir(), "tokens-"+other+".json"))
		if err != nil {
			continue
		}
		var o TokenStats
		if json.Unmarshal(b, &o) != nil {
			continue
		}
		if other == "claude" {
			out.Claude = o.Claude
		} else {
			out.Codex = o.Codex
		}
		if o.Updated.After(out.Updated) {
			out.Updated, out.ScanMs = o.Updated, o.ScanMs
		}
	}
	if out.Updated.IsZero() {
		return nil
	}
	return out
}

// ---------------------------------------------------------------- 알림

type threshold struct {
	key   string
	label string
	fire  int
	rearm int
	left  func(*State) int
	on    func(*State) bool
}

var thresholds = []threshold{
	{"claude5h", "Claude 5시간", 10, 20, func(s *State) int { return s.Claude.Short.Left }, func(s *State) bool { return s.Claude.Available }},
	{"claude7d", "Claude 주간", 20, 30, func(s *State) int { return s.Claude.Week.Left }, func(s *State) bool { return s.Claude.Available }},
	{"codex7d", "Codex 주간", 20, 30, func(s *State) int { return s.Codex.Week.Left }, func(s *State) bool { return s.Codex.Available }},
}

func checkAlerts(s *State) {
	s.Alerts = maps.Clone(s.Alerts)
	changed := false
	for _, t := range thresholds {
		if !t.on(s) {
			continue
		}
		v := t.left(s)
		if v < 0 {
			continue
		}
		fired := s.Alerts[t.key]
		switch {
		case !fired && v < t.fire:
			s.Alerts[t.key] = true
			changed = true
			notify(t.label+" 한도 임박", fmt.Sprintf("%d%% 남음", v))
		case fired && v >= t.rearm:
			// 창이 초기화돼 다시 여유가 생긴 순간. 임박 알림을 띄웠던 창만 알린다 —
			// 안 띄웠으면 사용자는 애초에 기다리고 있지 않았다.
			s.Alerts[t.key] = false
			changed = true
			notify(t.label+" 초기화", fmt.Sprintf("%d%% 남았다 — 다시 쓸 수 있다", v))
		}
	}
	if s.Codex.Available {
		lim := s.Codex.Limit
		fired := s.Alerts["codexLimit"]
		if lim != "" && !fired {
			s.Alerts["codexLimit"] = true
			changed = true
			notify("Codex 한도 도달", lim)
		} else if lim == "" && fired {
			s.Alerts["codexLimit"] = false
			changed = true
			notify("Codex 한도 해제", "다시 쓸 수 있다")
		}
	}
	if changed {
		mu.Lock()
		state.Alerts = s.Alerts
		snapshot := *state
		mu.Unlock()
		writeState(&snapshot)
	}
}

// ---------------------------------------------------------------- 트레이

func onReady() {
	// 맡은 에이전트의 창을 줄마다 하나씩. 클릭 대상이 아니므로 비활성으로 둔다.
	infoItems := []*systray.MenuItem{}
	for i := 0; i < 14; i++ {
		it := systray.AddMenuItem(" ", "")
		it.Disable()
		it.Hide()
		infoItems = append(infoItems, it)
	}
	systray.AddSeparator()
	mDetail := systray.AddMenuItem("상세 보기", "브라우저로 상세 화면을 연다")
	mRefresh := systray.AddMenuItem("지금 갱신", "")
	mCopy := systray.AddMenuItem("요약 복사", "")
	mFolder := systray.AddMenuItem("폴더 열기", "")
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("끝내기", "")

	// 두 에이전트를 한 아이콘에 담을 수 없으므로(systray 는 아이콘 1개다)
	// 주간 남은량이 더 적은 쪽을 아이콘에 띄우고, 둘 다 툴팁·메뉴·상세에 낸다.
	paint := func() {
		mu.RLock()
		s := *state
		mu.RUnlock()
		left, brand := iconChoice(&s)
		systray.SetIcon(trayIcon(left, brand))
		systray.SetTitle("")
		systray.SetTooltip(tooltip(&s))
		lines := ownLines(&s)
		for i, it := range infoItems {
			if i < len(lines) {
				it.SetTitle(lines[i])
				it.Show()
			} else {
				it.Hide()
			}
		}
	}
	paint()

	go func() {
		tick := time.NewTicker(tickInterval)
		tokenTick := time.NewTicker(tokenEvery)
		defer tick.Stop()
		defer tokenTick.Stop()
		for {
			select {
			case <-tick.C:
				// 새로 로그인한 에이전트가 있으면 반영한다(끄는 것은 재시작 때만).
				// -only 로 일부러 끈 쪽은 되살리지 않는다.
				if c, x := detectAgents(); *only == "" && ((c && !agents.claude) || (x && !agents.codex)) {
					agents.claude = agents.claude || c
					agents.codex = agents.codex || x
					logf("agent appeared: claude=%v codex=%v", agents.claude, agents.codex)
				}
				if tag() == "" {
					agents.antigravity = agents.antigravity || detectAntigravity()
				}
				refresh()
				paint()
			case <-tokenTick.C:
				t := scanTokens(agents.claude, agents.codex)
				mu.Lock()
				tokens = t
				mu.Unlock()
			case <-mDetail.ClickedCh:
				openURL("http://" + listenAddr() + "/")
			case <-mRefresh.ClickedCh:
				if tag() == "" {
					agents.antigravity = agents.antigravity || detectAntigravity()
				}
				refresh()
				paint()
				t := scanTokens(agents.claude, agents.codex)
				mu.Lock()
				tokens = t
				mu.Unlock()
			case <-mCopy.ClickedCh:
				mu.RLock()
				sum := state.Summary
				mu.RUnlock()
				copyToClipboard(sum)
			case <-mFolder.ClickedCh:
				openPath(dataDir())
			case <-mQuit.ClickedCh:
				systray.Quit()
				return
			}
		}
	}()
}

// 아이콘 하나에 무엇을 띄우나 — 급한 쪽(남은량이 적은 쪽).
func iconChoice(s *State) (int, color.NRGBA) {
	best, brand := -1, colCodex
	for _, v := range []struct {
		u     AgentUsage
		color color.NRGBA
	}{{s.Claude, colClaude}, {s.Codex, colCodex}, {s.Antigravity, colAntigravity}} {
		if !v.u.Available {
			continue
		}
		left := v.u.Week.Left
		if left < 0 {
			left = v.u.Short.Left
		}
		if best < 0 || (left >= 0 && left < best) {
			best, brand = left, v.color
		}
	}
	return best, brand
}

// 한 줄의 상세도.
//
// detailFull 은 메뉴용이다 — 고정폭 글꼴에 길이 제한이 없으니 자리맞춤까지 넣어 가장 자세히.
// 나머지는 툴팁용이고, 63자 한계(tipLimit 주석) 때문에 처음부터 축약 기준으로 시작한다.
// 남는 자리가 있어도 상세판으로 올리지 않는다 — 창이 하나인 Codex 만 상세판이 되어 Claude 와
// 생김새가 달라지는 문제가 있었다(2026-09-18). 두 툴팁이 같아 보이는 쪽을 택했다.
// "(6일 뒤)" 는 어느 단계에서도 지키고, 자리가 없으면 시각 → 날짜 순으로 버린다.
type lineDetail int

const (
	detailFull       lineDetail = iota // 메뉴:   주간   93% 남음 · 리셋 9/25(금) 08:00 (6일 뒤)
	detailTip                          // 툴팁:   주간 93% · 9/25 08:00 (6일 뒤)
	detailTipNoClock                   //         주간 93% · 9/25 (6일 뒤)
	detailTipETA                       //         주간 93% · 6일 뒤
	detailTipBare                      //         주간 93%
)

// 툴팁·메뉴에 쓰는 한 줄. "주간  100% 남음 · 리셋 9/25(금) 08:00 (7일 뒤)"
// 왼쪽을 창 이름으로 맞추고 %를 세 자리 폭으로 채워 위아래가 눈에 정렬돼 보이게 한다.
func windowLine(label string, w Window) string { return windowLineAt(label, w, detailFull) }

func windowLineAt(label string, w Window, d lineDetail) string {
	if w.Left < 0 {
		if d == detailFull {
			return fmt.Sprintf("%-5s  값 없음", label)
		}
		return label + " 값 없음"
	}

	// 메뉴판 — 자리맞춤(%-5s)은 고정폭으로 보는 메뉴에서만 쓸모가 있다.
	if d == detailFull {
		s := fmt.Sprintf("%-5s %3d%% 남음", label, w.Left)
		if w.ResetAt > 0 {
			s += " · 리셋 " + fmtResetKo(w.ResetAt, true)
			if u := fmtUntil(w.ResetAt); u != "" {
				s += " (" + u + ")"
			}
		}
		return s
	}

	// 툴팁판 — 자리맞춤 · "남음" · "리셋" · 요일을 빼고 남은 자리를 시각과 "N일 뒤" 에 쓴다.
	s := fmt.Sprintf("%s %d%%", label, w.Left)
	if w.ResetAt <= 0 || d == detailTipBare {
		return s
	}
	eta := fmtUntil(w.ResetAt)
	switch d {
	case detailTipETA:
		if eta == "" {
			return s + " · " + fmtResetDay(w.ResetAt)
		}
		return s + " · " + eta
	case detailTipNoClock:
		s += " · " + fmtResetDay(w.ResetAt)
	default:
		s += " · " + fmtResetKo(w.ResetAt, false)
	}
	if eta != "" {
		s += " (" + eta + ")"
	}
	return s
}

func windowName(w Window, fallback string) string {
	switch {
	case w.WindowS >= 2*86400:
		return "주간"
	case w.WindowS > 0:
		return fmt.Sprintf("%d시간", w.WindowS/3600)
	}
	return fallback
}

// 이 프로세스가 맡은 에이전트만 자세히 낸다 — 아이콘이 에이전트별로 갈렸으므로
// 툴팁에 남의 값을 섞으면 오히려 읽기 어렵다.
func agentLines(name string, a *AgentUsage) []string {
	return agentLinesAt(name, a, detailFull)
}

func agentLinesAt(name string, a *AgentUsage, d lineDetail) []string {
	lines := []string{name}
	if a.Week.Left >= 0 || a.Week.ResetAt > 0 {
		lines = append(lines, windowLineAt(windowName(a.Week, "주간"), a.Week, d))
	}
	if a.Short.Left >= 0 || a.Short.ResetAt > 0 {
		lines = append(lines, windowLineAt(windowName(a.Short, a.Short.Label), a.Short, d))
	}
	var notes []string
	if a.Error != "" && d == detailFull {
		notes = append(notes, a.Error)
	}
	if a.Limit != "" {
		notes = append(notes, "한도 도달")
	}
	switch {
	// 갓 받은 캐시는 live 와 다를 게 없다 — 굳이 알리지 않는다. 묵은 것만 밝힌다.
	case a.Src == "앱 기록":
		notes = append(notes, "앱 기록 "+fmtAge(a.AgeMin)+" 전")
	case a.Src == "직전값":
		notes = append(notes, "직전값 "+fmtAge(a.AgeMin)+" 전")
	case strings.HasPrefix(a.Src, "캐시") && a.AgeMin >= 2:
		notes = append(notes, a.Src)
	case a.Src == "rollout" && a.AgeMin >= 0:
		notes = append(notes, "기록 "+fmtAge(a.AgeMin)+" 전")
	}
	if !a.OK && a.Src == "" {
		notes = append(notes, "값을 못 받았다")
	}
	if len(notes) > 0 {
		lines = append(lines, strings.Join(notes, " · "))
	}
	return lines
}

func ownLines(s *State) []string { return ownLinesAt(s, detailFull) }

func ownLinesAt(s *State, d lineDetail) []string {
	var lines []string
	for _, v := range []struct {
		name string
		u    *AgentUsage
	}{{"Claude", &s.Claude}, {"Codex", &s.Codex}, {"Antigravity", &s.Antigravity}} {
		if v.u.Available {
			lines = append(lines, agentLinesAt(v.name, v.u, d)...)
		}
	}
	return lines
}

// 트레이 툴팁이 실제로 담는 길이. szTip 필드는 128 WCHAR 이지만 셸에 NOTIFYICON_VERSION_4
// 를 알리지 않으면 64자(63 + 종료 null)만 쓴다 — systray 가 그 상태라서, 126자로 자르던
// 예전 코드는 아무것도 안 하고 셸이 문장 중간을 잘라 버렸다("5시간  48% 남음 · 리").
// 그래서 여유 1자를 둔 62자에 맞춰 우리가 먼저 줄인다.
const tipLimit = 62

func tooltip(s *State) string {
	// 축약 기준(detailTip)에서 시작해 62자에 드는 첫 단계를 쓴다 — 두 에이전트가 같은 형식이다.
	for d := detailTip; d <= detailTipBare; d++ {
		t := strings.Join(ownLinesAt(s, d), "\n")
		if len([]rune(t)) <= tipLimit {
			return t
		}
	}
	t := strings.Join(ownLinesAt(s, detailTipBare), "\n")
	// 그래도 넘치면(비고가 길 때) 룬 단위로 자른다 — 바이트로 자르면 한글이 깨진다.
	if r := []rune(t); len(r) > tipLimit {
		t = string(r[:tipLimit-1]) + "…"
	}
	return t
}

// 자기 자신을 -only 로 두 번 띄운다. 부모는 바로 빠지므로 프로세스는 둘만 남는다.
// 지금 살아 있는 에이전트 이름. 자식을 띄울 때와 개수를 셀 때 쓴다.
func activeAgents() []string {
	var out []string
	if agents.claude {
		out = append(out, "claude")
	}
	if agents.codex {
		out = append(out, "codex")
	}
	if agents.antigravity {
		out = append(out, "antigravity")
	}
	return out
}

func spawnChildren(names []string) {
	exe, err := os.Executable()
	if err != nil {
		logf("자기 경로를 못 찾는다: %v", err)
		return
	}
	for _, a := range names {
		cmd := exec.Command(exe, "-only", a)
		hideWindow(cmd)
		if err := cmd.Start(); err != nil {
			logf("%s 인스턴스를 못 띄웠다: %v", a, err)
			continue
		}
		_ = cmd.Process.Release()
		logf("spawned: -only %s", a)
	}
}

// 아이콘이 실제로 어떻게 그려지는지 눈으로 확인할 때. 트레이 캡처가 안 되는 환경에서 쓴다.
func dumpIcons(dir string) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fmt.Println("폴더를 못 만든다:", err)
		return
	}
	for _, c := range []struct {
		name  string
		brand color.NRGBA
	}{{"claude", colClaude}, {"codex", colCodex}} {
		for _, left := range []int{-1, 0, 8, 26, 55, 90, 100} {
			for _, size := range []int{16, 22, 32, 44} {
				n := fmt.Sprintf("%s/%s-%d-%dpx.png", dir, c.name, left, size)
				_ = os.WriteFile(n, ringPNG(left, c.brand, size), 0o644)
			}
			_ = os.WriteFile(fmt.Sprintf("%s/%s-%d.ico", dir, c.name, left),
				pngToICO(ringPNG(left, c.brand, 32), 32), 0o644)
		}
	}
	fmt.Println("아이콘 견본을 썼다:", dir)
}
