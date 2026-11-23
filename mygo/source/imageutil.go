package source

import (
	"image"

	xdraw "golang.org/x/image/draw"
)

// ScaleThumb 缩到长边 target 以内（封面缩略 160px / 模糊背景 12px 共用；纯函数）。
//
// 用 x/image/draw.ApproxBiLinear（优化过的插值）替代旧的逐像素 src.At() 双线性：
// JPEG 解码结果是 *image.YCbCr，src.At() 每像素做 YCbCr→RGB 转换，
// 160×160 缩略图 = 10 万次转换/张，20 张搜索结果封面 → 卡死主线程。
// x/image/draw 对 YCbCr 有快速路径，实测快几十倍。
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
	xdraw.ApproxBiLinear.Scale(dst, dst.Bounds(), src, b, xdraw.Src, nil)
	return dst
}
