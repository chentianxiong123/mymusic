package plugin

import (
	"path/filepath"
	"testing"

	"mymusic-mygo/source"
)

// TestJSPluginLoadDemo 加载 demo-source 示例，验证注册 + 编译期契约。
func TestJSPluginLoadDemo(t *testing.T) {
	errs, err := LoadFromDir(filepath.Join("..", "plugins"))
	if err != nil {
		t.Fatalf("LoadFromDir: %v", err)
	}
	if len(errs) > 0 {
		t.Fatalf("load errs: %v", errs)
	}
	// demo 音源已注册
	src := Sources()["demo"]
	if src == nil {
		t.Fatal("demo source 未注册")
	}
	if src.Name() != "demo" {
		t.Fatalf("Name=%q", src.Name())
	}
	var _ source.MusicSource = src // 编译期契约
	// demo 歌词插件已注册
	if Lyric("demo") == nil {
		t.Fatal("demo lyric 未注册")
	}
	// demo 元数据已注册
	if InfoOf("source", "demo").Name != "demo" {
		t.Fatal("demo info 未注册")
	}
}

// TestJSPluginLoadConflict 与内置 key 同名应拒绝。
func TestJSPluginLoadConflict(t *testing.T) {
	code := `var meta={apiVersion:1,key:"bilibili",name:"x",capabilities:["source"],allowedHosts:["a.com"]};
	function search(c){return{total:0,songs:[]}} function getURL(c){return{url:"http://x"}}`
	if _, err := NewJSPlugin("conflict.js", code); err == nil {
		t.Fatal("应拒绝与内置 key 冲突")
	}
}

// TestJSPluginEvalError 语法错误应报错。
func TestJSPluginEvalError(t *testing.T) {
	if _, err := NewJSPlugin("bad.js", "this is not js {{"); err == nil {
		t.Fatal("应报语法错误")
	}
}

// TestHostAllowed 白名单逻辑（网络限制层）。
func TestHostAllowed(t *testing.T) {
	h := &jsHost{allowedHosts: []string{"music.163.com"}}
	if !h.hostAllowed("music.163.com") {
		t.Error("应允许 music.163.com")
	}
	if h.hostAllowed("evil.com") {
		t.Error("应拒绝 evil.com")
	}
	if h.hostAllowed("sub.music.163.com") {
		t.Error("精确匹配应拒绝子域")
	}
	// 通配 *.163.com
	h2 := &jsHost{allowedHosts: []string{"*.163.com"}}
	if !h2.hostAllowed("music.163.com") {
		t.Error("通配应允许子域")
	}
	// 未声明 = 全拒
	h3 := &jsHost{}
	if h3.hostAllowed("music.163.com") {
		t.Error("未声明 allowedHosts 应全拒")
	}
}

// TestParseLRC LRC 解析（宿主统一）。
func TestParseLRC(t *testing.T) {
	lrc := "[00:00.00]作词\n[00:05.50]第一句\n[01:02.3]第二句\n[02:00]整秒\nbad\n"
	lines := parseLRC(lrc)
	if len(lines) != 4 {
		t.Fatalf("len=%d", len(lines))
	}
	if lines[0].Time != 0 {
		t.Errorf("t0=%v", lines[0].Time)
	}
	if lines[1].Time != 5.5 {
		t.Errorf("t1=%v", lines[1].Time)
	}
	if lines[2].Time != 62.3 {
		t.Errorf("t2=%v", lines[2].Time)
	}
	if lines[3].Time != 120 {
		t.Errorf("t3=%v", lines[3].Time)
	}
	if lines[1].Text != "第一句" {
		t.Errorf("text=%q", lines[1].Text)
	}
	if lines[3].Text != "整秒" {
		t.Errorf("text3=%q", lines[3].Text)
	}
}
