//go:build native

// appicon —— 应用图标（🎵），各端共用同一份内嵌 PNG。
//
// 为什么内嵌：mygo 各端**打包图标**（Linux .desktop Icon=、Windows 资源、Android mipmap、
// iOS AppIcon）由 CLI 从 `resources/icon.png` 统一生成；但**运行时窗口图标**（Linux/Windows
// 标题栏 + 任务栏）须由 `win.SetIcon` 提供——不给就是"裸 X11 默认图标"。
// 这里把同一份 🎵 内嵌进二进制，两处都不缺。
package main

import (
	_ "embed"
	"log"

	"github.com/egoist/mygo"
)

// appIcon 是 1024×1024 的 🎵 应用图标（主题渐变圆角方 + 白色双八分音符）。
// 既是 `mygo build` 各端打包的源图（mygo.json `icon`），也是运行时窗口图标。
//
//go:embed resources/icon.png
var appIcon []byte

// applyAppIcon 把 🎵 图标设到运行时窗口（Linux/Windows 标题栏 + 任务栏）。
// macOS 窗口自身无图标（Dock 图标由 .icns 打包提供）。失败仅记日志，不影响启动。
//
// 设两次：立即设一次 + `OnReadyToShow`（窗口就绪、native 已建）再设一次——
// `SetIcon` 走 `w.do`（异步 post 主线程），在 `WhenReady` 回调里 native 可能还没建，
// 单次调用会被丢。幂等，重复设无害。
func applyAppIcon(win *mygo.Window) {
	if win == nil {
		return
	}
	set := func() {
		if err := win.SetIcon(appIcon); err != nil {
			log.Printf("[icon] 窗口图标设置失败：%v", err)
		}
	}
	set()
	win.OnReadyToShow(func() { set() })
}
