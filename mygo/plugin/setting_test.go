package plugin

import (
	"testing"
)

// fakeStore 内存设置存储（测试注入）。
type fakeStore struct {
	m map[string]string
}

func (f *fakeStore) Set(k, v string) error {
	if f.m == nil {
		f.m = map[string]string{}
	}
	f.m[k] = v
	return nil
}

func (f *fakeStore) Get(k string) (string, bool) {
	if f.m == nil {
		return "", false
	}
	v, ok := f.m[k]
	return v, ok
}

// TestSettingRegisterAndStore 设置声明注册 + 系统统一存储读写。
func TestSettingRegisterAndStore(t *testing.T) {
	fs := &fakeStore{}
	SetSettingStore(fs)

	// 注册设置声明（Go 插件）
	RegisterSettings("source", "testplug", []Setting{
		{Key: "quality", Name: "音质", Type: SettingSelect, Options: []string{"128k", "320k"}, Default: "320k"},
		{Key: "enabled", Name: "启用", Type: SettingBool, Default: "true"},
	})
	items := SettingsOf("source", "testplug")
	if len(items) != 2 {
		t.Fatalf("settings len=%d", len(items))
	}

	// 默认值（未存）
	if v := GetSetting("source", "testplug", "quality", "320k"); v != "320k" {
		t.Errorf("default quality=%q", v)
	}
	// 写 + 读（落 store）
	if err := SetSetting("source", "testplug", "quality", "128k"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if v := GetSetting("source", "testplug", "quality", "320k"); v != "128k" {
		t.Errorf("read quality=%q", v)
	}
	// 命名空间 key 正确（plugin:source/testplug/quality）
	if _, ok := fs.m["plugin:source/testplug/quality"]; !ok {
		t.Errorf("store key 错误: %v", fs.m)
	}
	// bool/int/float 便捷
	SetSettingBool("source", "testplug", "enabled", false)
	if GetSettingBool("source", "testplug", "enabled", true) {
		t.Error("bool 应 false")
	}
	SetSettingInt("source", "testplug", "n", 42)
	if GetSettingInt("source", "testplug", "n", 0) != 42 {
		t.Error("int 应 42")
	}
	SetSettingFloat("source", "testplug", "f", 0.5)
	if GetSettingFloat("source", "testplug", "f", 0) != 0.5 {
		t.Error("float 应 0.5")
	}
}

// TestSettingNilStore 无 store（内存模式）读默认值。
func TestSettingNilStore(t *testing.T) {
	SetSettingStore(nil)
	if v := GetSetting("source", "x", "k", "def"); v != "def" {
		t.Errorf("nil store 应返回默认 def, got %q", v)
	}
}

// TestJSSettingConfig host.config 读写（JS 插件设置走系统统一存储）。
func TestJSSettingConfig(t *testing.T) {
	fs := &fakeStore{}
	SetSettingStore(fs)
	// 插件声明设置（模拟 JS meta.settings 注册）
	RegisterSettings("source", "demo", []Setting{
		{Key: "quality", Name: "音质", Type: SettingSelect, Options: []string{"128k", "320k"}, Default: "320k"},
	})
	// 构造 JS 插件（demo 已注册为内置冲突检查——用非内置 key）
	code := `var meta={apiVersion:1,key:"jsset",name:"jsset",capabilities:["source"],allowedHosts:["a.com"],
	  settings:[{key:"quality",name:"音质",type:"select",options:["128k","320k"],default:"320k"}]};
	  function search(c){return{total:0,songs:[]}} function getURL(c){return{url:"http://x"}}`
	jp, err := NewJSPlugin("jsset.js", code)
	if err != nil {
		t.Fatalf("NewJSPlugin: %v", err)
	}
	// 绑定身份后，config 读写应走 store
	jp.host.withPlugin("source", "jsset")
	// 读默认（未存）
	if v := jp.host.configGet("quality", "320k"); v != "320k" {
		t.Errorf("config 默认=%q", v)
	}
	// 写
	if err := jp.host.configSet("quality", "128k"); err != nil {
		t.Fatalf("configSet: %v", err)
	}
	if v := jp.host.configGet("quality", "320k"); v != "128k" {
		t.Errorf("config 读=%q", v)
	}
	// store 落库（命名空间 plugin:source/jsset/quality）
	if _, ok := fs.m["plugin:source/jsset/quality"]; !ok {
		t.Errorf("store 未落库: %v", fs.m)
	}
}

// TestJSPluginRegistersSettings 加载 JS 插件时自动注册 meta.settings。
func TestJSPluginRegistersSettings(t *testing.T) {
	SetSettingStore(nil)
	code := `var meta={apiVersion:1,key:"regset",name:"regset",capabilities:["source"],allowedHosts:["a.com"],
	  settings:[{key:"quality",name:"音质",type:"select",options:["128k","320k"],default:"320k"}]};
	  function search(c){return{total:0,songs:[]}} function getURL(c){return{url:"http://x"}}`
	if _, err := NewJSPlugin("regset.js", code); err != nil {
		t.Fatalf("NewJSPlugin: %v", err)
	}
	items := SettingsOf("source", "regset")
	if len(items) != 1 || items[0].Key != "quality" || items[0].Default != "320k" {
		t.Fatalf("meta.settings 未注册: %+v", items)
	}
}
