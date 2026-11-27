//go:build native

// target_panel_test.go —— 目标页插件派生契约测试。
package main

import (
	"reflect"
	"testing"

	"mymusic-mygo/plugin"
)

// 目标页必须完全由插件面板派生：TargetPanelHost 只允许面板真正用到的方法，
// 防止内部状态（QR/token/登录）泄漏进接口，逼第三方实现一堆无关方法。
func TestTargetPanelHostSlim(t *testing.T) {
	typ := reflect.TypeOf((*plugin.TargetPanelHost)(nil)).Elem()
	n := typ.NumMethod()
	if n > 25 {
		t.Fatalf("TargetPanelHost 膨胀到 %d 个方法（上限 25）：接口应收口，只留面板渲染真正用到的能力", n)
	}
	t.Logf("TargetPanelHost 方法数 = %d（上限 25）", n)
}

// 面板注册表非空且 Order 有稳定排序（注册顺序 + 排序不抖动）。
func TestTargetPanelsRegistered(t *testing.T) {
	panels := plugin.TargetPanels()
	if len(panels) == 0 {
		t.Fatal("没有注册任何目标面板")
	}
	for i := 1; i < len(panels); i++ {
		if panels[i].Order < panels[i-1].Order {
			t.Fatalf("面板未按 Order 排序: %s(%d) 在 %s(%d) 前",
				panels[i-1].Kind, panels[i-1].Order, panels[i].Kind, panels[i].Order)
		}
	}
}
