package main

// 상세 화면 — 127.0.0.1 의 작은 HTTP 서버가 내는 HTML 한 장.
//
// 왜 네이티브 창이 아닌가: 트레이 라이브러리(systray)는 창을 만들지 못하고, 창을 쓰려면
// Windows 는 WinForms, macOS 는 Cocoa 로 따로 만들어야 한다(= 두 벌 유지). HTML 한 장이면
// 양쪽이 같고, 차트·표를 그리기도 훨씬 쉽다. 브라우저 탭으로 열린다.
//
// 링은 트레이 아이콘과 같은 코드로 그려 base64 PNG 로 심는다 — 화면과 아이콘이 어긋나지 않게.
//
// 2026-09-18 개편: "가독성이 떨어지고 칙칙하다 · 글자가 다 안 나온다" 는 지적을 받아 셋을 고쳤다.
//  1. 차트를 인라인 SVG 에서 HTML/CSS 막대로 바꿨다. SVG 는 viewBox(760)를 패널 폭(약 560)에
//     맞춰 줄이면 안의 text 까지 0.74 배로 같이 줄어 11px 눈금이 8px 로 뭉개졌다 — 글자가 안
//     보이던 원인이다. HTML 막대의 눈금·날짜는 진짜 DOM 텍스트라 어떤 폭에서도 지정한 크기다.
//  2. 명도차를 벌렸다. 배경 #18181b 과 카드 #202024 는 사실상 같은 색이라 카드 경계가 보이지
//     않았다. 배경은 더 어둡게 · 카드는 더 밝게 하고 경계선을 살렸다. 보조 텍스트도 한 단 밝게.
//  3. 숫자를 오른쪽 정렬 + tabular-nums 로 자리를 맞췄고, 리셋 시각을 사람이 읽는 표기로 바꿨다
//     (내일 08:00 · 18시간 뒤). 그 전에는 fri 08:00 이라 며칠인지 알 수 없었다 — 이미 있던
//     fmtResetHuman/fmtUntil 을 카드가 쓰지 않고 있었다.

import (
	"encoding/base64"
	"fmt"
	"html"
	"image/color"
	"strings"
	"time"
)

func fmtTokens(n int64) string {
	f := float64(n)
	switch {
	case f >= 1e9:
		return trimZero(fmt.Sprintf("%.1f", f/1e9)) + "B"
	case f >= 1e6:
		return trimZero(fmt.Sprintf("%.1f", f/1e6)) + "M"
	case f >= 1e3:
		return trimZero(fmt.Sprintf("%.1f", f/1e3)) + "k"
	}
	return fmt.Sprintf("%d", n)
}

func trimZero(s string) string { return strings.TrimSuffix(s, ".0") }

var weekdayKo = [...]string{"일", "월", "화", "수", "목", "금", "토"}

// 리셋 시각을 사람이 바로 읽게. 오늘/내일은 그렇게 쓰고, 그 밖은 9/25(금) 08:00.
func fmtResetHuman(unix int64) string { return fmtResetKo(unix, true) }

// withWeekday=false 면 요일을 뺀다 — 트레이 툴팁이 63자밖에 못 담아서 쓰는 축약판.
func fmtResetKo(unix int64, withWeekday bool) string {
	if unix <= 0 {
		return "?"
	}
	t := time.Unix(unix, 0).Local()
	now := time.Now().Local()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	days := int(t.Sub(today).Hours() / 24)
	switch days {
	case 0:
		return "오늘 " + t.Format("15:04")
	case 1:
		return "내일 " + t.Format("15:04")
	}
	if !withWeekday {
		return fmt.Sprintf("%d/%d %s", int(t.Month()), t.Day(), t.Format("15:04"))
	}
	return fmt.Sprintf("%d/%d(%s) %s", int(t.Month()), t.Day(), weekdayKo[int(t.Weekday())], t.Format("15:04"))
}

