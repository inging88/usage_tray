package main

// 상세 화면 — 127.0.0.1 의 작은 HTTP 서버가 내는 HTML 한 장.
//
// 왜 네이티브 창이 아닌가: 트레이 라이브러리(systray)는 창을 만들지 못하고, 창을 쓰려면
// Windows 는 WinForms, macOS 는 Cocoa 로 따로 만들어야 한다(= 두 벌 유지). HTML 한 장이면
// 양쪽이 같고, 차트·표를 그리기도 훨씬 쉽다. 브라우저 탭으로 열린다.
//
// 링은 트레이 아이콘과 같은 코드로 그려 base64 PNG 로 심는다 — 화면과 아이콘이 어긋나지 않게.

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

func fmtReset(unix int64, withDay bool) string {
	if unix <= 0 {
		return "?"
	}
	t := time.Unix(unix, 0).Local()
	if withDay {
		return strings.ToLower(t.Format("Mon 15:04"))
	}
	return t.Format("15:04")
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

type gaugeCard struct {
	Title string
	Left  int
	Brand color.NRGBA
	Reset string
	Note  string
}

// 막대 차트를 인라인 SVG 로. 의존성 없이 확대해도 깨지지 않는다.
func barChartSVG(days []DayRow, brand color.NRGBA, valueOf func(DayRow) int64) string {
	const w, h = 760.0, 170.0
	var max int64 = 1
	vals := make([]int64, len(days))
	for i, d := range days {
		vals[i] = valueOf(d)
		if vals[i] > max {
			max = vals[i]
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, `<svg viewBox="0 0 %.0f %.0f" class="chart">`, w, h)
	// 눈금 4줄 + 라벨
	for k := 0; k <= 3; k++ {
		y := h - 18 - (h-40)*float64(k)/3
		fmt.Fprintf(&b, `<line x1="0" y1="%.1f" x2="%.0f" y2="%.1f" class="grid"/>`, y, w-46, y)
		fmt.Fprintf(&b, `<text x="%.0f" y="%.1f" class="tick">%s</text>`, w-42, y+4, fmtTokens(max*int64(k)/3))
	}
	slot := (w - 52) / float64(len(days))
	bw := slot * 0.62
	rgb := fmt.Sprintf("rgb(%d,%d,%d)", brand.R, brand.G, brand.B)
	for i, v := range vals {
		bh := (h - 40) * float64(v) / float64(max)
		if bh < 0 {
			bh = 0
		}
		x := float64(i)*slot + (slot-bw)/2
		op := "0.55"
		if i == len(vals)-1 {
			op = "1"
		}
		if bh > 0 {
			fmt.Fprintf(&b, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" rx="2" fill="%s" opacity="%s"><title>%s · %s</title></rect>`,
				x, h-18-bh, bw, bh, rgb, op, days[i].Day, fmtTokens(v))
		}
		if i%2 == 1 || i == len(vals)-1 {
			fmt.Fprintf(&b, `<text x="%.1f" y="%.0f" class="xlab" text-anchor="middle">%s</text>`,
				x+bw/2, h-4, strings.Replace(days[i].Day[5:], "-", "/", 1))
		}
	}
	b.WriteString(`</svg>`)
	return b.String()
}

func renderPage(s *State, tok *TokenStats) string {
	var cards []gaugeCard
	if s.Claude.Available {
		n := ""
		if s.Claude.Src != "" && s.Claude.Src != "api" {
			n = s.Claude.Src
		}
		cards = append(cards,
			gaugeCard{"Claude · 5시간", s.Claude.Short.Left, colClaude, fmtReset(s.Claude.Short.ResetAt, false), n},
			gaugeCard{"Claude · 주간", s.Claude.Week.Left, colClaude, fmtReset(s.Claude.Week.ResetAt, true), s.Claude.Extra})
	}
	if s.Codex.Available {
		note := strings.ReplaceAll(strings.TrimPrefix(s.Codex.Plan, "self_serve_"), "_", " ")
		switch {
		case s.Codex.Limit != "":
			note = "한도 도달 · " + s.Codex.Limit
		case s.Codex.Src == "live":
			note = strings.TrimSpace(note + "  ·  live")
		case s.Codex.AgeMin >= 0:
			note = "기록 " + fmtAge(s.Codex.AgeMin) + " 전"
		}
		if s.Codex.Short.Left >= 0 {
			cards = append(cards, gaugeCard{"Codex · " + s.Codex.Short.Label, s.Codex.Short.Left, colCodex, fmtReset(s.Codex.Short.ResetAt, false), ""})
		}
		cards = append(cards, gaugeCard{"Codex · 주간", s.Codex.Week.Left, colCodex, fmtReset(s.Codex.Week.ResetAt, true), note})
	}

	var b strings.Builder
	b.WriteString(`<!doctype html><html lang="ko"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<meta http-equiv="refresh" content="30">
<title>사용량</title><style>
:root{--bg:#18181b;--panel:#202024;--line:#303036;--tx:#f4f4f5;--sub:#a1a1aa;--dim:#71717a}
*{box-sizing:border-box}
body{margin:0 auto;max-width:1180px;background:var(--bg);color:var(--tx);font:14px/1.5 "Malgun Gothic","Apple SD Gothic Neo",system-ui,sans-serif;padding:24px}
h1{font-size:22px;margin:0 0 2px}
.meta{color:var(--dim);font-size:12px;margin-bottom:18px}
.row{display:flex;flex-wrap:wrap;gap:12px;margin-bottom:18px}
.card{background:var(--panel);border-radius:12px;padding:16px;display:flex;gap:14px;align-items:center;min-width:250px;flex:1}
.card img{width:96px;height:96px}
.ct{font-weight:700;font-size:14px;margin-bottom:2px}
.cl{font-size:30px;font-weight:700;line-height:1.1}
.cl span{font-size:14px;color:var(--sub);font-weight:400}
.cs{color:var(--dim);font-size:12px;margin-top:6px}
.panel{background:var(--panel);border-radius:12px;padding:16px;flex:1;min-width:320px}
.panel h2{font-size:13px;margin:0 0 10px;font-weight:700}
.chart{width:100%;height:auto;display:block}
.grid{stroke:#2a2a30;stroke-width:1}
.tick,.xlab{fill:#71717a;font-size:11px;font-family:inherit}
table{width:100%;border-collapse:collapse;font-size:12.5px}
th{text-align:left;color:var(--dim);font-weight:400;font-size:11px;padding:0 8px 6px 0;border-bottom:1px solid var(--line)}
td{padding:5px 8px 5px 0;white-space:nowrap}
td.n{color:var(--sub)}
.today{color:var(--sub);font-size:12px;margin:0 0 16px}
footer{color:var(--dim);font-size:11px;margin-top:18px}
.warn{color:#fbbf24}.bad{color:#f87171}
</style></head><body>`)

	fmt.Fprintf(&b, `<h1>사용량</h1><div class="meta">갱신 %s · 30초마다 새로고침</div>`,
		s.Updated.Local().Format("15:04:05"))

	b.WriteString(`<div class="row">`)
	for _, c := range cards {
		cls := ""
		if c.Left >= 0 && c.Left < 20 {
			cls = " bad"
		} else if c.Left >= 0 && c.Left < 50 {
			cls = " warn"
		}
		leftTxt := "?"
		if c.Left >= 0 {
			leftTxt = fmt.Sprintf("%d<span>%%</span>", c.Left)
		}
		fmt.Fprintf(&b, `<div class="card"><img src="%s" alt=""><div><div class="ct" style="color:rgb(%d,%d,%d)">%s</div>`+
			`<div class="cl%s">%s</div><div class="cs">남음 · 리셋 %s%s</div></div></div>`,
			ringIMG(c.Left, c.Brand), c.Brand.R, c.Brand.G, c.Brand.B, html.EscapeString(c.Title),
			cls, leftTxt, html.EscapeString(c.Reset), noteHTML(c.Note))
	}
	b.WriteString(`</div>`)

	if tok == nil {
		b.WriteString(`<div class="today">토큰 집계가 아직 없다 — 첫 스캔이 끝나면 채워진다.</div>`)
	} else {
		b.WriteString(`<div class="row">`)
		if s.Claude.Available {
			fmt.Fprintf(&b, `<div class="panel"><h2>Claude · 14일 토큰 (입력+캐시+출력)</h2>%s</div>`,
				barChartSVG(tok.Claude.Days14, colClaude, func(d DayRow) int64 { return d.total() }))
		}
		if s.Codex.Available {
			fmt.Fprintf(&b, `<div class="panel"><h2>Codex · 14일 토큰 (입력+출력)</h2>%s</div>`,
				barChartSVG(tok.Codex.Days14, colCodex, func(d DayRow) int64 { return d.Input + d.Output }))
		}
		b.WriteString(`</div>`)

		var parts []string
		if s.Claude.Available {
			t := tok.Claude.Today
			parts = append(parts, fmt.Sprintf("Claude 입력 %s · 출력 %s · 요청 %d · 캐시 적중 %d%%",
				fmtTokens(t.Input+t.CacheRead+t.CacheCreate), fmtTokens(t.Output), t.Requests, t.hitPct()))
		}
		if s.Codex.Available {
			t := tok.Codex.Today
			parts = append(parts, fmt.Sprintf("Codex 입력 %s · 출력 %s · 요청 %d · 캐시 %d%%",
				fmtTokens(t.Input), fmtTokens(t.Output), t.Requests, t.hitPct()))
		}
		fmt.Fprintf(&b, `<div class="today">오늘 &nbsp; %s</div>`, html.EscapeString(strings.Join(parts, "   ·   ")))

		b.WriteString(`<div class="row">`)
		b.WriteString(`<div class="panel"><h2>모델별 · 7일</h2><table><tr><th>모델</th><th>요청</th><th>입력</th><th>출력</th><th>캐시</th></tr>`)
		for _, m := range mergeModels(s, tok) {
			fmt.Fprintf(&b, `<tr><td>%s</td><td class="n">%d</td><td class="n">%s</td><td class="n">%s</td><td class="n">%d%%</td></tr>`,
				html.EscapeString(m.Name), m.Requests, fmtTokens(m.Input+m.CacheCreate), fmtTokens(m.Output), m.hitPct())
		}
		b.WriteString(`</table></div>`)
		b.WriteString(`<div class="panel"><h2>프로젝트별 · 7일</h2><table><tr><th>프로젝트</th><th></th><th>요청</th><th>토큰</th></tr>`)
		both := s.Claude.Available && s.Codex.Available
		for _, p := range mergeProjects(s, tok) {
			who := ""
			if both {
				who = p.who
			}
			fmt.Fprintf(&b, `<tr><td>%s</td><td class="n">%s</td><td class="n">%d</td><td class="n">%s</td></tr>`,
				html.EscapeString(p.Name), who, p.Requests, fmtTokens(p.total()))
		}
		b.WriteString(`</table></div></div>`)
		fmt.Fprintf(&b, `<footer>집계 %s · %dms · 출처 Claude %s / Codex %s</footer>`,
			tok.Updated.Local().Format("2006-01-02 15:04:05"), tok.ScanMs,
			orDash(s.Claude.Src), orDash(s.Codex.Src))
	}
	b.WriteString(`</body></html>`)
	return b.String()
}

func noteHTML(note string) string {
	if note == "" {
		return ""
	}
	return " · " + html.EscapeString(note)
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

type projRow struct {
	NamedRow
	who string
}

func mergeModels(s *State, tok *TokenStats) []NamedRow {
	var out []NamedRow
	if s.Claude.Available {
		out = append(out, tok.Claude.Models7d...)
	}
	if s.Codex.Available {
		out = append(out, tok.Codex.Models7d...)
	}
	sortNamed(out)
	if len(out) > 6 {
		out = out[:6]
	}
	return out
}

func mergeProjects(s *State, tok *TokenStats) []projRow {
	var out []projRow
	if s.Claude.Available {
		for _, p := range tok.Claude.Projects7d {
			out = append(out, projRow{p, "Claude"})
		}
	}
	if s.Codex.Available {
		for _, p := range tok.Codex.Projects7d {
			out = append(out, projRow{p, "Codex"})
		}
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].total() > out[j-1].total(); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	if len(out) > 6 {
		out = out[:6]
	}
	return out
}

func sortNamed(rows []NamedRow) {
	for i := 1; i < len(rows); i++ {
		for j := i; j > 0 && rows[j].total() > rows[j-1].total(); j-- {
			rows[j], rows[j-1] = rows[j-1], rows[j]
		}
	}
}
