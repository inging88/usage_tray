package main

// 앱 아이콘. 한 장(assets/icon.png, 1024px)에서 나머지를 만든다.
//
//	assets/icon.ico      → rsrc_windows.syso 로 exe 에 박힌다(탐색기·작업표시줄·Alt+Tab).
//	assets/favicon32.png → 상세 화면 파비콘. 여기서 바이너리에 심는다.
//	assets/icon.png      → build.sh 가 맥에서 .icns 로 굽는다(sips·iconutil).
//
// 트레이/메뉴바 아이콘은 이것과 무관하다 — 그쪽은 남은량을 그리는 링 게이지다(icon.go).
//
// 아이콘을 바꾸려면 assets/icon.png 를 갈아 끼우고 나머지를 다시 만든다:
//
//	go run github.com/akavel/rsrc@latest -ico assets/icon.ico -arch amd64 -o rsrc_windows.syso

import _ "embed"

//go:embed assets/favicon32.png
var faviconPNG []byte
