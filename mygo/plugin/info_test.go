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
