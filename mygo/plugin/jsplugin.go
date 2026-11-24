package plugin

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/grafana/sobek"
)

// jsLog JS 插件日志（宿主统一输出）。
var jsLog = log.New(os.Stderr, "[jsplugin] ", log.LstdFlags)

// jsMeta JS 插件 meta（声明式契约，对齐交接文档 §4）。
type jsMeta struct {
	APIVersion   int      `json:"apiVersion"`
	Key          string   `json:"key"`
	Name         string   `json:"name"`
	Version      string   `json:"version"`
	Capabilities []string `json:"capabilities"`
	AllowedHosts []string `json:"allowedHosts"`
	RequiresCookie bool `json:"requiresCookie"` // 需 cookie 才激活（无 cookie 不注册）
	Order        int      `json:"order"`
	Settings     []jsSetting `json:"settings"`
}

// jsSetting JS 插件设置项声明（对齐 plugin.Setting；meta.settings 数组）。
// 系统统一管理：声明归插件，存储归 SettingStore。
type jsSetting struct {
	Key     string   `json:"key"`
	Name    string   `json:"name"`
	Type    string   `json:"type"`
	Options []string `json:"options"`
	Default string   `json:"default"`
	Desc    string   `json:"desc"`
}

// JSPlugin 一个已加载的 JS 插件（包装成接口）。
type JSPlugin struct {
	meta jsMeta
	vm   *sobek.Runtime
	host *jsHost

	searchFn sobek.Callable
	getURLFn sobek.Callable
	lyricFn  sobek.Callable
}

// jsTimeout JS 单次调用超时（能力沙箱层，掐断死循环）。
const jsTimeout = 5 * time.Second

// 系统级（Go 内置）能力 key 的防护清单见 builtin.go：由 RegisterSystem* 注册行为派生，
// 此处不再硬编码副本（避免新增 Go 内置能力时忘了同步）。

// LoadFromDir 扫描 dir/*/plugin.js 逐个加载注册。
// 单个插件失败不拖垮其他；返回错误列表 + 目录读取错误。
func LoadFromDir(dir string) ([]error, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // 无插件目录 = 正常
		}
		return nil, err
	}
	var errs []error
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		// 已有 Go 内置实现的 key：按目录名跳过（NewJSPlugin 会拒绝内置 key）
		if IsBuiltinKey("source", e.Name()) || IsBuiltinKey("lyric", e.Name()) || IsBuiltinKey("target", e.Name()) {
			continue
		}
		p := filepath.Join(dir, e.Name(), "plugin.js")
		code, err := os.ReadFile(p)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			errs = append(errs, fmt.Errorf("%s: %w", p, err))
			continue
		}
		jp, err := NewJSPlugin(p, string(code))
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", p, err))
			continue
		}
		// requiresCookie 源：无 cookie 不激活 source（搜索不出），但 Info/Settings 仍注册
		// ——否则插件页看不到它，用户没法进详情页登录（鸡生蛋）。
		inactive := false
		if jp.meta.RequiresCookie {
			ptype := "source"
			if len(jp.meta.Capabilities) > 0 {
				ptype = jp.meta.Capabilities[0]
			}
			if GetSetting(ptype, jp.meta.Key, "cookie", "") == "" {
				inactive = true
			}
		}
		// 元数据/设置始终注册（详情页可见）
		jp.registerMeta()
		if inactive {
			fmt.Printf("[plugin] %s 需登录 Cookie 才能激活（无 cookie，仅显示配置）\n", jp.meta.Key)
			continue
		}
		if err := jp.register(); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", p, err))
		}
	}
	return errs, nil
}

// NewJSPlugin 解析并校验一个 JS 插件（不注册，register 才进注册表）。
func NewJSPlugin(path, code string) (*JSPlugin, error) {
	vm := sobek.New()
	jp := &JSPlugin{vm: vm}
	jp.host = newJSHost(vm)
	if err := jp.host.inject(vm); err != nil {
		return nil, err
	}
	if _, err := vm.RunString(code); err != nil {
		return nil, fmt.Errorf("js eval: %w", err)
	}
	metaVal := vm.Get("meta")
	if metaVal == nil || sobek.IsUndefined(metaVal) || sobek.IsNull(metaVal) {
		return nil, fmt.Errorf("未声明 meta")
	}
	if err := jp.host.decode(metaVal, &jp.meta); err != nil {
		return nil, fmt.Errorf("meta: %w", err)
	}
	if err := jp.validate(); err != nil {
		return nil, err
	}
	// 绑定插件身份（设置持久化命名空间：pluginType=capabilities[0]，pluginKey=key）
	ptype := ""
	if len(jp.meta.Capabilities) > 0 {
		ptype = jp.meta.Capabilities[0]
	}
	jp.host.withPlugin(ptype, jp.meta.Key)
	jp.host.allowedHosts = jp.meta.AllowedHosts
	jp.bind()
	// 注册设置声明（meta.settings → 系统统一存储；NewJSPlugin 即注册，不依赖 register 路径）
	if len(jp.meta.Settings) > 0 {
		jp.registerSettings()
	}
	return jp, nil
}

