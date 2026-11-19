package plugin

import (
	"path/filepath"
	"testing"
)

func TestLiveKuwo(t *testing.T) {
	SetSettingStore(&memSettingStore{m: map[string]string{}})
	_ = SetSetting("source", "kuwo", "cookie", "test-cookie")
	if _, err := LoadFromDir(filepath.Join("..", "plugins")); err != nil {
		t.Fatalf("LoadFromDir: %v", err)
	}
	src := Sources()["kuwo"]
	if src == nil {
		t.Fatalf("kuwo 源未注册（requiresCookie 需 cookie）")
	}
	songs, total, err := src.Search("晴天", 1, 3)
	if err != nil {
		t.Fatalf("搜索: %v", err)
	}
	if len(songs) == 0 {
		t.Fatalf("无结果")
	}
	t.Logf("酷我搜索 total=%d 首条=%s/%s id=%s album=%s", total, songs[0].Title, songs[0].Artist, songs[0].ID, songs[0].Album)
	if songs[0].ID == "" || songs[0].Title == "" {
		t.Fatalf("字段缺失: %+v", songs[0])
	}
	// 假 cookie 取流：mobi.s 无真实登录 → 可能失败/需登录（预期不 panic，允许 error）。
	if st, err := src.GetStream(songs[0].ID, 1); err == nil {
		if len(st.URL) > 60 {
			t.Logf("酷我取流 OK: %s...", st.URL[:60])
		} else {
			t.Logf("酷我取流 OK: %s", st.URL)
		}
	} else {
		t.Logf("酷我取流 error=%v（假 cookie 预期失败）", err)
	}
}
