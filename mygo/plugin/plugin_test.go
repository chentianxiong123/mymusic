//go:build native

package plugin

import (
	"testing"
)

// TestRegisterEnabled 注册 + 启用/禁用。
func TestRegisterEnabled(t *testing.T) {
	SetEnabled("source", "testplug", false)
	if IsEnabled("source", "testplug") {
		t.Fatal("应已禁用")
	}
	SetEnabled("source", "testplug", true)
	if !IsEnabled("source", "testplug") {
		t.Fatal("应已启用")
	}
}

// TestRegisterInfo 元数据注册 + 分组查询。
func TestRegisterInfo(t *testing.T) {
	RegisterInfo(Info{Name: "t", Type: "source", Version: "1.0"})
	if InfoOf("source", "t").Name != "t" {
		t.Fatal("InfoOf 未取到")
	}
	if len(InfoNames("source")) == 0 {
		t.Fatal("InfoNames source 应为空?")
	}
}
