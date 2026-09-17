package main

// 토큰 집계 — Claude Code 트랜스크립트와 Codex rollout 을 증분으로 읽는다.
//
// 로그가 수백 MB 라 매번 통째로 읽을 수 없다. 파일마다 마지막으로 읽은 바이트 위치를
// tokens-cache.json 에 남기고 다음번엔 그 뒤만 읽는다(둘 다 append-only 로그다).
// 개행으로 끝나지 않는 마지막 줄은 아직 쓰이는 중이므로 남겨 둔다.
//
//	Claude : type=assistant 레코드의 message.usage — requestId 로 중복 제거(스트리밍으로 반복된다)
//	Codex  : type=token_usage_record 의 usage   — response_id 로 중복 제거
//
// 집계 키 = 날짜(로컬)|모델|프로젝트. 60일 보관.

import (
	"bufio"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const keepDays = 60

type Row struct {
	Input       int64 `json:"input"`
	CacheRead   int64 `json:"cacheRead"`
	CacheCreate int64 `json:"cacheCreate"`
	Output      int64 `json:"output"`
	Requests    int64 `json:"requests"`
}

func (r *Row) add(o Row) {
	r.Input += o.Input
	r.CacheRead += o.CacheRead
	r.CacheCreate += o.CacheCreate
	r.Output += o.Output
	r.Requests += o.Requests
}

func (r Row) total() int64 { return r.Input + r.CacheRead + r.CacheCreate + r.Output }

func (r Row) hitPct() int {
	in := r.Input + r.CacheRead + r.CacheCreate
	if in <= 0 {
		return 0
	}
	return int(100 * float64(r.CacheRead) / float64(in))
}

type fileState struct {
	Offset  int64  `json:"offset"`
	LastKey string `json:"lastKey"`
	Project string `json:"project,omitempty"`
	Model   string `json:"model,omitempty"`
}

type tokenCache struct {
	Version int                   `json:"version"`
	Files   map[string]*fileState `json:"files"`
	Claude  map[string]*Row       `json:"claude"`
	Codex   map[string]*Row       `json:"codex"`
}

type NamedRow struct {
	Name string `json:"name"`
	Row
}

type DayRow struct {
	Day string `json:"day"`
	Row
}

type AgentTokens struct {
	Days14     []DayRow   `json:"days14"`
	Models7d   []NamedRow `json:"models7d"`
	Projects7d []NamedRow `json:"projects7d"`
	Today      Row        `json:"today"`
}

type TokenStats struct {
	Updated time.Time   `json:"updated"`
	ScanMs  int64       `json:"scanMs"`
	Claude  AgentTokens `json:"claude"`
	Codex   AgentTokens `json:"codex"`
}

func cachePath() string { return filepath.Join(dataDir(), "tokens-cache"+suffix()+".json") }

func tokensPath() string { return filepath.Join(dataDir(), "tokens"+suffix()+".json") }

func loadCache() *tokenCache {
	c := &tokenCache{Version: 1, Files: map[string]*fileState{}, Claude: map[string]*Row{}, Codex: map[string]*Row{}}
	b, err := os.ReadFile(cachePath())
	if err != nil {
		return c
	}
	var got tokenCache
	if json.Unmarshal(b, &got) != nil || got.Version != 1 {
		return c
	}
	if got.Files == nil {
		got.Files = map[string]*fileState{}
	}
	if got.Claude == nil {
		got.Claude = map[string]*Row{}
	}
	if got.Codex == nil {
		got.Codex = map[string]*Row{}
	}
	return &got
}

func (c *tokenCache) save() {
	b, err := json.Marshal(c)
	if err != nil {
		return
	}
	tmp := cachePath() + ".tmp"
	if os.WriteFile(tmp, b, 0o644) == nil {
		_ = os.Rename(tmp, cachePath())
	}
}

func localDay(unix int64) string { return time.Unix(unix, 0).Local().Format("2006-01-02") }

func projName(cwd string) string {
	if cwd == "" {
		return "?"
	}
	c := strings.ReplaceAll(cwd, `\\?\`, "")
	c = strings.ReplaceAll(c, `\`, "/")
	c = strings.TrimRight(c, "/")
	if i := strings.LastIndexByte(c, '/'); i >= 0 && i+1 < len(c) {
		return c[i+1:]
	}
	return c
}

// 새로 늘어난 부분만 줄 단위로 넘긴다.
func scanNewLines(path string, st *fileState, fn func(line []byte)) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return
	}
	size := info.Size()
	if size < st.Offset { // 재작성됨
		st.Offset = 0
	}
	if size == st.Offset {
		return
	}
	if _, err := f.Seek(st.Offset, 0); err != nil {
		return
	}
	r := bufio.NewReaderSize(f, 1<<20)
	consumed := st.Offset
	for {
		line, err := r.ReadBytes('\n')
		if err != nil {
			// 개행 없이 끝났다 = 아직 쓰이는 중. 다음번에 다시 읽는다.
			break
		}
		consumed += int64(len(line))
		fn(line)
	}
	st.Offset = consumed
}

func walkLogs(root, prefix, suffix string, fn func(path string)) {
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		n := d.Name()
		if prefix != "" && !strings.HasPrefix(n, prefix) {
			return nil
		}
		if !strings.HasSuffix(n, suffix) {
			return nil
		}
		fn(p)
		return nil
	})
}

type claudeRec struct {
	Type      string `json:"type"`
	Timestamp string `json:"timestamp"`
	Cwd       string `json:"cwd"`
	RequestID string `json:"requestId"`
	Message   struct {
		ID    string `json:"id"`
		Model string `json:"model"`
		Usage struct {
			Input       int64 `json:"input_tokens"`
			CacheCreate int64 `json:"cache_creation_input_tokens"`
			CacheRead   int64 `json:"cache_read_input_tokens"`
			Output      int64 `json:"output_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

func (c *tokenCache) scanClaude() {
	walkLogs(claudeProjectsDir(), "", ".jsonl", func(p string) {
		st := c.Files["c:"+p]
		if st == nil {
			st = &fileState{}
			c.Files["c:"+p] = st
		}
		scanNewLines(p, st, func(line []byte) {
			if !strings.Contains(string(line), `"assistant"`) || !strings.Contains(string(line), `"usage"`) {
				return
			}
			var r claudeRec
			if json.Unmarshal(line, &r) != nil || r.Type != "assistant" {
				return
			}
			u := r.Message.Usage
			if u.Input == 0 && u.Output == 0 && u.CacheRead == 0 && u.CacheCreate == 0 {
				return
			}
			key := r.RequestID
			if key == "" {
				key = r.Message.ID
			}
			if key != "" && key == st.LastKey {
				return // 같은 응답의 스트리밍 반복
			}
			st.LastKey = key
			ts := parseISO(r.Timestamp)
			if ts == 0 {
				return
			}
			model := r.Message.Model
			if model == "" {
				model = "?"
			}
			k := localDay(ts) + "|" + model + "|" + projName(r.Cwd)
			row := c.Claude[k]
			if row == nil {
				row = &Row{}
				c.Claude[k] = row
			}
			row.add(Row{Input: u.Input, CacheRead: u.CacheRead, CacheCreate: u.CacheCreate, Output: u.Output, Requests: 1})
		})
	})
}

type codexRec struct {
	Type      string `json:"type"`
	Timestamp string `json:"timestamp"`
	Payload   struct {
		Cwd        string `json:"cwd"`
		Model      string `json:"model"`
		ResponseID string `json:"response_id"`
		Usage      struct {
			Input      int64 `json:"input_tokens"`
			Cached     int64 `json:"cached_input_tokens"`
			CacheWrite int64 `json:"cache_write_input_tokens"`
			Output     int64 `json:"output_tokens"`
		} `json:"usage"`
	} `json:"payload"`
}

func (c *tokenCache) scanCodex() {
	walkLogs(codexSessionsDir(), "rollout-", ".jsonl", func(p string) {
		st := c.Files["x:"+p]
		if st == nil {
			st = &fileState{Project: "?", Model: "?"}
			c.Files["x:"+p] = st
		}
		scanNewLines(p, st, func(line []byte) {
			s := string(line)
			switch {
			case strings.Contains(s, `"session_meta"`):
				var r codexRec
				if json.Unmarshal(line, &r) == nil && r.Payload.Cwd != "" {
					st.Project = projName(r.Payload.Cwd)
				}
				return
			case strings.Contains(s, `"turn_context"`):
				var r codexRec
				if json.Unmarshal(line, &r) == nil && r.Payload.Model != "" {
					st.Model = r.Payload.Model
				}
				return
			case !strings.Contains(s, `"token_usage_record"`):
				return
			}
			var r codexRec
			if json.Unmarshal(line, &r) != nil || r.Type != "token_usage_record" {
				return
			}
			u := r.Payload.Usage
			if u.Input == 0 && u.Output == 0 {
				return
			}
			if r.Payload.ResponseID != "" && r.Payload.ResponseID == st.LastKey {
				return
			}
			st.LastKey = r.Payload.ResponseID
			ts := parseISO(r.Timestamp)
			if ts == 0 {
				return
			}
			model := st.Model
			if model == "" {
				model = "?"
			}
			k := localDay(ts) + "|" + model + "|" + st.Project
			row := c.Codex[k]
			if row == nil {
				row = &Row{}
				c.Codex[k] = row
			}
			// Codex 의 cached_input_tokens 는 input 에 포함된 값이라 CacheRead 로 따로 센다.
			row.add(Row{Input: u.Input, CacheRead: u.Cached, CacheCreate: u.CacheWrite, Output: u.Output, Requests: 1})
		})
	})
}

func prune(m map[string]*Row) {
	cut := time.Now().AddDate(0, 0, -keepDays).Format("2006-01-02")
	for k := range m {
		if strings.SplitN(k, "|", 2)[0] < cut {
			delete(m, k)
		}
	}
}

func views(agg map[string]*Row, daysBack int) AgentTokens {
	var out AgentTokens
	today := time.Now().Local().Format("2006-01-02")
	since := time.Now().Local().AddDate(0, 0, -(daysBack - 1)).Format("2006-01-02")
	days, models, projs := map[string]*Row{}, map[string]*Row{}, map[string]*Row{}
	for k, row := range agg {
		parts := strings.SplitN(k, "|", 3)
		if len(parts) != 3 {
			continue
		}
		day, model, proj := parts[0], parts[1], parts[2]
		addTo(days, day, *row)
		if day >= since {
			addTo(models, model, *row)
			addTo(projs, proj, *row)
		}
		if day == today {
			out.Today.add(*row)
		}
	}
	// 14일 — 빈 날도 채운다(차트가 날짜를 건너뛰지 않게).
	for i := 13; i >= 0; i-- {
		d := time.Now().Local().AddDate(0, 0, -i).Format("2006-01-02")
		r := Row{}
		if got := days[d]; got != nil {
			r = *got
		}
		out.Days14 = append(out.Days14, DayRow{Day: d, Row: r})
	}
	out.Models7d = topRows(models, 6)
	out.Projects7d = topRows(projs, 6)
	return out
}

func addTo(m map[string]*Row, k string, r Row) {
	if m[k] == nil {
		m[k] = &Row{}
	}
	m[k].add(r)
}

func topRows(m map[string]*Row, n int) []NamedRow {
	var rows []NamedRow
	for k, v := range m {
		if k == "<synthetic>" {
			continue
		}
		rows = append(rows, NamedRow{Name: k, Row: *v})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].total() > rows[j].total() })
	if len(rows) > n {
		rows = rows[:n]
	}
	return rows
}

// 쓰는 에이전트만 스캔한다.
func scanTokens(wantClaude, wantCodex bool) *TokenStats {
	t0 := time.Now()
	c := loadCache()
	if wantClaude {
		c.scanClaude()
		prune(c.Claude)
	}
	if wantCodex {
		c.scanCodex()
		prune(c.Codex)
	}
	c.save()
	out := &TokenStats{
		Updated: time.Now(),
		ScanMs:  time.Since(t0).Milliseconds(),
		Claude:  views(c.Claude, 7),
		Codex:   views(c.Codex, 7),
	}
	// 옆 프로세스가 읽어 상세 화면에서 합칠 수 있게 파일로도 남긴다.
	if b, err := json.Marshal(out); err == nil {
		_ = os.WriteFile(tokensPath(), b, 0o644)
	}
	return out
}
