package main

// 트레이 아이콘 — 링 게이지를 그려 바이트로 낸다.
//
//	Windows : ICO 가 필요하다. Vista+ 는 ICO 안에 PNG 를 넣을 수 있어서 PNG 를 감싸 준다.
//	macOS   : PNG 를 그대로 받는다. 메뉴바는 보통 22pt 이므로 44px(레티나) 로 그린다.
//
// 링 길이 = 남은 %, 링 색 = 브랜드, 20% 미만이면 빨강. 숫자는 넣지 않는다(16px 에서 뭉개진다).
// 안티앨리어싱은 4배로 그려 축소하는 식으로 해결한다 — 외부 렌더 라이브러리를 쓰지 않는다.

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"runtime"
)

var (
	colClaude = color.NRGBA{217, 119, 87, 255}
	colCodex  = color.NRGBA{16, 163, 127, 255}
	colDanger = color.NRGBA{239, 68, 68, 255}
)

// size = 최종 픽셀 크기. ss = 슈퍼샘플 배수.
func ringPNG(left int, brand color.NRGBA, size int) []byte {
	const ss = 4
	big := image.NewNRGBA(image.Rect(0, 0, size*ss, size*ss))
	w := float64(size * ss)

	arc := brand
	if left >= 0 && left < 20 {
		arc = colDanger
	}
	track := color.NRGBA{brand.R, brand.G, brand.B, 60}

	cx, cy := w/2, w/2
	thick := w * 0.235
	rOuter := w/2 - w*0.045
	rInner := rOuter - thick

	sweep := 2 * math.Pi * math.Min(100, math.Max(0, float64(left))) / 100
	if left < 0 {
		sweep = 0
	}

	for y := 0; y < int(w); y++ {
		for x := 0; x < int(w); x++ {
			dx, dy := float64(x)+0.5-cx, float64(y)+0.5-cy
			d := math.Hypot(dx, dy)
			if d > rOuter || d < rInner {
				continue
			}
			// 12시에서 시계방향 각도
			ang := math.Atan2(dx, -dy)
			if ang < 0 {
				ang += 2 * math.Pi
			}
			c := track
			if left > 0 && ang <= sweep {
				c = arc
			}
			big.SetNRGBA(x, y, c)
		}
	}
	// 0% 는 호가 없어 '값 없음' 과 구별이 안 된다 — 12시에 점을 찍는다.
	if left == 0 {
		r := thick / 2
		ccx, ccy := cx, cy-(rOuter-thick/2)
		for y := int(ccy - r - 1); y <= int(ccy+r+1); y++ {
			for x := int(ccx - r - 1); x <= int(ccx+r+1); x++ {
				if x < 0 || y < 0 || x >= int(w) || y >= int(w) {
					continue
				}
				if math.Hypot(float64(x)+0.5-ccx, float64(y)+0.5-ccy) <= r {
					big.SetNRGBA(x, y, colDanger)
				}
			}
		}
	}

	small := image.NewNRGBA(image.Rect(0, 0, size, size))
	boxDownscale(small, big, ss)
	var buf bytes.Buffer
	_ = png.Encode(&buf, small)
	return buf.Bytes()
}

// 박스 필터 축소 — image/draw 의 Scaler 없이(x/image 의존을 피한다) 평균을 낸다.
func boxDownscale(dst *image.NRGBA, src *image.NRGBA, ss int) {
	b := dst.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			var r, g, bl, a int
			for sy := 0; sy < ss; sy++ {
				for sx := 0; sx < ss; sx++ {
					c := src.NRGBAAt(x*ss+sx, y*ss+sy)
					// 알파 가중 평균 — 그냥 더하면 투명 픽셀의 색이 번진다.
					r += int(c.R) * int(c.A)
					g += int(c.G) * int(c.A)
					bl += int(c.B) * int(c.A)
					a += int(c.A)
				}
			}
			n := ss * ss
			if a == 0 {
				dst.SetNRGBA(x, y, color.NRGBA{})
				continue
			}
			dst.SetNRGBA(x, y, color.NRGBA{
				R: uint8(r / a), G: uint8(g / a), B: uint8(bl / a), A: uint8(a / n),
			})
		}
	}
}

// PNG 를 ICO 로 감싼다(Vista+ 가 PNG 압축 ICO 를 읽는다).
func pngToICO(pngBytes []byte, size int) []byte {
	var buf bytes.Buffer
	w := byte(size)
	if size >= 256 {
		w = 0
	}
	// ICONDIR
	binary.Write(&buf, binary.LittleEndian, uint16(0)) // reserved
	binary.Write(&buf, binary.LittleEndian, uint16(1)) // type: icon
	binary.Write(&buf, binary.LittleEndian, uint16(1)) // count
	// ICONDIRENTRY
	buf.WriteByte(w)                                               // width
	buf.WriteByte(w)                                               // height
	buf.WriteByte(0)                                               // palette
	buf.WriteByte(0)                                               // reserved
	binary.Write(&buf, binary.LittleEndian, uint16(1))             // planes
	binary.Write(&buf, binary.LittleEndian, uint16(32))            // bpp
	binary.Write(&buf, binary.LittleEndian, uint32(len(pngBytes))) // size
	binary.Write(&buf, binary.LittleEndian, uint32(6+16))          // offset
	buf.Write(pngBytes)
	return buf.Bytes()
}

// 이 OS 의 트레이가 받는 형식으로.
func trayIcon(left int, brand color.NRGBA) []byte {
	if runtime.GOOS == "windows" {
		// 32px 로 그려 넣으면 100%·150% 배율 모두 깔끔하다.
		return pngToICO(ringPNG(left, brand, 32), 32)
	}
	// macOS 메뉴바: 레티나 기준 44px.
	return ringPNG(left, brand, 44)
}

// 상세 페이지에 쓰는 큰 링(HTML 에 base64 로 심는다).
func bigRingPNG(left int, brand color.NRGBA) []byte { return ringPNG(left, brand, 160) }

var _ = draw.Draw // image/draw 는 향후 합성용으로 남겨 둔다