// 시각 없이 날짜만 — 오늘 · 내일 · 9/25. 툴팁 자리가 모자랄 때 쓴다.
func fmtResetDay(unix int64) string {
	if unix <= 0 {
		return "?"
	}
	t := time.Unix(unix, 0).Local()
	now := time.Now().Local()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	switch int(t.Sub(today).Hours() / 24) {
	case 0:
		return "오늘"
	case 1:
		return "내일"
	}
	return fmt.Sprintf("%d/%d", int(t.Month()), t.Day())
}

// 얼마나 남았나 — 5시간 창은 분까지, 주간 창은 일까지.
func fmtUntil(unix int64) string {
	if unix <= 0 {
		return ""
	}
	d := time.Until(time.Unix(unix, 0))
	if d <= 0 {
		return "곧"
	}
	switch {
	case d >= 48*time.Hour:
		return fmt.Sprintf("%d일 뒤", int(d.Hours()/24))
	case d >= time.Hour:
		return fmt.Sprintf("%d시간 뒤", int(d.Hours()))
	default:
		return fmt.Sprintf("%d분 뒤", int(d.Minutes()))
	}
}

func fmtAge(min int) string {
	if min < 0 {
		return ""
	}
	if min >= 60 {
		return fmt.Sprintf("%dh%02dm", min/60, min%60)
	}
	return fmt.Sprintf("%dm", min)
}

func ringIMG(left int, brand color.NRGBA) string {
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(bigRingPNG(left, brand))
}

// 플랜 문자열을 사람이 읽게 — API 값 self_serve_business_prolite 를 Business Prolite 로.
func prettyPlan(p string) string {
	p = strings.ReplaceAll(strings.TrimPrefix(p, "self_serve_"), "_", " ")
	words := strings.Fields(p)
	for i, w := range words {
		r := []rune(w)
		words[i] = strings.ToUpper(string(r[:1])) + string(r[1:])
	}
	return strings.Join(words, " ")
}

// 에이전트 마크 — 이름과 브랜드 색. 표 · 제목 · 요약이 모두 이걸 쓴다.
type agentMark struct {
	name  string
	brand color.NRGBA
}

var (
	markClaude = agentMark{"Claude", colClaude}
	markCodex  = agentMark{"Codex", colCodex}
)

// 두 에이전트의 마크를 인라인 SVG 로 그린다. 파일을 읽지 않으므로 단일 실행파일 그대로다.
//
// Claude Code — 스타버스트(긴 살 4 + 짧은 살 4).
// Codex — 겹친 타원 3개(꽃 모양).
// 실제 배포 로고 파일이 있으면 이 함수만 바꿔 끼우면 된다.
func agentSVG(name string) string {
	if name == "Codex" {
		return `<svg viewBox="0 0 24 24" aria-hidden="true">` +
			`<g transform="translate(12 12)" fill="none" stroke="currentColor" stroke-width="1.7">` +
			`<ellipse rx="3.7" ry="8.6"/><ellipse rx="3.7" ry="8.6" transform="rotate(60)"/>` +
			`<ellipse rx="3.7" ry="8.6" transform="rotate(120)"/></g></svg>`
	}
	// 몸통 · 좌우 귀 · 다리 둘을 한 path 로 잇고, 눈은 같은 path 의 부분경로로 둔다.
	// evenodd 라 몸통 안에 든 눈만 구멍이 되고(서로 겹치지 않게 좌표를 잡았다) 배지의
	// 반투명 배경이 비쳐 원본 이미지처럼 어두운 눈이 된다.
	return `<svg viewBox="0 0 24 24" aria-hidden="true">` +
		`<path fill="currentColor" fill-rule="evenodd" d="` +
		`M4 4h16v12.5H4Z` + // 몸통
		`M2 8h2v4H2Z` + // 왼쪽 귀
		`M20 8h2v4h-2Z` + // 오른쪽 귀
		`M5.5 16.5h2v3h-2Z` + // 왼쪽 다리
		`M15.5 16.5h2v3h-2Z` + // 오른쪽 다리
		`M6.5 6.5h1.6v3.6H6.5Z` + // 왼쪽 눈(구멍)
		`M15.9 6.5h1.6v3.6h-1.6Z` + // 오른쪽 눈(구멍)
		`"/></svg>`
}

