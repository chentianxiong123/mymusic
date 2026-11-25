package plugin

import (
	"path/filepath"
	"testing"

	"github.com/grafana/sobek"
	"mymusic-mygo/source"
)

// TestJSPluginLoadNetease 网易云已改 Go 内置；JS 插件目录里有同 key 时跳过，
// 不重复注册（Go 实现由 main 注册，这里只验证跳过逻辑不报错）。
func TestJSPluginLoadNetease(t *testing.T) {
	errs, err := LoadFromDir(filepath.Join("..", "plugins"))
	if err != nil {
		t.Fatalf("LoadFromDir: %v", err)
	}
	if len(errs) > 0 {
		t.Fatalf("load errs: %v", errs)
	}
	// JS 版 netease 因内置 key 被跳过，不应覆盖歌词 netease。
	if Lyric("netease") != nil {
		t.Fatal("netease 不应注册 lyric 能力（歌词走内置）")
	}
	var _ source.MusicSource = (*source.Netease)(nil) // Go 版编译期契约
}

// TestJSPluginLoadConflict 与内置 key 同名应拒绝。
func TestJSPluginLoadConflict(t *testing.T) {
	code := `var meta={apiVersion:1,key:"bilibili",name:"x",capabilities:["source"],allowedHosts:["a.com"]};
	function search(c){return{total:0,songs:[]}} function getURL(c){return{url:"http://x"}}`
	if _, err := NewJSPlugin("conflict.js", code); err == nil {
		t.Fatal("应拒绝与内置 key 冲突")
	}
}

// TestJSPluginLyricConflict 音源 key=netease 声明 lyric 能力应拒绝
// （内置歌词 netease 已被占，JS 不得覆盖；音源 netease 本身允许）。
func TestJSPluginLyricConflict(t *testing.T) {
	code := `var meta={apiVersion:1,key:"netease",name:"x",capabilities:["source","lyric"],allowedHosts:["a.com"]};
	function search(c){return{total:0,songs:[]}} function getURL(c){return{url:"http://x"}} function lyric(c){return{lrc:""}}`
	if _, err := NewJSPlugin("lyric-conflict.js", code); err == nil {
		t.Fatal("netease 音源声明 lyric 应拒绝（内置歌词冲突）")
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

// TestJSPluginCtxConfigPersistent ctx.config 必须能读到 SettingStore 里的持久化 cookie。
// 回归：之前 makeCtx 注入空 map，所有 JS 插件 ctx.config.get("cookie") 恒为空 = 登录态失效。
func TestJSPluginCtxConfigPersistent(t *testing.T) {
	fs := &fakeStore{}
	SetSettingStore(fs)
	t.Cleanup(func() { SetSettingStore(nil) })
	_ = SetSetting("source", "kuwo", "cookie", "Hm_lvt_test=123")

	code := `var meta={apiVersion:1,key:"kuwo-test",name:"x",capabilities:["source"],allowedHosts:["a.com"]};
	function search(c){return {total:0,songs:[{id:c.config.get("cookie","NONE"),title:"x",artist:"y"}]}}
	function getURL(c){return{url:"http://x"}}`
	jp, err := NewJSPlugin("ctxcfg.js", code)
	if err != nil {
		t.Fatal(err)
	}
	jp.host.withPlugin("source", "kuwo") // 用 kuwo 的设置命名空间
	ctx, err := jp.makeCtx(map[string]interface{}{})
	if err != nil {
		t.Fatal(err)
	}
	_ = jp.vm.Set("ctxcfg", ctx)
	cfgObj := ctx.Get("config").ToObject(jp.vm)
	fn, ok := sobek.AssertFunction(cfgObj.Get("get"))
	if !ok {
		t.Fatal("ctx.config.get 不是函数（config 是空 map）")
	}
	v, err := fn(sobek.Undefined(), jp.vm.ToValue("cookie"), jp.vm.ToValue("NONE"))
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != "Hm_lvt_test=123" {
		t.Fatalf("ctx.config.get(cookie) = %q, want Hm_lvt_test=123", v.String())
	}
}
