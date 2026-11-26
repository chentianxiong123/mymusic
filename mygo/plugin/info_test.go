//go:build native

package plugin

import "testing"

func TestRegisterInfoMergeCapabilities(t *testing.T) {
	infos = map[string]Info{}
	enabled = map[string]bool{}
	orderOverride = map[string]int{}
	RegisterInfo(Info{Name: "netease", Type: "source", Icon: "netease", Order: 20})
	RegisterInfo(Info{Name: "netease", Type: "lyric", Desc: "歌词"})
	got := InfoOf("", "netease")
	if len(got.Capabilities) != 2 || !HasCapability("netease", "source") || !HasCapability("netease", "lyric") {
		t.Fatalf("能力应合并: %+v", got.Capabilities)
	}
	if len(AllInfos()) != 1 {
		t.Fatalf("同平台应只有一个条目, got %d", len(AllInfos()))
	}
	if len(InfoNames("source")) != 1 || len(InfoNames("lyric")) != 1 {
		t.Fatal("按能力过滤应各命中一次")
	}
}

func TestAllInfosDeterministic(t *testing.T) {
	infos = map[string]Info{}
	orderOverride = map[string]int{}
	// 多个同 Order 平台（模拟 map 遍历随机）
	for _, n := range []string{"netease", "local", "cast", "bluetooth", "speaker", "bilibili"} {
		RegisterInfo(Info{Name: n, Type: "source", Order: 20})
	}
	first := AllInfos()
	for i := 0; i < 50; i++ {
		got := AllInfos()
		if len(got) != len(first) {
			t.Fatalf("长度变了")
		}
		for j := range got {
			if got[j].Name != first[j].Name {
				t.Fatalf("第 %d 次调用顺序变了: %s != %s", i, got[j].Name, first[j].Name)
			}
		}
	}
}
