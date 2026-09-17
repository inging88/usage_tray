# usage_tray

Claude Code 와 Codex 의 남은 사용량을 **작업표시줄(윈도우) · 메뉴바(맥)** 에 띄운다.

단일 실행 파일이고 설치 과정도 설정 파일도 없다 — 이미 로그인해 둔 자격증명을 읽어
공식 엔드포인트에 물어본다. 윈도우와 맥이 같은 코드로 돈다.

**쓰는 에이전트만 나온다.** Claude Code 만 쓰면 Codex 아이콘은 아예 없고,
둘 다 없으면 실행되지 않는다.

## 한눈에

| | |
|---|---|
| 아이콘 | 링 게이지. 링 길이 = 주간 남은 %, 색 = 브랜드(Claude 주황 · Codex 초록), 20% 미만 빨강 |
| 아이콘 2개 | 트레이 API 는 프로세스당 아이콘 1개다. 그래서 **에이전트마다 프로세스를 하나씩** 띄운다 — 실행 파일을 그냥 실행하면 자기 자신을 `-only claude` · `-only codex` 로 두 번 띄우고 부모는 빠진다 |
| 툴팁·메뉴 | 맡은 에이전트의 창별 남은량과 리셋 시각 — "주간 99% 남음 · 리셋 9/25(금) 08:00 (6일 뒤)" |
| 상세 | 메뉴 → 상세 보기 → 브라우저(claude 47113 · codex 47114). **어느 쪽을 눌러도 둘을 합친 화면**이 뜬다 |
| 알림 | 임계값을 처음 넘을 때 한 번(Windows 풍선 · macOS 알림센터). 회복하면 다시 무장 |
| 쓰는 에이전트만 | 자격증명이 있는 쪽만 나온다. 둘 다 없으면 실행하지 않는다 |

## 빌드

Go 1.22+ 가 필요하다. 의존성은 트레이 라이브러리 하나(`fyne.io/systray`) 뿐이다.

### Windows

```powershell
cd $env:USERPROFILE\.usage-tray\go
powershell -ExecutionPolicy Bypass -File build.ps1        # dist\usage-tray.exe
powershell -ExecutionPolicy Bypass -File build.ps1 -Run   # 빌드하고 바로 실행
```

`CGO_ENABLED=0` 이라 컴파일러가 따로 필요 없다. 결과는 6~8MB 단일 exe.

**로그인 시 자동 실행**: `Win+R` → `shell:startup` → 그 폴더에 `usage-tray.exe` 바로가기를 넣는다.

### macOS

```bash
cd ~/.usage-tray/go
./build.sh            # dist/UsageTray.app + dist/usage-tray
open dist/UsageTray.app
```

**맥에서 직접 빌드해야 한다.** 메뉴바(NSStatusItem)는 Cocoa API 라 cgo 를 거치고, cgo 는 크로스
컴파일이 안 된다. 윈도우에서 맥 바이너리를 만들 수 없다는 뜻이다.
필요한 것은 Xcode Command Line Tools 뿐이다 — `xcode-select --install`. 전체 Xcode 는 필요 없다.

`build.sh` 는 arm64 + amd64 유니버설로 만들고 `.app` 번들에 `LSUIElement=true` 를 넣는다
(Dock 아이콘 없이 메뉴바에만 뜬다). ad-hoc 서명도 한다 — 애플 실리콘은 서명 없는 바이너리를
아예 실행하지 않기 때문이다.

**로그인 시 자동 실행**: 시스템 설정 → 일반 → 로그인 항목 → `+` → `UsageTray.app`.

**다른 맥으로 옮길 때**: 서명이 ad-hoc 이라 Gatekeeper 가 막는다. 받는 쪽에서
`xattr -dr com.apple.quarantine /경로/UsageTray.app` 한 번이면 열린다. 정식 배포로 경고 없이
열리게 하려면 Apple Developer 계정($99/년)으로 공증(notarization)이 필요하다.

## 값은 어디서 오나

