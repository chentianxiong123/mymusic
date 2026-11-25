//go:build live

package plugin

import (
	"path/filepath"
	"testing"
)

func TestLiveMiguLyric(t *testing.T) {
	SetSettingStore(&memSettingStore{m: map[string]string{}})
	if _, err := LoadFromDir(filepath.Join("..", "plugins")); err != nil {
		t.Fatalf("LoadFromDir: %v", err)
	}
	prov := Lyric("migu")
	if prov == nil {
		t.Fatalf("migu 歌词未注册")
	}
	lines, err := prov.Fetch([]string{"晴天", "周杰伦"}, "600902000006889366")
	if err != nil {
		t.Logf("咪咕歌词 error=%v", err)
		return
	}
	if len(lines) == 0 {
		t.Fatalf("咪咕歌词空")
	}
	t.Logf("咪咕歌词 %d 行，首行: %s", len(lines), lines[0].Text)
}
