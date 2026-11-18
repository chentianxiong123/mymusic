// Package target 输出目标抽象：本机声卡/音箱/DLNA/蓝牙/插件都实现 OutputTarget。
// 核心原则：复用主线 speaker/cast 域包，这里只做接口抽象 + 注册表，不重写业务。
package target

// Target 输出目标接口：投送当前播放到某个设备。
// UI/播放核心只认这个接口，不认具体设备（音箱/DLNA/本机/蓝牙）。
type Target interface {
	// Name 目标类型名（"speaker"/"cast"/"local"/...）
	Name() string
	// DeviceID 目标设备唯一 id（本机=""）
	DeviceID() string
	// DeviceName 展示名（音箱名/DLNA 名/"本机声卡"）
	DeviceName() string
}

// Registry 目标注册表（插件系统接入点：第三方注册新 Target）。
type Registry struct {
	targets []Target
}

// NewRegistry 创建注册表。
func NewRegistry() *Registry { return &Registry{} }

// Register 注册目标（启动时加载内置 + 插件扫描）。
func (r *Registry) Register(t Target) { r.targets = append(r.targets, t) }

// List 当前可用目标列表。
func (r *Registry) List() []Target { return r.targets }

// Clear 清空（刷新设备时重建）。
func (r *Registry) Clear() { r.targets = r.targets[:0] }