| 에이전트 | 출처 | 비고 |
|---|---|---|
| Claude | `GET api.anthropic.com/api/oauth/usage` | 실시간. Claude Code 가 안 떠 있어도 받는다 |
| Codex | `GET chatgpt.com/backend-api/codex/usage` | 실시간. Codex 앱 usage 화면과 같은 값 |

Codex 는 Cloudflare 가 HTTP 스택을 보고 막는 경우가 있다(.NET 은 실제로 403). Go 로 먼저
시도하고, 실패하면 **`curl` 로 한 번 더** 시도한다 — curl 은 Windows 10 1803+ · macOS 기본
탑재다. 그래도 실패하면 rollout 기록으로 폴백하고 `기록 N분 전` 을 붙인다.

창(window)은 이름이 아니라 **길이로 분류**한다. 계정에 따라 `primary_window` 가 5시간이기도
하고 주간이기도 하다. `limit_window_seconds >= 2일` 이면 주간으로 본다.

## 자격증명을 어디서 찾나

| | Windows | macOS |
|---|---|---|
| Claude | `%LOCALAPPDATA%\Claude Code\credentials.json` → `~\.claude\.credentials.json` | `~/.claude/.credentials.json` → `~/Library/Application Support/Claude Code/credentials.json` → **Keychain** |
| Codex | `~\.codex\auth.json` | `~/.codex/auth.json` |

환경변수 `CLAUDE_CODE_OAUTH_TOKEN` 이 있으면 그게 우선이다.

**macOS Keychain 은 실기 확인을 못 했다.** 파일이 없으면 `security find-generic-password -s <서비스명> -w`
로 후보 서비스명(`Claude Code-credentials` · `Claude Code` · `claude-code`)을 차례로 시도한다.
안 잡히면 실제 이름을 찾아 환경변수로 지정한다:

```bash
# 키체인에서 Claude 항목 찾기
security dump-keychain | grep -i -A2 claude | head -20

# 찾은 서비스명을 지정
export USAGE_TRAY_CLAUDE_KEYCHAIN_SERVICE="찾은 이름"
```

## 호출을 아끼는 장치

Claude 사용량 엔드포인트는 자주 부르면 429 를 준다(실측 하루 272건 — 트레이와 statusline 이
같은 곳을 각각 60초마다 부르고 있었다). 세 겹으로 막는다.

1. `statusline.ps1` 이 받아 둔 캐시(`%TEMP%\claude\statusline-usage-cache.json`)가 2분 이내면
   그걸 쓴다 — Claude Code 가 떠 있는 동안에는 API 를 아예 부르지 않는다
2. 직접 부를 때도 최소 간격을 지킨다 (Claude 3분 · Codex 2분). 창은 분 단위로 변하지 않는다
3. 429·503 을 받으면 5분부터 두 배씩, 최대 30분까지 쉰다

조회가 실패해도 화면을 비우지 않는다 — 직전 정상값을 두고 `직전값 12m 전` 처럼 나이만 붙인다.
아이콘이 회색 '값 없음' 으로 깜빡이지 않게 하는 장치다.

## 토큰 집계

`~/.claude/projects/**/*.jsonl` 과 `~/.codex/sessions/**/rollout-*.jsonl` 을 **증분으로** 읽는다.
파일마다 마지막으로 읽은 바이트 위치를 `tokens-cache.json` 에 남기고 다음번엔 그 뒤만 읽는다.
중복 제거 키는 Claude 가 `requestId`, Codex 가 `response_id` 다. 60일 보관.

## 파일이 놓이는 곳

| | 경로 |
|---|---|
| Windows | `%LOCALAPPDATA%\usage-tray\` |
| macOS | `~/Library/Application Support/usage-tray/` |

그 안에 에이전트별로 갈라진 파일이 생긴다 — `state-claude.json` · `state-codex.json`(전체 값),
`status-claude.txt` · `status-codex.txt`(한 줄 요약), `tokens-cache-<agent>.json` · `tokens-<agent>.json`,
그리고 공용 `usage-tray.log`. 두 프로세스가 같은 파일을 동시에 쓰면 깨지므로 갈라 뒀다.
다른 위젯은 `http://127.0.0.1:47113/state.json`(합친 값)을 읽는 게 가장 간단하다.

