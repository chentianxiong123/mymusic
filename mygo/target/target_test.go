//go:build native

package target

import "testing"

// 编译期契约：四类目标必须实现 Target 接口（编译器强制）。
var (
	_ Target = (*LocalPlayer)(nil)
	_ Target = (*XiaoaiSpeaker)(nil)
	_ Target = (*CastDevice)(nil)
	_ Target = (*Bluetooth)(nil)
)

// TestPlayErr 投送业务错误（code!=0）。
func TestPlayErr(t *testing.T) {
	e := &PlayErr{"音箱不可用"}
	if e.Error() != "音箱不可用" {
		t.Fatalf("PlayErr 消息错: %q", e.Error())
	}
}

// TestBluetoothPlayNoSink 失败路径：未连接蓝牙（无 sink）应报错不 panic。
func TestBluetoothPlayNoSink(t *testing.T) {
	b := NewBluetooth("00:00:00:00:00:00", "测试")
	if err := b.Play("BV1", PlayOpts{}); err == nil {
		t.Fatal("无 sink 应报错")
	}
}
