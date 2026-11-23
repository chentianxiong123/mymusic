// Package widgetoverlay 安卓悬浮歌词挂件：Go 侧启动/停止 Kotlin Service。
// Kotlin 源 + mygo-plugin.json 在同目录，mygo build 扫描 import 图时自动注入 APK。
// 桌面/其它平台 mygo.Native 返回 errors.ErrUnsupported，调用方据此隐藏开关。
package widgetoverlay

import (
	"context"
	"log"

	"github.com/egoist/mygo"
)

// Supported 当前平台是否支持浮窗（仅 Android 有 Kotlin 实现）。
// mygo.Native 永远非 nil，靠实际调用是否 ErrUnsupported 判断。
func Supported() bool {
	var out struct {
		Granted bool `json:"granted"`
	}
	return mygo.Native("lyricoverlay").Call(context.Background(), "granted", nil, &out) == nil
}

// Granted 是否已获得"显示在其它应用上层"权限。
func Granted() (bool, error) {
	var out struct {
		Granted bool `json:"granted"`
	}
	if err := mygo.Native("lyricoverlay").Call(context.Background(), "granted", nil, &out); err != nil {
		return false, err
	}
	return out.Granted, nil
}

// Start 启动浮窗（未授权时返回 false，调用方应引导去设置页）。
func Start() (bool, error) {
	var out struct {
		Granted bool `json:"granted"`
	}
	if err := mygo.Native("lyricoverlay").Call(context.Background(), "start", nil, &out); err != nil {
		return false, err
	}
	return out.Granted, nil
}

// Stop 停止浮窗。
func Stop() error {
	return mygo.Native("lyricoverlay").Call(context.Background(), "stop", nil, nil)
}

// OpenSettings 打开悬浮窗权限设置页。
func OpenSettings() error {
	return mygo.Native("lyricoverlay").Call(context.Background(), "openSettings", nil, nil)
}

// TryStart 一键启动：未授权自动拉起设置页（返回 false 表示需用户授权）。
func TryStart() bool {
	ok, err := Start()
	if err != nil {
		log.Printf("[overlay] start: %v", err)
		return false
	}
	if ok {
		return true
	}
	if err := OpenSettings(); err != nil {
		log.Printf("[overlay] open settings: %v", err)
	}
	return false
}
