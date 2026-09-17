//go:build !windows

package main

// macOS(및 리눅스) 쪽 잡일. macOS 는 전부 OS 기본 명령으로 해결된다 — 추가 설치가 없다.

import (
	"os/exec"
	"runtime"
	"strings"
)

func hideWindow(cmd *exec.Cmd) {} // 창이 뜨지 않는다

// macOS 알림센터. 터미널에서 직접 돌리면 "터미널" 이름으로 뜨고, .app 으로 묶으면 그 이름으로 뜬다.
func notify(title, msg string) {
	switch runtime.GOOS {
	case "darwin":
		script := `display notification "` + escapeAS(msg) + `" with title "` + escapeAS(title) + `"`
		_ = exec.Command("osascript", "-e", script).Start()
	default:
		_ = exec.Command("notify-send", title, msg).Start()
	}
}

func escapeAS(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	return strings.ReplaceAll(s, `"`, `\"`)
}

func copyToClipboard(s string) {
	bin := "pbcopy"
	if runtime.GOOS != "darwin" {
		bin = "xclip"
	}
	cmd := exec.Command(bin)
	in, err := cmd.StdinPipe()
	if err != nil {
		return
	}
	if cmd.Start() != nil {
		return
	}
	_, _ = in.Write([]byte(s))
	_ = in.Close()
	_ = cmd.Wait()
}

func openURL(u string) { openPath(u) }

func openPath(p string) {
	bin := "open"
	if runtime.GOOS != "darwin" {
		bin = "xdg-open"
	}
	_ = exec.Command(bin, p).Start()
}
