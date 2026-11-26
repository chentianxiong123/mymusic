package plugin

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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

// memSettingStore 测试用内存设置存储。
type memSettingStore map[string]string

func (m memSettingStore) Set(k, v string) error { m[k] = v; return nil }
func (m memSettingStore) Get(k string) (string, bool) {
	v, ok := m[k]
	return v, ok
}

// TestHostAllowed_HostSettings 用户自填 URL host 自动放行（WebDAV 场景），其余仍拒绝。
func TestHostAllowed_HostSettings(t *testing.T) {
	store := memSettingStore{}
	SetSettingStore(store)
	defer SetSettingStore(nil)

	h := &jsHost{
		allowedHosts: []string{"builtin.example.com"},
		hostSettings: []string{"server_url"},
		pluginType:   "source",
		pluginKey:    "webdav",
	}
	// 未填设置 → 只放行静态白名单
	if h.hostAllowed("nas.local") {
		t.Error("未填 server_url 时不应放行 nas.local")
	}
	// 用户填了 URL → 该 host 放行
	if err := SetSetting("source", "webdav", "server_url", "http://nas.local:5005/dav/"); err != nil {
		t.Fatal(err)
	}
	if !h.hostAllowed("nas.local") {
		t.Error("hostSettings 声明的设置项里的 host 应放行")
	}
	// 只放行填的那个，其它仍拒
	if h.hostAllowed("evil.com") {
		t.Error("只应放行用户填的那一个 host，evil.com 必须拒")
	}
	// 静态白名单仍然有效
	if !h.hostAllowed("builtin.example.com") {
		t.Error("静态白名单应仍然有效")
	}
}

// TestJSPluginLoadWebDAV webdav 插件能加载（hostSettings 声明 + source 能力）。
func TestJSPluginLoadWebDAV(t *testing.T) {
	errs, err := LoadFromDir(filepath.Join("..", "plugins"))
	if err != nil {
		t.Fatalf("LoadFromDir: %v", err)
	}
	if len(errs) > 0 {
		t.Fatalf("load errs: %v", errs)
	}
	if Sources()["webdav"] == nil {
		t.Fatal("webdav 音源未注册")
	}
}

// TestWebDAVPluginSearch 端到端：mock WebDAV PROPFIND → 插件解析 → 过滤/分页。
func TestWebDAVPluginSearch(t *testing.T) {
	// mock WebDAV：根目录下 1 个音乐文件 + 1 个子目录
	const xmlBody = `<?xml version="1.0"?><D:multistatus xmlns:D="DAV:">
  <D:response><D:href>/dav/</D:href><D:propstat><D:prop><D:resourcetype><D:collection/></D:resourcetype></D:prop></D:propstat></D:response>
  <D:response><D:href>/dav/%E6%99%B4%E5%A4%A9.mp3</D:href><D:propstat><D:prop><D:resourcetype/></D:prop></D:propstat></D:response>
  <D:response><D:href>/dav/sub/</D:href><D:propstat><D:prop><D:resourcetype><D:collection/></D:resourcetype></D:prop></D:propstat></D:response>
</D:multistatus>`
	var propfindCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		propfindCount++
		if r.Method != "PROPFIND" {
			t.Errorf("want PROPFIND, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(207)
		if strings.HasPrefix(r.URL.Path, "/dav/sub") { // 子目录里再来一首
			w.Write([]byte(`<D:multistatus xmlns:D="DAV:">
  <D:response><D:href>/dav/sub/song2.flac</D:href><D:propstat><D:prop><D:resourcetype/></D:prop></D:propstat></D:response>
</D:multistatus>`))
			return
		}
		w.Write([]byte(xmlBody))
	}))
	defer srv.Close()

	store := memSettingStore{}
	SetSettingStore(store)
	defer SetSettingStore(nil)
	_ = SetSetting("source", "webdav", "server_url", srv.URL+"/dav/")
	_ = SetSetting("source", "webdav", "music_path", "")
	_ = SetSetting("source", "webdav", "searchSize", "40")

	code, err := os.ReadFile(filepath.Join("..", "plugins", "webdav", "plugin.js"))
	if err != nil {
		t.Fatal(err)
	}
	jp, err := NewJSPlugin("webdav.js", string(code))
	if err != nil {
		t.Fatal(err)
	}
	src := &jsSource{jp: jp}
	songs, total, err := src.Search("晴天", 1, 40)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if total != 1 || len(songs) != 1 {
		t.Fatalf("关键词 晴天: total=%d songs=%d, want 1/1", total, len(songs))
	}
	if !strings.Contains(songs[0].Title, "晴天") {
		t.Errorf("标题=%q, want 含晴天", songs[0].Title)
	}
	// 空关键词 → 全部 2 首（根目录 mp3 + 子目录 flac），验证递归
	songs, total, err = src.Search("", 1, 40)
	if err != nil {
		t.Fatalf("search all: %v", err)
	}
	if total != 2 || len(songs) != 2 {
		t.Fatalf("全部: total=%d songs=%d, want 2/2 (递归子目录)", total, len(songs))
	}
	// 缓存：第二次搜索不应重新 PROPFIND（第一次 2 次调用 = 根 + 子目录）
	callsAfterFirst := propfindCount
	_, _, err = src.Search("晴天", 1, 40)
	if err != nil {
		t.Fatal(err)
	}
	if propfindCount != callsAfterFirst {
		t.Errorf("缓存未命中：第二次搜索又发了 PROPFIND（%d → %d）", callsAfterFirst, propfindCount)
	}
}
