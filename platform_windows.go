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
func promoteTrayIcons(restartExplorer bool) {
	exe, err := os.Executable()
	if err != nil {
		fmt.Println("자기 경로를 못 찾는다:", err)
		return
	}
	me := strings.ToLower(filepath.Base(exe))

	const root = `Control Panel\NotifyIconSettings`
	k, err := registry.OpenKey(registry.CURRENT_USER, root, registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		fmt.Println("NotifyIconSettings 를 못 연다(윈도우 11 이 아닐 수 있다):", err)
		return
	}
	names, err := k.ReadSubKeyNames(-1)
	k.Close()
	if err != nil {
		fmt.Println("항목을 못 읽는다:", err)
		return
	}

	found := 0
	for _, n := range names {
		sub, err := registry.OpenKey(registry.CURRENT_USER, root+`\`+n, registry.QUERY_VALUE|registry.SET_VALUE)
		if err != nil {
			continue
		}
		p, _, err := sub.GetStringValue("ExecutablePath")
		if err == nil && strings.Contains(strings.ToLower(p), me) {
			if err := sub.SetDWordValue("IsPromoted", 1); err == nil {
				found++
			}
		}
		sub.Close()
	}

	if found == 0 {
		fmt.Println("이 실행 파일의 트레이 항목이 없다. 앱을 한 번 실행해 아이콘이 뜬 뒤에 다시 시도한다.")
		return
	}
	fmt.Printf("작업표시줄에 고정했다 (항목 %d개).\n", found)

	if restartExplorer {
		fmt.Println("explorer 를 재시작한다 — 작업표시줄이 잠깐 사라진다")
		_ = exec.Command("taskkill", "/F", "/IM", "explorer.exe").Run()
		time.Sleep(2 * time.Second)
		_ = exec.Command("explorer.exe").Start()
		fmt.Println("완료")
		return
	}
	fmt.Println("적용하려면 explorer 를 재시작하거나 로그아웃/재부팅한다 (-promote -restart-explorer 로 한 번에).")
}
