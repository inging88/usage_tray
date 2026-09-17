package main

// Codex 실시간 조회가 실패했을 때의 폴백 — rollout 파일 꼬리에 남은 마지막 rate_limits 줄.
// codex 가 서버와 턴을 돌 때만 기록되므로 유휴 중에는 묵는다. 그래서 나이(ageMin)를 함께 낸다.

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type rolloutLine struct {
	Timestamp string `json:"timestamp"`
	Payload   struct {
		RateLimits *struct {
			Primary *struct {
				UsedPercent   float64 `json:"used_percent"`
				WindowMinutes int64   `json:"window_minutes"`
				ResetsAt      int64   `json:"resets_at"`
			} `json:"primary"`
			Secondary *struct {
				UsedPercent   float64 `json:"used_percent"`
				WindowMinutes int64   `json:"window_minutes"`
				ResetsAt      int64   `json:"resets_at"`
			} `json:"secondary"`
			ReachedType *string `json:"rate_limit_reached_type"`
		} `json:"rate_limits"`
	} `json:"payload"`
}

// 가장 최근에 쓰인 rollout 파일.
func newestRollout() string {
	dir := codexSessionsDir()
	var best string
	var bestMod time.Time
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		name := d.Name()
		if !strings.HasPrefix(name, "rollout-") || !strings.HasSuffix(name, ".jsonl") {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		if info.ModTime().After(bestMod) {
			bestMod, best = info.ModTime(), p
		}
		return nil
	})
	return best
}

// 파일 꼬리 n 바이트. codex 가 쓰는 중일 수 있으므로 읽기 전용으로 열고 마지막 줄만 본다.
func tailBytes(path string, n int64) []byte {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil
	}
	size := st.Size()
	if n > size {
		n = size
	}
	buf := make([]byte, n)
	if _, err := f.ReadAt(buf, size-n); err != nil {
		return nil
	}
	// 앞이 잘린 첫 줄은 버린다.
	if n < size {
		if i := bytes.IndexByte(buf, '\n'); i >= 0 {
			buf = buf[i+1:]
		}
	}
	return buf
}

func codexFromRollout(u AgentUsage) AgentUsage {
	path := newestRollout()
	if path == "" {
		u.Model, u.Effort = codexConfigDefaults()
		return u
	}
	lines := bytes.Split(tailBytes(path, 256*1024), []byte("\n"))
	var ts int64
	rateSeen, numSeen := false, false
	for i := len(lines) - 1; i >= 0; i-- {
		ln := lines[i]
		if len(ln) < 20 || !bytes.Contains(ln, []byte(`"rate_limits"`)) {
			continue
		}
		var o rolloutLine
		if json.Unmarshal(ln, &o) != nil || o.Payload.RateLimits == nil {
			continue
		}
		rl := o.Payload.RateLimits
		if ts == 0 {
			ts = parseISO(o.Timestamp)
		}
		// 한도에 걸린 줄은 primary/secondary 가 null 이고 reached 만 남는다.
		// 도달 여부는 가장 최신 줄에서만, 숫자는 그 직전 줄에서 가져온다.
		if !rateSeen {
			rateSeen = true
			if rl.ReachedType != nil && *rl.ReachedType != "" {
				u.Limit = *rl.ReachedType
			}
		}
		if !numSeen && (rl.Primary != nil || rl.Secondary != nil) {
			numSeen = true
			if p := rl.Primary; p != nil {
				pc := p.UsedPercent
				w := Window{Left: leftFrom(&pc), ResetAt: p.ResetsAt, WindowS: p.WindowMinutes * 60, Label: "5h"}
				if w.WindowS >= 2*86400 {
					w.Label = "7d"
					u.Week = w
				} else {
					u.Short = w
				}
			}
			if s := rl.Secondary; s != nil {
				sc := s.UsedPercent
				w := Window{Left: leftFrom(&sc), ResetAt: s.ResetsAt, WindowS: s.WindowMinutes * 60, Label: "7d"}
				if w.WindowS > 0 && w.WindowS < 2*86400 {
					w.Label = "5h"
					u.Short = w
				} else {
					u.Week = w
				}
			}
		}
		if rateSeen && numSeen {
			break
		}
	}
	if ts > 0 {
		u.AgeMin = int(time.Since(time.Unix(ts, 0)).Minutes())
	}
	u.Src = "rollout"
	u.OK = u.Week.Left >= 0 || u.Short.Left >= 0 || u.Limit != ""
	u.Model, u.Effort = codexConfigDefaults()
	return u
}
