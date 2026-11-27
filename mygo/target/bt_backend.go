// Package target 蓝牙后端抽象：各端用自己的蓝牙/音频路由 API，目标插件只认接口。
//
// 设计：
//   - BtBackend 定义统一能力（发现/连接/断开/状态/音频路由/恢复本机）
//   - 各 GOOS 一个内置实现（bt_linux/darwin/windows/android/other.go）
//   - 第三方可 RegisterBtBackend 覆盖（同名后注册优先）
//   - 无可用后端时优雅降级（UI 提示，不 panic）
package target

// BtDevice 蓝牙设备信息（各端统一 DTO）。
type BtDevice struct {
	MAC        string // 设备唯一 id（Linux=MAC，macOS=地址，Windows=InstanceId，Android=地址）
	Name       string // 设备名
	Connected  bool   // 是否已连接
	Supported  bool   // 是否是音频设备（过滤鼠标/键盘）
	Ready      bool   // 是否真的可投（已连接 + 系统有音频输出端点）——只有这个才显示"音频到此"
}

// BtBackend 蓝牙平台后端：封装各端蓝牙栈 + 音频路由 API。
type BtBackend interface {
	Name() string                     // 平台标识（"linux-bluez"/"macos-blueutil"/...）
	Available() bool                  // 依赖命令/系统 API 是否就绪
	List() []BtDevice                 // 已配对设备（含连接状态）
	Connect(id string) error          // 连接设备
	Disconnect(id string) error       // 断开设备
	RouteTo(id string) error          // 把音频输出路由到该设备
	RouteToLocal() error              // 恢复本机默认输出
}

var btBackends []BtBackend

// RegisterBtBackend 注册平台后端（同名覆盖，后注册优先）。
// 平台内置后端在各自 bt_*.go 的 init 注册；第三方插件可在运行时覆盖。
func RegisterBtBackend(b BtBackend) {
	if b == nil {
		return
	}
	for i, x := range btBackends {
		if x.Name() == b.Name() {
			btBackends[i] = b
			return
		}
	}
	btBackends = append(btBackends, b)
}

// BtBackends 所有已注册后端（UI 可展示诊断）。
func BtBackends() []BtBackend {
	out := make([]BtBackend, len(btBackends))
	copy(out, btBackends)
	return out
}

// Bt 当前可用后端（第一个 Available；无则 nil）。
func Bt() BtBackend {
	for _, b := range btBackends {
		if b.Available() {
			return b
		}
	}
	return nil
}

// BtAvailable 是否有可用蓝牙后端。
func BtAvailable() bool { return Bt() != nil }
