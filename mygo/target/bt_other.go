//go:build !linux && !darwin && !windows && !android

// bt_other.go —— 其它平台（iOS/harmony/web）：无蓝牙后端，UI 优雅降级。
package target

type btNone struct{}

func init() { RegisterBtBackend(&btNone{}) }

func (b *btNone) Name() string         { return "none" }
func (b *btNone) Available() bool      { return false }
func (b *btNone) List() []BtDevice     { return nil }
func (b *btNone) Connect(string) error { return unsupported() }
func (b *btNone) Disconnect(string) error { return unsupported() }
func (b *btNone) RouteTo(string) error    { return unsupported() }
func (b *btNone) RouteToLocal() error     { return &BtUnsupported{} }
