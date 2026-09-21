# usage_tray

Claude Code · Codex · Antigravity 의 남은 사용량을 **작업표시줄(Windows) · 메뉴바(macOS)** 에 띄운다.

- 링 길이가 남은 %, 20% 밑이면 빨강. 색이 어느 에이전트인지 알려 준다
- **윈도우는 에이전트마다 링 하나**, **맥은 링 하나에 가장 급한 에이전트** ([왜 다른가](#아이콘-개수가-os-마다-다른-이유))
- 마우스를 올리면 창별 남은량과 리셋 시각, 클릭하면 전부 합친 상세 화면(브라우저)
- 남은량이 임계값을 넘으면 알림 한 번
- **쓰는 에이전트만 나온다.** 하나도 없으면 실행되지 않는다

바뀐 점과 연결 조건은 [UPGRADE.md](UPGRADE.md) 를 먼저 읽는다.

설치 과정도 설정 파일도 없다. 이미 로그인해 둔 자격증명을 읽을 뿐이고, 자기 계정의 자기
사용량만 보인다.

## 다운로드

[Releases](https://github.com/inging88/usage_tray/releases) 에서 자기 OS 파일을 받는다.

| OS | 파일 |
|---|---|
| Windows | `usage-tray.exe` |
| macOS | `UsageTray-macos-universal.zip` |

---

# Windows

## 설치 — 한 줄

`usage-tray.exe` 를 **계속 둘 폴더**에 놓고(경로가 바뀌면 다시 걸어야 한다) 한 줄만 친다.

```powershell
# PowerShell 은 현재 폴더를 PATH 로 찾지 않는다 — `.\` 를 붙이거나 전체 경로를 쓴다
.\usage-tray.exe -install
```

이 한 줄이 셋을 한다.

1. 시작프로그램에 바로가기를 만든다 → 다음 로그인부터 저절로 뜬다
2. 아이콘을 오버플로(∧)에서 작업표시줄로 꺼낸다(`IsPromoted=1`)
3. 트레이를 지금 띄운다

떼려면 `.\usage-tray.exe -uninstall`. 자동 실행만 떼고 아이콘 설정은 건드리지 않는다.

"Windows 의 PC 보호" 가 뜨면 `추가 정보` → `실행` — 서명이 없어서 나는 경고다.

## 어떻게 동작하나

새 트레이 아이콘은 기본으로 오버플로 안에 숨는다. Windows 11 은 아이콘별 표시 여부를
`HKCU\Control Panel\NotifyIconSettings\<id>\IsPromoted` 로 기억한다(`<id>` 는 실행 파일 경로
해시라 각 항목의 `ExecutablePath` 로 찾는다). 그 항목은 아이콘이 한 번 뜬 뒤에야 생기므로,
`-install` 은 항목이 없으면 앱을 먼저 띄워 만들고 기다린다.

**순서가 핵심이다.** 값을 먼저 박고 트레이를 새로 띄우면 셸이 아이콘 등록 시점에 그 값을 읽어
바로 작업표시줄에 올린다 — explorer 를 재시작할 일이 없다. 값만 따로 넣으려면 `-promote`,
셸이 값을 안 읽는 드문 경우에만 `-promote -restart-explorer`(작업표시줄이 몇 초 사라진다).

손으로 하려면 설정 → 개인 설정 → 작업 표시줄 → 시스템 트레이 아이콘에서 켜거나,
오버플로를 열어 아이콘을 작업표시줄로 끌어다 놓는다.

## 직접 빌드

Go 1.26+ 만 있으면 된다. 컴파일러는 필요 없다(`CGO_ENABLED=0`).

```powershell
git clone https://github.com/inging88/usage_tray.git
cd usage_tray
powershell -ExecutionPolicy Bypass -File build.ps1 -Run
```

## 파일이 놓이는 곳

`%LOCALAPPDATA%\usage-tray\` — 에이전트마다 갈라 띄우므로 `state-<agent>.json` ·
`status-<agent>.txt` · `tokens-<agent>.json` 과 `usage-tray.log` 다. 지울 때는 실행 파일과
이 폴더만 삭제하면 된다. `USAGE_TRAY_DATA_DIR` 을 주면 그 폴더를 대신 쓴다(검증용).

---

# macOS

## 실행

```bash
unzip UsageTray-macos-universal.zip
xattr -dr com.apple.quarantine UsageTray.app   # 서명이 ad-hoc 이라 한 번 필요하다
mv UsageTray.app /Applications/               # 계속 둘 자리로 옮긴다
/Applications/UsageTray.app/Contents/MacOS/usage-tray -install
```

마지막 줄이 로그인 항목에 등록하고 앱을 띄운다. 메뉴바에 링이 생기고, Dock 아이콘은 없다
(`LSUIElement`). 처음 한 번 맥이 **'시스템 이벤트' 제어 권한**을 물으면 허용한다 — 로그인
항목을 넣는 데 필요하다. 거절했다면 시스템 설정 → 개인정보 보호 및 보안 → 자동화 에서 켠다.

떼려면 `-uninstall`. 앱을 옮겼다면 새 자리에서 `-install` 을 다시 한 번 돌린다(옛 항목은
자동으로 지운다).

손으로 하려면 시스템 설정 → 일반 → 로그인 항목 → `+` → `UsageTray.app`

## 직접 빌드

**맥에서 빌드해야 한다.** 메뉴바(NSStatusItem)가 Cocoa API 라 cgo 를 거치고, cgo 는 크로스
빌드가 안 된다. Xcode Command Line Tools 만 있으면 된다 — `xcode-select --install`.

```bash
git clone https://github.com/inging88/usage_tray.git
cd usage_tray
./build.sh          # dist/UsageTray.app (arm64 + amd64 유니버설)
open dist/UsageTray.app
```

## 파일이 놓이는 곳

`~/Library/Application Support/usage-tray/` — 맥은 한 프로세스로 도니 접미사 없이
`state.json` · `status.txt` · `tokens.json` · `usage-tray.log` 다.

## 맥에서만 걸리는 것

Claude 자격증명이 Keychain 에 있을 수 있다. 파일에서 못 찾으면 후보 서비스명
(`Claude Code-credentials` · `Claude Code` · `claude-code`)을 시도한다. 그래도 안 되면:

```bash
security dump-keychain | grep -i -A2 claude | head -20
export USAGE_TRAY_CLAUDE_KEYCHAIN_SERVICE="찾은 이름"
```

---

# 공통

## 값은 어디서 오나

| 에이전트 | 엔드포인트 |
|---|---|
| Claude | `api.anthropic.com/api/oauth/usage` |
| Codex | `chatgpt.com/backend-api/codex/usage` |
| Antigravity | 로컬에서 도는 Antigravity 언어 서버 (`127.0.0.1` 루프백, macOS 만) |

Claude · Codex 는 비공개 경로다. 실패하면 로컬 기록으로 폴백하고 화면에 출처를 표시한다.
Antigravity 는 앱이 떠 있을 때만 잡히고, 자격증명을 루프백 밖으로 보내지 않으며 리디렉션도
따르지 않는다. 모델별 잔여 한도만 읽고 토큰 수나 과거 사용량은 수집하지 않는다.

호출은 아낀다 — statusline 이 받아 둔 캐시가 2분 이내면 그걸 쓰고, 직접 부를 때도 최소
간격(Claude 3분 · Codex 2분)을 지키며, 429 를 받으면 5분부터 두 배씩 최대 30분 쉰다.
실패해도 화면을 비우지 않고 직전 정상값에 나이를 붙인다.

## 자격증명을 찾는 자리

| | Windows | macOS |
|---|---|---|
| Claude | `%LOCALAPPDATA%\Claude Code\credentials.json` → `~\.claude\.credentials.json` | `~/.claude/.credentials.json` → `~/Library/Application Support/Claude Code/credentials.json` → Keychain |
| Codex | `~\.codex\auth.json` | `~/.codex/auth.json` |

`CLAUDE_CODE_OAUTH_TOKEN` 이 있으면 그게 우선이고, `CLAUDE_CONFIG_DIR` 을 주면 `~/.claude`
대신 그 폴더를 본다.

맥에서는 파일과 Keychain 에서 못 찾으면 Claude 데스크톱 앱의 OAuth 캐시
(`~/Library/Application Support/Claude/config.json`)를 마지막으로 본다. Electron safeStorage
로 잠긴 **지정된 필드만** 메모리에서 푼다 — 브라우저 쿠키나 Keychain 전체를 뒤지지 않고,
복호화 키와 토큰은 파일에도 로그에도 남기지 않는다. macOS 가 `Claude Safe Storage` 접근을
물으면 허용해야 한다. 그마저 막히면 같은 앱의 24시간 이내 사용 기록
(`plan-usage-history.json`)을 '앱 기록' 으로 표시한다 — 실시간 조회가 아니고 리셋 시각이
없어 미확인으로 나온다.

## 아이콘 개수가 OS 마다 다른 이유

트레이 API 는 프로세스당 아이콘 1개만 준다. 링을 여러 개 내려면 프로세스를 갈라야 한다.

**윈도우는 가른다.** 실행 파일이 자기 자신을 `-only claude` · `-only codex` 로 띄우고 부모는
빠진다. 작업표시줄에는 아이콘이 나란히 붙어도 문제가 없고, 에이전트마다 링 하나가 한눈에
낫다. 상세 화면은 어느 쪽을 눌러도 전부 합쳐 보여 준다
(claude 47113 · codex 47114 · antigravity 47115).

**맥은 못 가른다.** `.app` 본체가 자식을 띄우고 빠지는 순간 LaunchServices 가 앱이 끝난 것으로
보고, 번들 밖에서 도는 자식은 `NSStatusItem` 을 못 얻는다 — 메뉴바에 **아무것도 안 뜬다.**
0.3.0 이전 맥 빌드가 실행은 되는데 메뉴바가 비어 있던 이유가 이것이다. 그래서 맥은 한
프로세스가 링 하나를 띄우고 그 링에 **남은량이 가장 적은** 에이전트를 그린다. 툴팁과 상세
화면(47113)에는 쓰는 에이전트를 모두 담는다.

어느 쪽이든 `-only` 로 띄우면 그 에이전트 하나만 맡는다.

## 플래그

| 플래그 | 하는 일 |
|---|---|
| `-once` | 한 줄 요약과 툴팁 내용을 출력하고 끝낸다 |
| `-tokens` | 토큰 집계를 JSON 으로 출력 |
| `-only claude` / `-only codex` / `-only antigravity` | 한쪽만 다룬다 |
| `-icons <폴더>` | 아이콘 견본을 PNG/ICO 로 떨어뜨린다 |
| `-notify` | 알림을 한 번 띄워 본다 |
| `-install` | 로그인 자동 실행 + 아이콘 꺼내기 + 지금 띄우기를 한 번에 |
| `-uninstall` | `-install` 로 건 자동 실행을 뗀다 |
| `-promote` | 아이콘을 작업표시줄에 고정한다(윈도우). `-restart-explorer` 와 함께 쓰면 explorer 까지 재시작 |

## 문제 해결

| 증상 | 조치 |
|---|---|
| 아무것도 안 뜬다 | 자격증명이 없다. `usage-tray -once` 로 확인 |
| 링이 오버플로(∧) 안에만 있다 | `-install` 을 한 번 돌린다(윈도우) |
| 로그인해도 안 뜬다 | 실행 파일을 옮겼다면 새 자리에서 `-install` 을 다시 돌린다 |
| 이미 실행 중이라고 나온다 | 그 포트를 쓰는 인스턴스가 이미 있다 |
| Codex 가 `기록 N분 전` 으로만 나온다 | 토큰이 만료됐을 수 있다. `codex` 를 한 번 실행 |
| 값이 안 변한다 | 최소 간격 탓이다(3분). 우클릭 → 지금 갱신 |
| 토큰 표가 비어 있다 | 첫 스캔이 아직이다. `-tokens` 로 직접 돌려 본다 |

로그는 데이터 폴더의 `usage-tray.log` 이고 실패 사유만 쌓인다.
