// 蓝牙目标插件：平台无关包装，实际蓝牙/路由操作委托给 BtBackend（各端实现）。
package target

import "homecast/model"

// Bluetooth 蓝牙音箱投送目标（每台设备一个实例）。
// Play = 路由音频到蓝牙 sink；Stop = 恢复本机 sink。
type Bluetooth struct {
	id   string
	name string
}

// NewBluetooth 创建蓝牙目标插件（id=设备标识，name=设备名）。
func NewBluetooth(id, name string) *Bluetooth {
	return &Bluetooth{id: id, name: name}
}

func (t *Bluetooth) Name() string       { return "bluetooth" }
func (t *Bluetooth) DeviceID() string   { return t.id }
func (t *Bluetooth) DeviceName() string { return t.name }

func (t *Bluetooth) Play(ref model.SongRef, opts PlayOpts) error {
	b := Bt()
	if b == nil {
		return &BtUnsupported{}
	}
	return b.RouteTo(t.id)
}

func (t *Bluetooth) Stop() error {
	b := Bt()
	if b == nil {
		return &BtUnsupported{}
	}
	return b.RouteToLocal()
}

// Connect/Disconnect 供 UI 直接操作设备（不走 Target 接口）。
func (t *Bluetooth) Connect() error {
	b := Bt()
	if b == nil {
		return &BtUnsupported{}
	}
	return b.Connect(t.id)
}

func (t *Bluetooth) Disconnect() error {
	b := Bt()
	if b == nil {
		return &BtUnsupported{}
	}
	return b.Disconnect(t.id)
}

// ScanBluetooth 发现蓝牙设备（平台后端 List；无可用后端返回 nil）。
func ScanBluetooth() []BtDevice {
	b := Bt()
	if b == nil {
		return nil
	}
	return b.List()
}

// BtUnsupported 当前平台没有可用蓝牙后端。
type BtUnsupported struct{}

func (e *BtUnsupported) Error() string { return "当前平台不支持蓝牙输出" }
