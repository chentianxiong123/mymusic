//go:build native

package target

import "testing"

// 编译期契约：三类目标必须实现 Target 接口（编译器强制）。
var (
	_ Target = (*LocalPlayer)(nil)
	_ Target = (*XiaoaiSpeaker)(nil)
	_ Target = (*CastDevice)(nil)
)

// TestPlayErr 投送业务错误（code!=0）。
func TestPlayErr(t *testing.T) {
	e := &PlayErr{"音箱不可用"}
	if e.Error() != "音箱不可用" {
		t.Fatalf("PlayErr 消息错: %q", e.Error())
	}
}