## 아이콘이 오버플로에 숨을 때 (Windows 11)

새 트레이 아이콘은 기본으로 오버플로(∧) 안에 숨는다. 설정 → 개인 설정 → 작업 표시줄 →
시스템 트레이 아이콘에서 켜거나, 레지스트리 `HKCU\Control Panel\NotifyIconSettings\<id>\IsPromoted = 1` 을
직접 넣는다(`<id>` 는 실행 파일 경로 해시라 각 항목의 `ExecutablePath` 로 찾는다).
적용에는 explorer 재시작이 필요하고, 재시작 뒤에는 트레이 앱도 한 번 다시 실행해야 아이콘이 재등록된다.

## 다른 사람에게 주기

받는 쪽에 필요한 것은 **Claude Code 나 Codex 중 하나에 이미 로그인돼 있는 것** 뿐이다.
설정 파일도, 토큰 입력도, 관리자 권한도 없다. 자기 계정의 자기 사용량만 보인다.

| 전달 방법 | 받는 쪽이 할 일 | 언제 |
|---|---|---|
| 릴리스 링크 | Releases 에서 자기 OS 파일을 받아 실행 | 저장소가 공개이거나 상대가 협업자일 때 |
| 파일만 전달 | 받은 파일을 실행 | 저장소 접근을 주고 싶지 않을 때. 가장 간단하다 |
| 직접 빌드 | `git clone` 후 `build.ps1` 또는 `build.sh` | Go 가 있고 코드를 보겠다는 사람 |

저장소가 **비공개**면 릴리스 파일도 로그인과 권한이 있어야 내려받을 수 있다. 공개로 바꾸거나
(Settings → General → Danger Zone → Change visibility) 협업자로 추가한다
(Settings → Collaborators). 번거로우면 파일만 건네는 편이 빠르다.

### 윈도우에서 처음 실행할 때

1. `usage-tray.exe` 를 아무 폴더에 둔다
2. 더블클릭 → "Windows 의 PC 보호" 창이 뜨면 `추가 정보` → `실행`
   (서명이 없어서 나는 경고다. 파일을 신뢰할 때만 실행한다)
3. 트레이에 링이 생긴다. 안 보이면 오버플로(∧) 안이다 — 위 절 참고
4. 로그인할 때 자동 실행: `Win+R` → `shell:startup` → 그 폴더에 바로가기를 넣는다

### 맥에서 처음 실행할 때

```bash
unzip UsageTray-macos-universal.zip
xattr -dr com.apple.quarantine UsageTray.app   # 서명이 ad-hoc 이라 한 번 필요하다
open UsageTray.app
```

메뉴바에 링이 생긴다. 자동 실행은 시스템 설정 → 일반 → 로그인 항목에 추가한다.

### 확인하고 지우기

`usage-tray -once` 를 터미널에서 돌리면 한 줄로 상태가 나온다. 아무것도 안 뜨면 자격증명이
없는 것이다. 끝낼 때는 아이콘 우클릭 → 끝내기. 지울 때는 실행 파일과 데이터 폴더를 삭제한다 —
그 밖에 남기는 것은 없다.

## 문제 해결

| 증상 | 원인 · 조치 |
|---|---|
| 아무것도 안 뜬다 | 자격증명이 없다. `usage-tray.exe -once` (맥은 `./usage-tray -once`) 로 확인 |
| 이미 실행 중이라고 나온다 | 포트 47113 을 이미 쓰고 있다 = 다른 인스턴스가 떠 있다 |
| Codex 가 `기록 N분 전` 으로만 나온다 | 실시간 조회 실패. 로그를 본다. 토큰 만료면 `codex` 를 한 번 실행 |
| 맥에서 "확인되지 않은 개발자" | `xattr -dr com.apple.quarantine UsageTray.app` |
| 맥에서 Claude 가 안 잡힌다 | Keychain 서비스명 문제. 위 '자격증명' 절 참고 |
| 토큰 표가 비어 있다 | 첫 스캔이 아직이다. `-tokens` 로 직접 돌려 본다 |
