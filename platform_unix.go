//go:build !windows

package main

// macOS(및 리눅스) 쪽 잡일. macOS 는 전부 OS 기본 명령으로 해결된다 — 추가 설치가 없다.

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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

// 작업표시줄 고정은 윈도우 개념이다. 맥 메뉴바 아이콘은 늘 보인다(자리가 모자라면 OS 가 줄인다).
func promoteTrayIcons(restartExplorer bool) {
	fmt.Println("-promote 는 윈도우 전용이다. 맥 메뉴바 아이콘은 따로 고정할 필요가 없다.")
}

// ---------------------------------------------------------------- -install

func osa(script string) error { return exec.Command("osascript", "-e", script).Run() }

// 자기가 들어 있는 .app 을 찾는다. 로그인 항목에는 번들을 넣어야 한다 — 안쪽 바이너리를 직접
// 넣으면 LSUIElement 가 적용되지 않아 Dock 에 뜨거나 메뉴바에 안 붙는다.
// 경로 모양: .../UsageTray.app/Contents/MacOS/usage-tray
func appBundle() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	d := filepath.Dir(exe)
	if filepath.Base(d) != "MacOS" {
		return ""
	}
	if d = filepath.Dir(d); filepath.Base(d) != "Contents" {
		return ""
	}
	if d = filepath.Dir(d); filepath.Ext(d) != ".app" {
		return ""
	}
	return d
}

// 로그인 항목 등록 + 지금 띄우기를 한 번에. 윈도우의 -install 과 짝이다.
func installStartup() {
	if runtime.GOOS != "darwin" {
		fmt.Println("-install 은 윈도우·macOS 에서만 쓴다.")
		return
	}
	app := appBundle()
	if app == "" {
		fmt.Println("UsageTray.app 안에서 실행해야 한다. ./build.sh 로 만든 dist/UsageTray.app 을 쓴다:")
		fmt.Println("  dist/UsageTray.app/Contents/MacOS/usage-tray -install")
		return
	}
	name := strings.TrimSuffix(filepath.Base(app), ".app")

	// 같은 이름이 남아 있으면 지우고 다시 넣는다 — 경로가 바뀌었을 때 옛 항목이 남지 않게.
	// 항목이 없으면 오류가 나므로 무시한다.
	_ = osa(`tell application "System Events" to delete (every login item whose name is "` + name + `")`)
	if err := osa(`tell application "System Events" to make login item at end with properties {path:"` + app + `", hidden:true}`); err != nil {
		fmt.Println("로그인 항목을 못 넣었다:", err)
		fmt.Println("맥이 '시스템 이벤트' 제어 권한을 물으면 허용한다.")
		fmt.Println("직접 넣으려면 시스템 설정 → 일반 → 로그인 항목 → + → " + filepath.Base(app))
		return
	}
	fmt.Println("로그인 자동 실행 등록:", app)

	if err := exec.Command("open", "-a", app).Run(); err != nil {
		fmt.Println("앱을 못 띄웠다 — 직접 연다:", err)
		return
	}
	fmt.Println("완료 — 메뉴바에 링이 보인다. 다음 로그인부터는 저절로 뜬다.")
}

func uninstallStartup() {
	if runtime.GOOS != "darwin" {
		fmt.Println("-uninstall 은 윈도우·macOS 에서만 쓴다.")
		return
	}
	app := appBundle()
	name := "UsageTray"
	if app != "" {
		name = strings.TrimSuffix(filepath.Base(app), ".app")
	}
	if err := osa(`tell application "System Events" to delete (every login item whose name is "` + name + `")`); err != nil {
		fmt.Println("로그인 항목을 못 지웠다:", err)
		fmt.Println("시스템 설정 → 일반 → 로그인 항목 에서 직접 뺀다.")
		return
	}
	fmt.Println("자동 실행 해제:", name)
}
