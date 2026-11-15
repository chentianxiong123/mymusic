//go:build native

// scale：MyGo 跟随系统文字缩放（对齐主线 GUI wails3 WebKit 的 text-scaling-factor）。
// 主线 GUI（WebKit/浏览器）跟随 GNOME text-scaling-factor 放大文字；MyGo（GTK）只认
// 整数 GDK scale（gtkWidgetGetScaleFactor，150% 非整数 → 1x 不放大）→ 组件大小跟主线
// 不一致。这里读 text-scaling-factor（如 1.5）作全局 uiScale，渲染 px 统一乘它。
package main

import (
	"os/exec"
	"strconv"
	"strings"
)

// uiScale 全局渲染缩放因子（读 GNOME text-scaling-factor；默认 1）。
var uiScale float32 = 1

func init() {
	// gsettings get org.gnome.desktop.interface text-scaling-factor → "1.5"
	out, err := exec.Command("gsettings", "get", "org.gnome.desktop.interface", "text-scaling-factor").Output()
	if err != nil {
		return
	}
	v, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 32)
	if err != nil || v < 0.5 || v > 3 {
		return
	}
	uiScale = float32(v)
}
