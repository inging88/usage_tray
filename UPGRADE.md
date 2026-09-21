# UsageTray 1.1.0 — macOS 확장판

원본: https://github.com/inging88/usage_tray

## 사용 방법

기존 UsageTray를 종료하고 새 UsageTray.app을 실행합니다. 기본 상세 화면은 http://127.0.0.1:47113/ 입니다. 메뉴바 링 하나에 연결된 서비스 중 가장 적게 남은 한도를 표시합니다. 상세 화면에는 Claude, Codex, Antigravity를 함께 표시합니다.

- Claude: Claude Code OAuth 로그인과 Claude 데스크톱 OAuth 캐시를 지원합니다. macOS가 `Claude Safe Storage` 접근을 요청하면 허용합니다. Claude 앱이 로그인 토큰을 갱신하므로 앱을 로그인 상태로 유지합니다.
- Claude 키체인 접근이 안 되면: 데스크톱 앱의 24시간 이내 로컬 사용량 기록을 표시하며 기록 나이를 표시합니다. 이 기록에는 리셋 시각이 없으므로 미확인으로 표시합니다. 앱 기록은 실시간 조회가 아닙니다.
- Antigravity: 앱을 실행하고 로그인한 상태에서 모델별 잔여 한도와 리셋 시각을 자동 조회합니다. Antigravity 안에서 사용하는 Claude 모델의 한도와 Claude 구독 한도는 별도입니다. 현재 Antigravity 연결은 macOS만 지원합니다.
- Antigravity 토큰 수·과거 사용량은 수집하지 않습니다. 모델별 할당량을 임의로 토큰 수나 일간/주간 한도로 환산하지 않습니다.

## 변경 사항

- 인증 실패 시 서비스를 숨기지 않고 연결 상태를 표시합니다.
- `CLAUDE_CONFIG_DIR`와 macOS 키체인의 hex 인코딩 자격증명을 처리합니다.
- Claude 데스크톱의 지정된 OAuth 캐시 필드만 메모리에서 복호화합니다. 브라우저 쿠키나 전체 키체인을 검색하지 않습니다. 암호화 키·로그인 토큰은 파일과 로그에 저장하지 않습니다.
- Antigravity의 실행 중인 로컬 서버를 감지해 모델별 사용량을 가져옵니다. 인증 값을 로컬 루프백 주소에만 보내며 리디렉션을 따르지 않습니다.
- Codex의 `rate_limit_reached_type` 필드가 문자열, 객체, null 등으로 바뀌어도 한도 응답 전체가 실패하지 않습니다.
- 마지막 정상 조회의 실제 시각을 유지해 오래된 기록이 계속 새 값처럼 보이지 않게 수정했습니다.
- 초기 토큰 집계 및 알림 상태의 동시 접근을 보완했습니다.

## 조회 주기와 저장 위치

화면 새로고침 30초, 내부 상태 확인 60초, Claude API 최소 3분, Codex·Antigravity 최소 2분, 토큰 기록 집계 5분입니다. Claude 429/503 오류는 최대 30분까지 재시도를 늦춥니다.

기본 데이터: `~/Library/Application Support/usage-tray/`. 원본 앱의 기존 데이터 위치를 유지합니다. macOS 기본 실행은 통합 `state.json`을 사용합니다. 고급 실행 옵션 `-only claude|codex|antigravity`도 지원합니다.

Windows는 예전처럼 에이전트마다 프로세스를 하나씩 띄워 트레이 아이콘을 따로 냅니다(`state-<agent>.json`). 링을 하나로 합치는 것은 macOS 전용 동작이며, 메뉴바에서는 프로세스를 가르면 아이콘이 아예 뜨지 않기 때문입니다.

검증 시 `USAGE_TRAY_DATA_DIR`로 별도 데이터 폴더를 지정할 수 있습니다.

## 빌드와 검증

Go 1.26 이상과 Xcode Command Line Tools가 설치된 macOS에서:

```sh
go test -race ./...
go vet ./...
./build.sh
```

결과는 arm64 + amd64 유니버설 앱이며 로컬 ad-hoc 서명을 사용합니다. Apple 공증 앱은 아닙니다.

사용량 조회는 원본 앱과 마찬가지로 서비스의 비공개 엔드포인트 및 로컬 앱 형식에 의존하므로 향후 서비스 업데이트에 맞춘 수정이 필요할 수 있습니다.