// cls: "lg" 표의 큰 배지 · "sm" 제목·요약의 작은 마크.
func agentIcon(m agentMark, cls string) string {
	return fmt.Sprintf(`<span class="ic %s" style="--c:%d,%d,%d" title="%s">%s</span>`,
		cls, m.brand.R, m.brand.G, m.brand.B, html.EscapeString(m.name), agentSVG(m.name))
}

// 카드에 붙는 작은 꼬리표. Kind 는 "" 보통 · "bad" 경고 · "live" 실시간.
type chip struct {
	Text string
	Kind string
}

type gaugeCard struct {
	Title   string
	Left    int
	Brand   color.NRGBA
	ResetAt int64
	Chips   []chip
	Mark    agentMark
}

// 14일 막대 차트 — HTML/CSS. 텍스트가 축소되지 않는 것이 SVG 대신 쓰는 이유다.
func barChartHTML(days []DayRow, brand color.NRGBA, valueOf func(DayRow) int64) string {
	var max int64 = 1
	vals := make([]int64, len(days))
	for i, d := range days {
		vals[i] = valueOf(d)
		if vals[i] > max {
			max = vals[i]
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, `<div class="chart" style="--brand:rgb(%d,%d,%d)">`, brand.R, brand.G, brand.B)

	// 눈금 4줄 — 위에서 아래로. 라벨은 왼쪽 여백에 오른쪽 정렬로 앉힌다.
	b.WriteString(`<div class="ch-scale" aria-hidden="true">`)
	for k := 3; k >= 0; k-- {
		fmt.Fprintf(&b, `<div class="ch-line"><span>%s</span></div>`, fmtTokens(max*int64(k)/3))
	}
	b.WriteString(`</div><div class="ch-bars">`)

	for i, v := range vals {
		h := 0.0
		if v > 0 {
			h = float64(v) / float64(max) * 100
			if h < 1.2 {
				h = 1.2
			}
		}
		cls := "ch-bar"
		if i == len(vals)-1 {
			cls += " is-today"
		}
		// 날짜는 14일을 빠짐없이 적는다. 붙어 보이지 않게 앞의 0 을 떼어 9/18 꼴로 쓰고,
		// 칸이 정말 좁은 화면(휴대폰 폭)에서만 CSS 가 .sec 을 숨긴다.
		md := days[i].Day[5:] // "09-18"
		label := strings.TrimLeft(md[:2], "0") + "/" + md[3:]
		xcls := "ch-x"
		if i%2 == 0 && i != len(vals)-1 {
			xcls += " sec"
		}
		fmt.Fprintf(&b, `<div class="ch-col" title="%s · %s"><div class="ch-track">`+
			`<div class="%s" style="height:%.1f%%"></div></div><div class="%s">%s</div></div>`,
			html.EscapeString(days[i].Day), fmtTokens(v), cls, h, xcls, label)
	}

	b.WriteString(`</div></div>`)
	return b.String()
}

const pageCSS = `
:root{
  --bg:#0d0e12; --panel:#1b1d24; --panel2:#23252e; --line:#31343f;
  --tx:#f4f5f7; --sub:#c2c6d0; --dim:#8b909c;
  --warn:#fbbf24; --bad:#fb7185;
}
*{box-sizing:border-box}
body{margin:0 auto;max-width:1180px;background:var(--bg);color:var(--tx);
  font:14px/1.55 "Segoe UI","Malgun Gothic","Apple SD Gothic Neo",system-ui,sans-serif;padding:26px 24px 34px}
.num,em,.cl,.ch-line span,.ch-x{font-variant-numeric:tabular-nums}

header{display:flex;align-items:center;flex-wrap:wrap;gap:6px 12px;margin-bottom:16px}
h1{font-size:25px;letter-spacing:-.01em;margin:0}
.brand{display:inline-flex;align-items:center;gap:8px;margin-right:2px}
.meta{color:var(--dim);font-size:12px}
.live{display:inline-block;width:6px;height:6px;border-radius:50%;background:#4ade80;margin-right:5px}

.row{display:grid;gap:14px;margin-bottom:14px}
.row.cards{grid-template-columns:repeat(auto-fit,minmax(290px,1fr))}
.row.two{grid-template-columns:repeat(auto-fit,minmax(420px,1fr))}

.card,.panel,.stat{background:var(--panel);border:1px solid var(--line);border-radius:14px}
.card{padding:16px 18px;display:flex;gap:16px;align-items:center}
.card img{width:82px;height:82px;flex:none}
.cbody{min-width:0}
.ct{font-weight:700;font-size:13.5px;margin-bottom:2px;display:flex;align-items:center;gap:9px}
.cl{font-size:33px;font-weight:700;line-height:1.15}
.cl span{font-size:13px;color:var(--sub);font-weight:500;margin-left:4px}
.cl.warn{color:var(--warn)} .cl.bad{color:var(--bad)}
.cs{color:var(--sub);font-size:12.5px;margin-top:3px}
.chips{display:flex;flex-wrap:wrap;gap:5px;margin-top:8px}
.chip{background:var(--panel2);border:1px solid var(--line);color:var(--dim);
  font-size:11px;line-height:1;padding:4px 8px;border-radius:999px}
.chip.live{color:#86efac;border-color:#2c4a3a}
.chip.bad{color:var(--bad);border-color:#5b2b36}

.panel{padding:16px 18px}
.panel h2{font-size:13px;color:var(--sub);margin:0 0 14px;font-weight:700;
  display:flex;align-items:center;gap:9px}

.chart{position:relative;padding-left:58px}
.ch-scale{position:absolute;left:58px;right:0;top:0;height:152px;
  display:flex;flex-direction:column;justify-content:space-between;pointer-events:none}
.ch-line{position:relative;border-top:1px solid var(--line)}
.ch-line:not(:first-child){border-top-style:dashed}
.ch-line span{position:absolute;left:-58px;top:-9px;width:50px;text-align:right;
  font-size:11.5px;color:var(--dim)}
.ch-bars{position:relative;display:flex;gap:6px;height:176px}
.ch-col{flex:1;display:flex;flex-direction:column;min-width:0}
.ch-track{flex:1;display:flex;align-items:flex-end}
.ch-bar{width:100%;background:var(--brand);opacity:.5;border-radius:4px 4px 0 0;min-height:2px}
.ch-bar.is-today{opacity:1}
.ch-col:hover .ch-bar{opacity:.85}
.ch-x{height:24px;line-height:24px;text-align:center;font-size:11px;color:var(--dim);
  letter-spacing:-.2px;white-space:nowrap}
@media (max-width:700px){.ch-x.sec{visibility:hidden}}

.stats{display:grid;gap:12px;grid-template-columns:repeat(auto-fit,minmax(400px,1fr));margin-bottom:14px}
.stat{padding:12px 16px;display:flex;flex-wrap:wrap;align-items:center;gap:6px 18px}
.stat b{font-size:13px;display:flex;align-items:center;gap:9px}
.stat i{font-style:normal;color:var(--dim);font-size:12px}
.stat em{font-style:normal;color:var(--tx);font-weight:700;font-size:13px;margin-left:5px}

table{width:100%;border-collapse:collapse;font-size:13px}
th{text-align:left;color:var(--dim);font-weight:500;font-size:11.5px;
  padding:0 0 8px;border-bottom:1px solid var(--line)}
td{padding:9px 0;border-bottom:1px solid rgba(255,255,255,.045)}
tr:last-child td{border-bottom:0}
th.num,td.num{text-align:right;padding-left:14px}
td.num{color:var(--sub)}
td.name{max-width:220px}
td.name,td.plain{overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
td.who{width:1%;white-space:nowrap}
.mrow{display:flex;align-items:center;min-width:0}
.mt{overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.ic{display:inline-flex;align-items:center;justify-content:center;flex:none;color:rgb(var(--c))}
.ic svg{width:100%;height:100%;display:block}
.ic.lg{width:32px;height:32px;padding:4px;margin-right:12px;border-radius:9px;
  background:rgba(var(--c),.15);border:1px solid rgba(var(--c),.42)}
.ic.sm{width:30px;height:30px}
.ic.xl{width:46px;height:46px;padding:6px;border-radius:13px;
  background:rgba(var(--c),.15);border:1px solid rgba(var(--c),.42)}
.marks{display:inline-flex;gap:7px;align-items:center}
tfoot td{border-top:1px solid var(--line);border-bottom:0;padding-top:10px;font-weight:700;color:var(--tx)}
tfoot tr+tr td{border-top:0;padding-top:2px}
tfoot td.num{color:var(--tx)}
tfoot .sum{color:var(--sub);font-weight:500;font-size:12px}

footer{color:var(--dim);font-size:11.5px;margin-top:18px}
.none{color:var(--sub);font-size:13px}
@media (max-width:620px){
  body{padding:18px 14px 26px}
  .card{gap:12px;padding:14px} .card img{width:64px;height:64px} .cl{font-size:28px}
}
`

func renderPage(s *State, tok *TokenStats) string {
	var b strings.Builder
	b.WriteString(`<!doctype html><html lang="ko"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<meta name="color-scheme" content="dark">
<meta http-equiv="refresh" content="30">
<title>사용량</title><style>` + pageCSS + `</style></head><body>`)

	// 최상단 — 쓰는 에이전트의 마크를 크게 세우고 그 옆에 제목·갱신 시각.
	brand := ""
	if s.Claude.Available {
		brand += agentIcon(markClaude, "xl")
	}
	if s.Codex.Available {
		brand += agentIcon(markCodex, "xl")
	}
	fmt.Fprintf(&b, `<header><span class="brand">%s</span><h1>사용량</h1>`+
		`<div class="meta"><span class="live"></span>%s 갱신 · 30초마다</div></header>`,
		brand, s.Updated.Local().Format("15:04:05"))

	b.WriteString(`<div class="row cards">`)
	for _, c := range gaugeCards(s) {
		b.WriteString(cardHTML(c))
	}
	b.WriteString(`</div>`)

	if tok == nil {
		b.WriteString(`<div class="panel none">토큰 집계가 아직 없다 — 첫 스캔이 끝나면 채워진다.</div></body></html>`)
		return b.String()
	}

	b.WriteString(`<div class="row two">`)
	if s.Claude.Available {
		fmt.Fprintf(&b, `<div class="panel"><h2>%sClaude · 14일 토큰 (입력+캐시+출력)</h2>%s</div>`,
			agentIcon(markClaude, "sm"),
			barChartHTML(tok.Claude.Days14, colClaude, func(d DayRow) int64 { return d.total() }))
	}
	if s.Codex.Available {
		fmt.Fprintf(&b, `<div class="panel"><h2>%sCodex · 14일 토큰 (입력+출력)</h2>%s</div>`,
			agentIcon(markCodex, "sm"),
			barChartHTML(tok.Codex.Days14, colCodex, func(d DayRow) int64 { return d.Input + d.Output }))
	}
	b.WriteString(`</div>`)

	b.WriteString(`<div class="stats">`)
	if s.Claude.Available {
		t := tok.Claude.Today
		b.WriteString(statHTML("Claude 오늘", markClaude, [][2]string{
			{"입력", fmtTokens(t.Input + t.CacheRead + t.CacheCreate)},
			{"출력", fmtTokens(t.Output)},
			{"요청", fmt.Sprintf("%d", t.Requests)},
			{"캐시 적중", fmt.Sprintf("%d%%", t.hitPct())},
		}))
	}
	if s.Codex.Available {
		t := tok.Codex.Today
		b.WriteString(statHTML("Codex 오늘", markCodex, [][2]string{
			{"입력", fmtTokens(t.Input)},
			{"출력", fmtTokens(t.Output)},
			{"요청", fmt.Sprintf("%d", t.Requests)},
			{"캐시", fmt.Sprintf("%d%%", t.hitPct())},
		}))
	}
	b.WriteString(`</div>`)

	b.WriteString(`<div class="row two">`)

	models := mergeModels(s, tok)
	b.WriteString(`<div class="panel"><h2>모델별 · 7일</h2><table>` +
		`<tr><th>모델</th><th class="num">요청</th><th class="num">입력</th>` +
		`<th class="num">출력</th><th class="num">캐시</th></tr>`)
	for _, m := range models {
		fmt.Fprintf(&b, `<tr><td class="name">%s</td><td class="num">%d</td>`+
			`<td class="num">%s</td><td class="num">%s</td><td class="num">%d%%</td></tr>`,
			modelCell(m), m.Requests,
			fmtTokens(m.Input+m.CacheCreate), fmtTokens(m.Output), m.hitPct())
	}
	b.WriteString(`<tfoot>`)
	for _, t := range agentTotals(s, tok, models) {
		fmt.Fprintf(&b, `<tr><td class="sum">%s</td><td class="num">%d</td>`+
			`<td class="num">%s</td><td class="num">%s</td><td class="num">%d%%</td></tr>`,
			html.EscapeString(t.label), t.Requests,
			fmtTokens(t.Input+t.CacheCreate), fmtTokens(t.Output), t.hitPct())
	}
	b.WriteString(`</tfoot></table></div>`)

	projects := mergeProjects(s, tok)
	b.WriteString(`<div class="panel"><h2>프로젝트별 · 7일</h2><table>` +
		`<tr><th>프로젝트</th><th></th><th class="num">요청</th><th class="num">토큰</th></tr>`)
	for _, p := range projects {
		fmt.Fprintf(&b, `<tr><td class="plain" title="%s">%s</td><td class="who">%s</td>`+
			`<td class="num">%d</td><td class="num">%s</td></tr>`,
			html.EscapeString(p.Name), html.EscapeString(p.Name),
			marksHTML(p.marks), p.Requests, fmtTokens(p.total()))
	}
	b.WriteString(`<tfoot>`)
	for _, t := range agentTotals(s, tok, projects) {
		fmt.Fprintf(&b, `<tr><td class="sum" colspan="2">%s</td><td class="num">%d</td>`+
			`<td class="num">%s</td></tr>`,
			html.EscapeString(t.label), t.Requests, fmtTokens(t.total()))
	}
	b.WriteString(`</tfoot></table></div></div>`)

	fmt.Fprintf(&b, `<footer>집계 %s · %dms · 출처 Claude %s / Codex %s</footer>`,
		tok.Updated.Local().Format("2006-01-02 15:04:05"), tok.ScanMs,
		orDash(s.Claude.Src), orDash(s.Codex.Src))

	b.WriteString(`</body></html>`)
	return b.String()
}

func gaugeCards(s *State) []gaugeCard {
	var cards []gaugeCard
	if s.Claude.Available {
		var src []chip
		if s.Claude.Src != "" && s.Claude.Src != "api" {
			src = []chip{{Text: s.Claude.Src}}
		}
		week := src
		if s.Claude.Extra != "" {
			week = append([]chip{{Text: s.Claude.Extra}}, src...)
		}
		cards = append(cards,
			gaugeCard{"Claude · 5시간", s.Claude.Short.Left, colClaude, s.Claude.Short.ResetAt, src, markClaude},
			gaugeCard{"Claude · 주간", s.Claude.Week.Left, colClaude, s.Claude.Week.ResetAt, week, markClaude})
	}
	if s.Codex.Available {
		var chips []chip
		if p := prettyPlan(s.Codex.Plan); p != "" {
			chips = append(chips, chip{Text: p})
		}
		switch {
		case s.Codex.Limit != "":
			chips = append(chips, chip{Text: "한도 도달 · " + s.Codex.Limit, Kind: "bad"})
		case s.Codex.Src == "live":
			chips = append(chips, chip{Text: "live", Kind: "live"})
		case s.Codex.AgeMin >= 0:
			chips = append(chips, chip{Text: "기록 " + fmtAge(s.Codex.AgeMin) + " 전"})
		}
		if s.Codex.Short.Left >= 0 {
			cards = append(cards, gaugeCard{"Codex · " + s.Codex.Short.Label,
				s.Codex.Short.Left, colCodex, s.Codex.Short.ResetAt, nil, markCodex})
		}
		cards = append(cards, gaugeCard{"Codex · 주간", s.Codex.Week.Left, colCodex,
			s.Codex.Week.ResetAt, chips, markCodex})
	}
	return cards
}

func cardHTML(c gaugeCard) string {
	cls := ""
	if c.Left >= 0 && c.Left < 20 {
		cls = " bad"
	} else if c.Left >= 0 && c.Left < 50 {
		cls = " warn"
	}
	left := `?<span>남음</span>`
	if c.Left >= 0 {
		left = fmt.Sprintf(`%d<span>%% 남음</span>`, c.Left)
	}

	reset := "리셋 시각 미확인"
	if c.ResetAt > 0 {
		reset = "리셋 " + fmtResetHuman(c.ResetAt)
		if u := fmtUntil(c.ResetAt); u != "" {
			reset += " · " + u
		}
	}

	chips := ""
	var cb strings.Builder
	n := 0
	for _, ch := range c.Chips {
		if ch.Text == "" {
			continue
		}
		k := ""
		if ch.Kind != "" {
			k = " " + ch.Kind
		}
		fmt.Fprintf(&cb, `<span class="chip%s">%s</span>`, k, html.EscapeString(ch.Text))
		n++
	}
	if n > 0 {
		chips = `<div class="chips">` + cb.String() + `</div>`
	}

	return fmt.Sprintf(`<div class="card"><img src="%s" alt=""><div class="cbody">`+
		`<div class="ct" style="color:rgb(%d,%d,%d)">%s%s</div><div class="cl%s">%s</div>`+
		`<div class="cs">%s</div>%s</div></div>`,
		ringIMG(c.Left, c.Brand), c.Brand.R, c.Brand.G, c.Brand.B,
		agentIcon(c.Mark, "sm"), html.EscapeString(c.Title), cls, left, html.EscapeString(reset), chips)
}

func statHTML(title string, m agentMark, pairs [][2]string) string {
	var b strings.Builder
	fmt.Fprintf(&b, `<div class="stat"><b>%s%s</b>`, agentIcon(m, "sm"), html.EscapeString(title))
	for _, p := range pairs {
		fmt.Fprintf(&b, `<i>%s<em>%s</em></i>`, html.EscapeString(p[0]), html.EscapeString(p[1]))
	}
	b.WriteString(`</div>`)
	return b.String()
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// 표의 한 줄 — 어느 에이전트에서 왔는지 마크로 들고 다닌다. 프로젝트 표는 이름으로 병합하므로
// 마크가 둘일 수 있다(예: skmp 를 Claude 와 Codex 가 함께 쓴다). 모델 표는 항상 하나다.
type projRow struct {
	NamedRow
	marks []agentMark
}

// 합계 행. label 은 "Claude 합계" · "전체 합계".
type totalRow struct {
	Row
	label string
}

func mergeModels(s *State, tok *TokenStats) []projRow {
	var out []projRow
	if s.Claude.Available {
		for _, m := range tok.Claude.Models7d {
			out = append(out, projRow{m, []agentMark{markClaude}})
		}
	}
	if s.Codex.Available {
		for _, m := range tok.Codex.Models7d {
			out = append(out, projRow{m, []agentMark{markCodex}})
		}
	}
	sortRows(out)
	if len(out) > 6 {
		out = out[:6]
	}
	return out
}

// 모델 한 줄 — 그 모델을 돌린 에이전트 마크 + 모델명. 마크가 곧 분류라 별도 태그 열을 뺐다.
func modelCell(m projRow) string {
	icon := ""
	if len(m.marks) > 0 {
		icon = agentIcon(m.marks[0], "lg")
	}
	return fmt.Sprintf(`<div class="mrow" title="%s">%s<span class="mt">%s</span></div>`,
		html.EscapeString(m.Name), icon, html.EscapeString(m.Name))
}

func marksHTML(marks []agentMark) string {
	if len(marks) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<span class="marks">`)
	for _, m := range marks {
		b.WriteString(agentIcon(m, "sm"))
	}
	b.WriteString(`</span>`)
	return b.String()
}

// 합계 — 표에 보이는 상위 6개가 아니라 7일 창의 전체 합이다(AgentTokens.Total7d).
// 옛 tokens.json 캐시에는 그 값이 없으므로, 0 이면 보이는 줄의 합으로 메운다.
func agentTotals(s *State, tok *TokenStats, shown []projRow) []totalRow {
	var out []totalRow
	var all Row
	add := func(name string, r Row) {
		if r.Requests == 0 {
			r = sumShown(shown, name)
		}
		if r.Requests == 0 && r.total() == 0 {
			return
		}
		all.add(r)
		out = append(out, totalRow{r, name + " 합계"})
	}
	if s.Claude.Available {
		add("Claude", tok.Claude.Total7d)
	}
	if s.Codex.Available {
		add("Codex", tok.Codex.Total7d)
	}
	switch len(out) {
	case 0:
		return nil
	case 1:
		out[0].label = "합계"
	default:
		out = append(out, totalRow{all, "전체 합계"})
	}
	return out
}

// 옛 캐시 대비용 — 그 에이전트의 마크가 붙은 줄만 더한다. 병합된 줄은 양쪽에 걸리므로
// 합이 살짝 커질 수 있으나, Total7d 가 있으면 이 경로는 타지 않는다.
func sumShown(shown []projRow, who string) Row {
	var r Row
	for _, x := range shown {
		for _, m := range x.marks {
			if m.name == who {
				r.add(x.Row)
				break
			}
		}
	}
	return r
}

// 프로젝트는 **이름으로 병합**한다 — 같은 저장소를 Claude 와 Codex 로 같이 쓰면 두 줄로
// 갈려 보여서 한눈에 "이 프로젝트에 얼마 썼나" 가 안 나왔다. 합쳐서 한 줄로 내고 어느
// 에이전트가 쓰였는지는 마크로 보인다.
func mergeProjects(s *State, tok *TokenStats) []projRow {
	byName := map[string]*projRow{}
	var order []string
	take := func(rows []NamedRow, mark agentMark) {
		for _, p := range rows {
			cur := byName[p.Name]
			if cur == nil {
				cur = &projRow{NamedRow: NamedRow{Name: p.Name}}
				byName[p.Name] = cur
				order = append(order, p.Name)
			}
			cur.NamedRow.Row.add(p.Row)
			cur.marks = append(cur.marks, mark)
		}
	}
	if s.Claude.Available {
		take(tok.Claude.Projects7d, markClaude)
	}
	if s.Codex.Available {
		take(tok.Codex.Projects7d, markCodex)
	}

	var out []projRow
	for _, name := range order {
		out = append(out, *byName[name])
	}
	sortRows(out)
	if len(out) > 6 {
		out = out[:6]
	}
	return out
}

func sortRows(rows []projRow) {
	for i := 1; i < len(rows); i++ {
		for j := i; j > 0 && rows[j].total() > rows[j-1].total(); j-- {
			rows[j], rows[j-1] = rows[j-1], rows[j]
		}
	}
}
