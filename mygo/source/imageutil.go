package source

import (
	"image"
	"image/color"
)

// ScaleThumb 双线性缩到长边 target 以内（无第三方依赖；纯函数，核心层通用——谁都能用）。
// 封面缩略（160px）/背景模糊降采样（12px）复用；非插件层，任何调用方 import 即可。
func ScaleThumb(src image.Image, target int) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= target && h <= target {
		return src
	}
	nw, nh := target, target
	if w > h {
		nh = h * target / w
	} else {
		nw = w * target / h
	}
	if nw < 1 {
		nw = 1
	}
	if nh < 1 {
		nh = 1
	}
	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	w1, h1 := w-1, h-1
	for y := 0; y < nh; y++ {
		syf := float64(y) * float64(h1) / float64(nh-1)
		y0, y1 := int(syf), min(int(syf)+1, h1)
		fy := syf - float64(y0)
		for x := 0; x < nw; x++ {
			sxf := float64(x) * float64(w1) / float64(nw-1)
			x0, x1 := int(sxf), min(int(sxf)+1, w1)
			fx := sxf - float64(x0)
			dst.Set(x, y, blerp(src.At(x0, y0), src.At(x1, y0), src.At(x0, y1), src.At(x1, y1), fx, fy))
		}
	}
	return dst
}

// blerp 双线性插值（RGBA 分量）。
func blerp(c00, c01, c10, c11 color.Color, fx, fy float64) color.Color {
	r00, g00, b00, a00 := c00.RGBA()
	r01, g01, b01, a01 := c01.RGBA()
	r10, g10, b10, a10 := c10.RGBA()
	r11, g11, b11, a11 := c11.RGBA()
	lerp2 := func(a, b, c, d float64) float64 {
		return (a+(b-a)*fx)*(1-fy) + (c+(d-c)*fx)*fy
	}
	return color.NRGBA{R: uint8(lerp2(float64(r00>>8), float64(r01>>8), float64(r10>>8), float64(r11>>8))),
		G: uint8(lerp2(float64(g00>>8), float64(g01>>8), float64(g10>>8), float64(g11>>8))),
		B: uint8(lerp2(float64(b00>>8), float64(b01>>8), float64(b10>>8), float64(b11>>8))),
		A: uint8(lerp2(float64(a00>>8), float64(a01>>8), float64(a10>>8), float64(a11>>8)))}
}
