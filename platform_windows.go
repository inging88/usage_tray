//go:build windows

package main

// Windows 전용 잡일 — 콘솔 창 숨기기, 알림, 클립보드, 열기.

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
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
