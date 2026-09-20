//go:build windows

package main

// Windows 전용 잡일 — 콘솔 창 숨기기, 알림, 클립보드, 열기.

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/windows/registry"
)

// curl 같은 자식 프로세스가 콘솔 창을 번쩍이지 않게.
func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
}

func runHidden(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	hideWindow(cmd)
	return cmd.Start()
}

// 풍선 알림. WinRT 토스트는 AppID 등록이 필요해서, 확실히 동작하는 NotifyIcon 풍선을 쓴다.
// 알림은 임계값을 넘을 때만 나므로 잠깐 도는 powershell 하나면 충분하다.
func notify(title, msg string) {
	ps := `[void][Reflection.Assembly]::LoadWithPartialName('System.Windows.Forms')
$n = New-Object System.Windows.Forms.NotifyIcon
$n.Icon = [System.Drawing.SystemIcons]::Information
$n.Visible = $true
$n.ShowBalloonTip(15000, $args[0], $args[1], [System.Windows.Forms.ToolTipIcon]::Warning)
Start-Sleep -Seconds 12
$n.Dispose()`
	f := filepath.Join(os.TempDir(), "usage-tray-notify.ps1")
	// PowerShell 5.1 은 BOM 없는 UTF-8 을 CP949 로 읽는다 — 한글이 깨지므로 BOM 을 붙인다.
	_ = os.WriteFile(f, append([]byte{0xEF, 0xBB, 0xBF}, []byte(ps)...), 0o644)
	_ = runHidden("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", f, title, msg)
}

