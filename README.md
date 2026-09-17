# usage_tray

Claude Code 와 Codex 의 남은 사용량을 **작업표시줄(Windows) · 메뉴바(macOS)** 에 띄운다.

- 링 게이지 하나 = 에이전트 하나. 링 길이가 주간 남은 %, 20% 밑이면 빨강
- 마우스를 올리면 창별 남은량과 리셋 시각, 클릭하면 상세 화면(브라우저)
- 남은량이 임계값을 넘으면 알림 한 번
- **쓰는 에이전트만 나온다.** 둘 다 없으면 실행되지 않는다

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

## 실행

1. `usage-tray.exe` 를 아무 폴더에 둔다
2. 더블클릭 → "Windows 의 PC 보호" 가 뜨면 `추가 정보` → `실행` (서명이 없어서 나는 경고다)
3. 트레이에 링이 생긴다

## 아이콘이 안 보이면

오버플로(∧) 안에 있다. 설정 → 개인 설정 → 작업 표시줄 → 시스템 트레이 아이콘에서 켠다.
레지스트리로 직접 켜려면 `HKCU\Control Panel\NotifyIconSettings\<id>\IsPromoted = 1`
(`<id>` 는 각 항목의 `ExecutablePath` 로 찾는다). 아이콘이 한 번 나타난 뒤에야 항목이 생기고,
바꾼 뒤에는 explorer 를 재시작해야 적용된다.

## 로그인할 때 자동 실행

`Win+R` → `shell:startup` → 그 폴더에 `usage-tray.exe` 바로가기를 넣는다.

## 직접 빌드

Go 1.22+ 만 있으면 된다. 컴파일러는 필요 없다(`CGO_ENABLED=0`).

```powershell
git clone https://github.com/inging88/usage_tray.git
cd usage_tray
powershell -ExecutionPolicy Bypass -File build.ps1 -Run
```

## 파일이 놓이는 곳

`%LOCALAPPDATA%\usage-tray\` — `state-<agent>.json` · `status-<agent>.txt` ·
`tokens-<agent>.json` · `usage-tray.log`. 지울 때는 실행 파일과 이 폴더만 삭제하면 된다.

---

# macOS

## 실행

```bash
unzip UsageTray-macos-universal.zip
xattr -dr com.apple.quarantine UsageTray.app   # 서명이 ad-hoc 이라 한 번 필요하다
open UsageTray.app
```

메뉴바에 링이 생긴다. Dock 아이콘은 없다(`LSUIElement`).

## 로그인할 때 자동 실행

시스템 설정 → 일반 → 로그인 항목 → `+` → `UsageTray.app`

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

`~/Library/Application Support/usage-tray/` — Windows 와 같은 구성.

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

둘 다 비공개 경로다. 실패하면 로컬 기록으로 폴백하고 화면에 출처를 표시한다.

호출은 아낀다 — statusline 이 받아 둔 캐시가 2분 이내면 그걸 쓰고, 직접 부를 때도 최소
간격(Claude 3분 · Codex 2분)을 지키며, 429 를 받으면 5분부터 두 배씩 최대 30분 쉰다.
실패해도 화면을 비우지 않고 직전 정상값에 나이를 붙인다.

## 자격증명을 찾는 자리

| | Windows | macOS |
|---|---|---|
| Claude | `%LOCALAPPDATA%\Claude Code\credentials.json` → `~\.claude\.credentials.json` | `~/.claude/.credentials.json` → `~/Library/Application Support/Claude Code/credentials.json` → Keychain |
| Codex | `~\.codex\auth.json` | `~/.codex/auth.json` |

`CLAUDE_CODE_OAUTH_TOKEN` 이 있으면 그게 우선이다.

## 아이콘이 에이전트마다 하나인 이유

트레이 API 가 프로세스당 아이콘 1개만 허용한다. 그래서 실행 파일이 자기 자신을
`-only claude` · `-only codex` 로 두 번 띄우고 부모는 빠진다. 상세 화면은 어느 쪽을 눌러도
둘을 합쳐 보여 준다(claude 47113 · codex 47114).

## 플래그

| 플래그 | 하는 일 |
|---|---|
| `-once` | 한 줄 요약과 툴팁 내용을 출력하고 끝낸다 |
| `-tokens` | 토큰 집계를 JSON 으로 출력 |
| `-only claude` / `-only codex` | 한쪽만 다룬다 |
| `-icons <폴더>` | 아이콘 견본을 PNG/ICO 로 떨어뜨린다 |
| `-notify` | 알림을 한 번 띄워 본다 |

## 문제 해결

| 증상 | 조치 |
|---|---|
| 아무것도 안 뜬다 | 자격증명이 없다. `usage-tray -once` 로 확인 |
| 이미 실행 중이라고 나온다 | 그 포트를 쓰는 인스턴스가 이미 있다 |
| Codex 가 `기록 N분 전` 으로만 나온다 | 토큰이 만료됐을 수 있다. `codex` 를 한 번 실행 |
| 값이 안 변한다 | 최소 간격 탓이다(3분). 우클릭 → 지금 갱신 |
| 토큰 표가 비어 있다 | 첫 스캔이 아직이다. `-tokens` 로 직접 돌려 본다 |

로그는 데이터 폴더의 `usage-tray.log` 이고 실패 사유만 쌓인다.
