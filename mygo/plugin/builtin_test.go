package plugin

// 系统级能力清单测试：验证「系统级 = Go 内置」的 key 受保护、JS 插件不可覆盖。

import (
	hmodel "homecast/model"
	"testing"

	"mymusic-mygo/model"
	"mymusic-mygo/source"
	"mymusic-mygo/target"
)

// --- 测试替身（tag 用于区分"系统实例"与"后注册的同名实例"） ---

type stubSource struct{ n, tag string }

func (s stubSource) Name() string { return s.n }
func (s stubSource) Search(keyword string, page, size int) ([]source.Song, int, error) {
	return nil, 0, nil
}
func (s stubSource) GetStream(ref hmodel.SongRef, quality int) (*source.Stream, error) {
	return nil, nil
}
func (s stubSource) GetLyric(ref hmodel.SongRef, keywords []string, sid int) ([]model.LyricLine, error) {
	return nil, nil
}

type stubLyric struct{ n, tag string }

func (l stubLyric) Name() string { return l.n }
func (l stubLyric) Fetch(keywords []string, sid string) ([]model.LyricLine, error) {
	return nil, nil
}

type stubTarget struct{ n, tag string }

func (t stubTarget) Name() string                                   { return t.n }
func (t stubTarget) DeviceID() string                               { return "" }
func (t stubTarget) DeviceName() string                             { return t.n }
func (t stubTarget) Play(ref hmodel.SongRef, opts target.PlayOpts) error { return nil }
func (t stubTarget) Stop() error                                    { return nil }

// TestSystemLyricKeyProtected 系统级词源 key 受保护：校验报错 + 公共入口拒绝覆盖。
func TestSystemLyricKeyProtected(t *testing.T) {
	const key = "netease"
	t.Cleanup(func() { delete(lyrics, key) })
	RegisterSystemLyric(stubLyric{n: key, tag: "system"})

	if !IsBuiltinKey("lyric", key) {
		t.Fatal("netease 应是系统级词源")
	}
	if err := CheckPluginKey("lyric", key); err == nil {
		t.Fatal("CheckPluginKey 应拒绝系统级词源 key（JS 插件不可占用）")
	}
	// 公共入口（JS/第三方用）：拒绝覆盖，系统实现必须保持不动
	RegisterLyric(stubLyric{n: key, tag: "js"})
	if got := lyrics[key].(stubLyric).tag; got != "system" {
		t.Errorf("系统级词源被覆盖：tag=%q，期望仍为 %q", got, "system")
	}
}

// TestSystemSourceKeyProtected 系统级音源 key 受保护。
// 用 local 验证：此前硬编码清单漏登记 local，JS 插件取 key "local" 可静默覆盖本机源。
func TestSystemSourceKeyProtected(t *testing.T) {
	const key = "local"
	t.Cleanup(func() { delete(sources, key) })
	RegisterSystemSource(stubSource{n: key, tag: "system"})

	if err := CheckPluginKey("source", key); err == nil {
		t.Fatal("CheckPluginKey 应拒绝系统级音源 key")
	}
	RegisterSource(stubSource{n: key, tag: "js"})
	if got := Sources()[key].(stubSource).tag; got != "system" {
		t.Errorf("系统级音源被覆盖：tag=%q，期望仍为 %q", got, "system")
	}
}

// TestSystemTargetKeyProtected 系统级播放目标 key 受保护。
func TestSystemTargetKeyProtected(t *testing.T) {
	const key = "cast"
	t.Cleanup(func() { delete(targets, key+"/") })
	RegisterSystemTarget(stubTarget{n: key, tag: "system"})

	if err := CheckPluginKey("target", key); err == nil {
		t.Fatal("CheckPluginKey 应拒绝系统级目标 key")
	}
	RegisterTarget(stubTarget{n: key, tag: "js"})
	if got := Targets()[key+"/"].(stubTarget).tag; got != "system" {
		t.Errorf("系统级目标被覆盖：tag=%q，期望仍为 %q", got, "system")
	}
}

// TestCheckPluginKeyAllowsNonBuiltin 非系统级 key 正常放行（不误伤 JS 插件）。
func TestCheckPluginKeyAllowsNonBuiltin(t *testing.T) {
	for _, kind := range []string{"source", "lyric", "target"} {
		key := "stub-" + kind + "-free"
		if IsBuiltinKey(kind, key) {
			t.Fatalf("%s/%s 不应是系统级", kind, key)
		}
		if err := CheckPluginKey(kind, key); err != nil {
			t.Errorf("CheckPluginKey(%s,%s) 误报: %v", kind, key, err)
		}
	}
}

// TestSystemCapabilityKeys 清单内容与 main.go 的实际系统注册一致（8 个能力）。
func TestSystemCapabilityKeys(t *testing.T) {
	want := map[string][]string{
		"source": {"bilibili", "local", "netease"},
		"lyric":  {"netease"},
		"target": {"bluetooth", "cast", "local", "speaker"},
	}
	got := SystemCapabilityKeys()
	for kind, keys := range want {
		if !setEqual(got[kind], keys) {
			t.Errorf("SystemCapabilityKeys()[%s]=%v，期望 %v", kind, got[kind], keys)
		}
	}
}

// TestEnsureSystemKeyPanicsOnUndeclared 新增 Go 能力却忘了登记清单 → 当场 panic（防静默漂移）。
func TestEnsureSystemKeyPanicsOnUndeclared(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("未声明的 key 走 RegisterSystem* 应 panic（防漂移），实际没 panic")
		}
	}()
	RegisterSystemLyric(stubLyric{n: "undeclared-lyr"})
}

func setEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	am := map[string]bool{}
	for _, x := range a {
		am[x] = true
	}
	for _, x := range b {
		if !am[x] {
			return false
		}
	}
	return true
}
