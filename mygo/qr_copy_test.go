//go:build native

// qr_copy_test.go —— 二维码复制内容有效性测试（不碰网络/剪贴板）。
package main

import (
	"bytes"
	"testing"

	"github.com/egoist/mygo/ui"
)

// 复制出去的必须是真 PNG（能被 mygo 解码），否则粘贴到聊天窗口是坏图。
func TestQRPNGIsDecodable(t *testing.T) {
	png, err := qrPNG("https://example.com/login?codekey=test", 256)
	if err != nil {
		t.Fatalf("qrPNG 生成失败: %v", err)
	}
	if len(png) == 0 {
		t.Fatal("qrPNG 返回空数据")
	}
	// PNG 魔数
	if !bytes.HasPrefix(png, []byte{0x89, 'P', 'N', 'G'}) {
		t.Fatalf("qrPNG 不是 PNG 格式，前缀=%v", png[:8])
	}
	img, err := ui.DecodeImage(png)
	if err != nil || img == nil {
		t.Fatalf("复制的二维码 PNG 无法解码: %v", err)
	}
}

// 服务器直返图片（QQ/咪咕）的复制路径：s.Image 非空时应直接作为 PNG 复制，
// 不能再二次编码（会丢 qrsig 对应关系）。这里验证直返 PNG 同样可解码。
func TestQRServerImageCopyable(t *testing.T) {
	png, err := qrPNG("qq-qrsig-direct-image", 256)
	if err != nil {
		t.Fatal(err)
	}
	img, derr := ui.DecodeImage(png)
	if derr != nil || img == nil {
		t.Fatalf("服务器直返二维码 PNG 解码失败: %v", derr)
	}
}