func (jp *JSPlugin) validate() error {
	if jp.meta.APIVersion != 1 {
		return fmt.Errorf("不支持的 apiVersion=%d", jp.meta.APIVersion)
	}
	if jp.meta.Key == "" {
		return fmt.Errorf("meta.key 为空")
	}
	if jp.meta.Name == "" {
		jp.meta.Name = jp.meta.Key
	}
	// 按 capability 类型校验系统级 key 占用（JS 不得覆盖 Go 内置能力）。
	// 防护清单由 RegisterSystem* 注册行为派生（builtin.go），不含人工副本。
	for _, cap := range jp.meta.Capabilities {
		if err := CheckPluginKey(cap, jp.meta.Key); err != nil {
			return err
		}
	}
	return nil
}

// bind 绑定导出函数（search/getURL/lyric，全局函数形式，非 ES module）。
func (jp *JSPlugin) bind() {
	if f, ok := sobek.AssertFunction(jp.vm.Get("search")); ok {
		jp.searchFn = f
	}
	if f, ok := sobek.AssertFunction(jp.vm.Get("getURL")); ok {
		jp.getURLFn = f
	}
	if f, ok := sobek.AssertFunction(jp.vm.Get("lyric")); ok {
		jp.lyricFn = f
	}
}

// register 按 capabilities 注册到插件注册表（source/lyric）。
func (jp *JSPlugin) register() error {
	if len(jp.meta.Capabilities) == 0 {
		return fmt.Errorf("capabilities 为空")
	}
	for _, cap := range jp.meta.Capabilities {
		switch cap {
		case "source":
			if jp.searchFn == nil || jp.getURLFn == nil {
				return fmt.Errorf("capabilities 含 source 但缺 search/getURL")
			}
			RegisterSource(&jsSource{jp: jp})
		case "lyric":
			if jp.lyricFn == nil {
				return fmt.Errorf("capabilities 含 lyric 但缺 lyric 函数")
			}
			RegisterLyric(&jsLyric{jp: jp})
		default:
			return fmt.Errorf("未知 capability %q", cap)
		}
	}
	return nil
}

// registerMeta 只注册元数据（插件页展示）——requiresCookie 无 cookie 时也要能进详情页登录。
func (jp *JSPlugin) registerMeta() {
	for _, cap := range jp.meta.Capabilities {
		if cap == "source" || cap == "lyric" {
			RegisterInfo(Info{Name: jp.meta.Key, Type: cap, Icon: jp.meta.Key, Version: jp.meta.Version, Desc: jp.meta.Name, Order: jp.meta.Order})
		}
	}
}

// registerSettings 把 meta.settings 声明注册进设置表（系统统一存储）。
func (jp *JSPlugin) registerSettings() {
	ptype := ""
	if len(jp.meta.Capabilities) > 0 {
		ptype = jp.meta.Capabilities[0]
	}
	items := make([]Setting, 0, len(jp.meta.Settings))
	for _, js := range jp.meta.Settings {
		items = append(items, Setting{
			Key:     js.Key,
			Name:    js.Name,
			Type:    SettingValue(js.Type),
			Options: js.Options,
			Default: js.Default,
			Desc:    js.Desc,
		})
	}
	RegisterSettings(ptype, jp.meta.Key, items)
}

// call 调用 JS 函数（带超时，能力沙箱层）。
// 用 goroutine + Interrupt 掐断死循环/卡死。
func (jp *JSPlugin) call(fn sobek.Callable, args ...sobek.Value) (sobek.Value, error) {
	done := make(chan struct{})
	var val sobek.Value
	var err error
	go func() {
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("js panic: %v", r)
			}
			close(done)
		}()
		val, err = fn(sobek.Undefined(), args...)
	}()
	select {
	case <-done:
	case <-time.After(jsTimeout):
		jp.vm.Interrupt("js plugin timeout")
		<-done
		return nil, fmt.Errorf("js 超时(%v)", jsTimeout)
	}
	return val, err
}

// makeCtx 构造 JS ctx 对象（{数据字段..., config, host}）。
func (jp *JSPlugin) makeCtx(fields map[string]interface{}) (*sobek.Object, error) {
	obj := jp.vm.NewObject()
	for k, v := range fields {
		if err := obj.Set(k, v); err != nil {
			return nil, err
		}
	}
	// config：插件设置（默认空；可扩展持久化）
	if err := obj.Set("config", map[string]interface{}{}); err != nil {
		return nil, err
	}
	// host 也注入 ctx（插件可用 ctx.host 或全局 host）
	if err := obj.Set("host", jp.vm.Get("host")); err != nil {
		return nil, err
	}
	return obj, nil
}
