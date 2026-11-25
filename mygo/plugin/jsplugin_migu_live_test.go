//go:build live

package plugin

import (
	hmodel "homecast/model"
	"path/filepath"
	"testing"
)

func TestLiveMigu(t *testing.T) {
	if _, err := LoadFromDir(filepath.Join("..", "plugins")); err != nil {
		t.Fatalf("LoadFromDir: %v", err)
	}
	src := Sources()["migu"]
	if src == nil {
		t.Fatalf("migu 源未注册")
	}
	songs, total, err := src.Search("晴天", 1, 3)
	if err != nil {
		t.Fatalf("搜索: %v", err)
	}
	if len(songs) == 0 {
		t.Fatalf("无结果")
	}
	t.Logf("咪咕搜索 total=%d 首条=%s/%s id=%s album=%s", total, songs[0].Title, songs[0].Artist, songs[0].ID, songs[0].Album)
	st, err := src.GetStream(hmodel.SongRef{ID: songs[0].ID}, 1)
	if err != nil {
		t.Fatalf("免登录取流失败: %v", err)
	}
	if st.URL == "" {
		t.Fatalf("取流 URL 为空")
	}
	t.Logf("咪咕免登录取流 OK: %s...", st.URL[:60])
}