func copyToClipboard(s string) {
	cmd := exec.Command("cmd", "/c", "clip")
	hideWindow(cmd)
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

func openURL(u string) { _ = runHidden("rundll32", "url.dll,FileProtocolHandler", u) }

func openPath(p string) { _ = exec.Command("explorer.exe", p).Start() }

// 트레이 아이콘을 오버플로(∧)에서 작업표시줄로 끌어올린다 — 설정의 '시스템 트레이 아이콘' 토글과 같다.
//
// Windows 11 은 아이콘별 표시 여부를 HKCU\Control Panel\NotifyIconSettings\<id>\IsPromoted 로
// 기억한다. <id> 는 실행 파일 경로 등으로 만든 해시라 이름으로는 못 찾고, 각 항목의
// ExecutablePath 를 보고 고른다. 아이콘이 한 번이라도 뜬 뒤에야 항목이 생기므로,
// 앱을 한 번 실행한 다음에 이 플래그를 쓴다.
const notifyIconRoot = `Control Panel\NotifyIconSettings`

// 이 실행 파일의 트레이 항목에 IsPromoted 를 박는다. 반환값은 건드린 항목 수.
// set 이 false 면 세기만 한다.
func promoteEntries(exe string, set bool) int {
	me := strings.ToLower(filepath.Base(exe))
	k, err := registry.OpenKey(registry.CURRENT_USER, notifyIconRoot, registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		return 0
	}
	names, err := k.ReadSubKeyNames(-1)
	k.Close()
	if err != nil {
		return 0
	}
	access := uint32(registry.QUERY_VALUE)
	if set {
		access |= registry.SET_VALUE
	}
	found := 0
	for _, n := range names {
		sub, err := registry.OpenKey(registry.CURRENT_USER, notifyIconRoot+`\`+n, access)
		if err != nil {
			continue
		}
		p, _, err := sub.GetStringValue("ExecutablePath")
		if err == nil && strings.Contains(strings.ToLower(p), me) {
			if !set {
				found++
			} else if err := sub.SetDWordValue("IsPromoted", 1); err == nil {
				found++
			}
		}
		sub.Close()
	}
	return found
}

// 떠 있는 트레이를 내리고 새로 띄운다. 자기 자신은 죽이지 않는다 — 죽으면 되살릴 주체가 없다.
//
// IsPromoted 를 먼저 박아 두고 이 함수를 부르면 explorer 를 건드릴 필요가 없다. 셸은 아이콘이
// 등록되는 시점에 그 값을 읽기 때문이다.
func restartTray(exe string) bool {
	kill := exec.Command("taskkill", "/F", "/IM", filepath.Base(exe), "/FI",
		fmt.Sprintf("PID ne %d", os.Getpid()))
	hideWindow(kill)
	_ = kill.Run()
	time.Sleep(500 * time.Millisecond)

	again := exec.Command(exe)
	hideWindow(again)
	if err := again.Start(); err != nil {
		fmt.Println("다시 띄우지 못했다 — 직접 실행한다:", err)
		return false
	}
	_ = again.Process.Release()
	return true
}

func promoteTrayIcons(restartExplorer bool) {
	exe, err := os.Executable()
	if err != nil {
		fmt.Println("자기 경로를 못 찾는다:", err)
		return
	}

	found := promoteEntries(exe, true)
	if found == 0 {
		fmt.Println("이 실행 파일의 트레이 항목이 없다. 앱을 한 번 실행해 아이콘이 뜬 뒤에 다시 시도한다.")
		return
	}
	fmt.Printf("작업표시줄에 고정했다 (항목 %d개).\n", found)

	if restartExplorer {
		// 값을 이미 박았으므로 트레이만 다시 띄우면 된다. explorer 재시작은 셸이 값을 안 읽는
		// 드문 경우를 위한 보험이라 여기 남겨 둔다 — 작업표시줄이 몇 초 사라진다.
		fmt.Println("트레이를 내린다")
		kill := exec.Command("taskkill", "/F", "/IM", filepath.Base(exe), "/FI",
			fmt.Sprintf("PID ne %d", os.Getpid()))
		hideWindow(kill)
		_ = kill.Run()

		fmt.Println("explorer 를 재시작한다 — 작업표시줄이 잠깐 사라진다")
		k2 := exec.Command("taskkill", "/F", "/IM", "explorer.exe")
		hideWindow(k2)
		_ = k2.Run()
		time.Sleep(2 * time.Second)
		if err := exec.Command("explorer.exe").Start(); err != nil {
			fmt.Println("explorer 를 못 띄웠다 — 작업 관리자에서 새 작업으로 explorer.exe 를 실행한다:", err)
		}
		time.Sleep(3 * time.Second)

		fmt.Println("트레이를 다시 띄운다")
		if restartTray(exe) {
			fmt.Println("완료 — 몇 초 뒤 작업표시줄에 링이 보인다")
		}
		return
	}
	fmt.Println("적용하려면 트레이를 다시 띄운다 — 셸은 아이콘이 등록될 때 이 값을 읽는다.")
	fmt.Println("한 번에 끝내려면 -install 을 쓴다(자동 실행 등록까지 한다).")
}

// ---------------------------------------------------------------- -install

func startupLink() string {
	return filepath.Join(os.Getenv("APPDATA"),
		`Microsoft\Windows\Start Menu\Programs\Startup`, "usage-tray.lnk")
}

// PowerShell 리터럴 안에 넣을 문자열. 작은따옴표만 이스케이프하면 된다.
func psQuote(s string) string { return strings.ReplaceAll(s, "'", "''") }

// powershell 한 조각을 돌리고 끝날 때까지 기다린다.
// PowerShell 5.1 은 BOM 없는 UTF-8 을 CP949 로 읽으므로 한글이 든 스크립트에는 BOM 을 붙인다.
func runPS(name, script string) error {
	f := filepath.Join(os.TempDir(), name)
	if err := os.WriteFile(f, append([]byte{0xEF, 0xBB, 0xBF}, []byte(script)...), 0o644); err != nil {
		return err
	}
	defer func() { _ = os.Remove(f) }()
	cmd := exec.Command("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", f)
	hideWindow(cmd)
	return cmd.Run()
}

// 로그인 자동 실행 + 작업표시줄 고정 + 지금 띄우기를 한 번에.
//
// 순서가 핵심이다. IsPromoted 를 먼저 박고 트레이를 새로 띄우면 셸이 등록 시점에 그 값을 읽어
// 바로 작업표시줄에 올린다 — explorer 를 재시작할 일이 없다.
func installStartup() {
	exe, err := os.Executable()
	if err != nil {
		fmt.Println("자기 경로를 못 찾는다:", err)
		return
	}
	link := startupLink()
	script := "$s = New-Object -ComObject WScript.Shell\n" +
		"$l = $s.CreateShortcut('" + psQuote(link) + "')\n" +
		"$l.TargetPath = '" + psQuote(exe) + "'\n" +
		"$l.WorkingDirectory = '" + psQuote(filepath.Dir(exe)) + "'\n" +
		"$l.Description = 'Claude Code / Codex 사용량 트레이'\n" +
		"$l.IconLocation = '" + psQuote(exe) + ",0'\n" +
		"$l.Save()\n"
	if err := runPS("usage-tray-install.ps1", script); err != nil {
		fmt.Println("시작프로그램 바로가기를 못 만들었다:", err)
		return
	}
	fmt.Println("로그인 자동 실행 등록:", link)
	fmt.Println("  ->", exe)

	// 아이콘 항목은 한 번 떠 봐야 생긴다. 없으면 띄우고 생길 때까지 기다린다.
	if promoteEntries(exe, false) == 0 {
		fmt.Println("트레이 항목이 없다 — 한 번 띄워 만든다")
		first := exec.Command(exe)
		hideWindow(first)
		if err := first.Start(); err == nil {
			_ = first.Process.Release()
		}
		for i := 0; i < 30 && promoteEntries(exe, false) == 0; i++ {
			time.Sleep(500 * time.Millisecond)
		}
	}

	n := promoteEntries(exe, true)
	if n == 0 {
		fmt.Println("작업표시줄 고정은 못 했다 — 아이콘이 뜬 뒤 -promote 를 한 번 쓴다.")
		fmt.Println("자동 실행 등록은 끝났으니 다음 로그인부터는 저절로 뜬다.")
		return
	}
	fmt.Printf("작업표시줄에 고정했다 (항목 %d개)\n", n)

	if restartTray(exe) {
		fmt.Println("완료 — 몇 초 뒤 작업표시줄에 링이 보인다. 다음 로그인부터는 저절로 뜬다.")
	}
}

func uninstallStartup() {
	link := startupLink()
	switch err := os.Remove(link); {
	case err == nil:
		fmt.Println("자동 실행 해제:", link)
	case os.IsNotExist(err):
		fmt.Println("등록된 자동 실행이 없다:", link)
	default:
		fmt.Println("바로가기를 못 지웠다:", err)
		return
	}
	fmt.Println("작업표시줄 고정(IsPromoted)은 그대로 둔다 — 설정 → 작업 표시줄에서 끈다.")
}
