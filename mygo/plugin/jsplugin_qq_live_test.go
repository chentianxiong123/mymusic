package plugin

import (
	"path/filepath"
	"testing"
)

// TestLiveQQ 验证 QQ 插件：加载/搜索（匿名可用）/匿名取流（应报需登录）。
func TestLiveQQ(t *testing.T) {
	if _, err := LoadFromDir(filepath.Join("..", "plugins")); err != nil {
		t.Fatalf("LoadFromDir: %v", err)
	}
	src := Sources()["qqmusic"]
	if src == nil {
		t.Fatalf("qqmusic 源未注册")
	}
	if src.Name() != "qqmusic" {
		t.Fatalf("Name()=%q 期望 qqmusic（key 标识；显示名走 InfoOf.Desc）", src.Name())
	}
	songs, total, err := src.Search("晴天", 1, 3)
	if err != nil {
		t.Fatalf("搜索: %v", err)
	}
	if len(songs) == 0 {
		t.Fatalf("无结果")
	}
	t.Logf("QQ 搜索 total=%d 首条=%s/%s mid=%s album=%s", total, songs[0].Title, songs[0].Artist, songs[0].ID, songs[0].Album)
	if songs[0].ID == "" || songs[0].Title == "" {
		t.Fatalf("字段缺失: %+v", songs[0])
	}
	// 匿名取流应报需登录（code=1000 无 purl）。
	_, err = src.GetStream(songs[0].ID, 1)
	if err == nil {
		t.Logf("匿名取流返回了 URL（可能免费试听或接口变化）")
	} else {
		t.Logf("匿名取流 error=%v（预期需登录）", err)
	}
}
