//go:build native

package lyric

import "testing"

// 编译期契约：网易云歌词源必须实现 Provider 接口。
var _ Provider = (*Netease)(nil)

// TestNeteaseFetchEmpty 失败路径：空关键词应无词（业务正常），不 panic。
func TestNeteaseFetchEmpty(t *testing.T) {
	p := NewNetease()
	lines, err := p.Fetch(nil, "")
	if err != nil {
		t.Fatalf("空关键词不应报错: %v", err)
	}
	if lines != nil {
		t.Fatalf("空关键词应无词: %v", lines)
	}
}
