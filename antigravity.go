package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

func detectAntigravity() bool {
	if runtime.GOOS != "darwin" {
		return false
	}
	for _, p := range []string{"/Applications/Antigravity.app", filepath.Join(homeDir(), "Applications", "Antigravity.app"), filepath.Join(homeDir(), "Library", "Application Support", "Antigravity")} {
		if _, err := os.Stat(p); err == nil {
			return true
		}
	}
	return false
}

type agProcess struct{ pid, csrf string }

var csrfArgument = regexp.MustCompile(`--csrf_token(?:=|\s+)([^\s]+)`)

func parseAGProcesses(data string) []agProcess {
	var processes []agProcess
	for _, line := range strings.Split(data, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		// macOS truncates comm; the first args field contains the executable path.
		if !strings.Contains(strings.ToLower(fields[2]), "language_server") || !strings.Contains(strings.ToLower(line), "antigravity") {
			continue
		}
		if _, err := strconv.Atoi(fields[0]); err != nil {
			continue
		}
		m := csrfArgument.FindStringSubmatch(line)
		if len(m) != 2 {
			continue
		}
		processes = append(processes, agProcess{fields[0], m[1]})
	}
	return processes
}
func parseAGPorts(data string) []string {
	seen := map[string]bool{}
	var ports []string
	for _, line := range strings.Split(data, "\n") {
		if !strings.HasPrefix(line, "n") {
			continue
		}
		i := strings.LastIndexByte(line, ':')
		if i < 0 {
			continue
		}
		port := line[i+1:]
		n, err := strconv.Atoi(port)
		if err == nil && n > 0 && n <= 65535 && !seen[port] {
			seen[port] = true
			ports = append(ports, port)
		}
	}
	return ports
}

type agModel struct {
	Label        string `json:"label"`
	ModelOrAlias struct {
		Model string `json:"model"`
	} `json:"modelOrAlias"`
	QuotaInfo *struct {
		RemainingFraction *float64 `json:"remainingFraction"`
		ResetTime         string   `json:"resetTime"`
	} `json:"quotaInfo"`
}
type agStatus struct {
	CascadeModelConfigData struct {
		ClientModelConfigs []agModel `json:"clientModelConfigs"`
	} `json:"cascadeModelConfigData"`
}

func parseAntigravity(data []byte) (AgentUsage, error) {
	u := emptyAgent()
	u.Available = true
	var envelope struct {
		UserStatus *agStatus `json:"userStatus"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return u, err
	}
	status := envelope.UserStatus
	if status == nil {
		status = &agStatus{}
		if err := json.Unmarshal(data, status); err != nil {
			return u, err
		}
	}
	for _, model := range status.CascadeModelConfigData.ClientModelConfigs {
		if model.QuotaInfo == nil {
			continue
		}
		fraction := model.QuotaInfo.RemainingFraction
		// Missing quota is unknown, never assume it means 0% or 100%.
		if fraction == nil {
			continue
		}
		f := *fraction
		if f < 0 || f > 1 {
			continue
		}
		label := model.Label
		if label == "" {
			label = model.ModelOrAlias.Model
		}
		if label == "" {
			label = "모델"
		}
		w := Window{Left: int(math.Round(f * 100)), ResetAt: parseISO(model.QuotaInfo.ResetTime), Label: label}
		u.Models = append(u.Models, w)
		if u.Short.Left < 0 || w.Left < u.Short.Left {
			u.Short = w
			u.Short.Label = "모델 최소"
		}
	}
	if len(u.Models) == 0 {
		return u, fmt.Errorf("모델별 한도가 응답에 없습니다")
	}
	sort.SliceStable(u.Models, func(i, j int) bool { return u.Models[i].Label < u.Models[j].Label })
	u.OK = true
	u.Src = "local"
	u.AgeMin = 0
	u.FetchedAt = time.Now()
	return u, nil
}

func fetchAntigravity() AgentUsage {
	u := emptyAgent()
	u.Available = true
	if runtime.GOOS != "darwin" {
		u.Error = "현재 Antigravity 자동 연동은 macOS를 지원합니다"
		return u
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "/bin/ps", "-axo", "pid=,comm=,args=").Output()
	if err != nil {
		u.Error = "Antigravity 프로세스를 확인하지 못했습니다"
		return u
	}
	processes := parseAGProcesses(string(out))
	if len(processes) == 0 {
		u.Error = "Antigravity 앱을 열고 로그인해 주세요"
		return u
	}
	// Only loopback connections to ports owned by the identified language server.
	// Its HTTPS endpoint uses an ephemeral self-signed certificate.
	transport := &http.Transport{Proxy: nil, TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Timeout: time.Second, Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	for _, process := range processes {
		portData, err := exec.CommandContext(ctx, "/usr/sbin/lsof", "-nP", "-a", "-p", process.pid, "-iTCP", "-sTCP:LISTEN", "-Fn").Output()
		if err != nil {
			continue
		}
		for _, port := range parseAGPorts(string(portData)) {
			for _, scheme := range []string{"https", "http"} {
				if ctx.Err() != nil {
					u.Error = "Antigravity 조회 시간이 초과되었습니다"
					return u
				}
				url := scheme + "://127.0.0.1:" + port + "/exa.language_server_pb.LanguageServerService/GetUserStatus"
				req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(`{"metadata":{"ideName":"antigravity","extensionName":"antigravity","locale":"en"}}`))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Connect-Protocol-Version", "1")
				req.Header.Set("X-Codeium-Csrf-Token", process.csrf)
				resp, err := client.Do(req)
				if err != nil {
					continue
				}
				body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
				resp.Body.Close()
				if err != nil || resp.StatusCode != 200 {
					continue
				}
				if result, err := parseAntigravity(body); err == nil {
					return result
				}
			}
		}
	}
	u.Error = "Antigravity의 모델별 한도를 받지 못했습니다. 앱 로그인 상태를 확인해 주세요"
	return u
}
