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
	Order        int      `json:"order"`
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

// builtinKeys 内置插件名（JS 不得同名覆盖，防注册表冲突）。
var builtinKeys = map[string]bool{
	"bilibili": true, "netease": true, "local": true,
	"cast": true, "bluetooth": true, "speaker": true,
}

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
	jp.host.allowedHosts = jp.meta.AllowedHosts
	jp.bind()
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
	if builtinKeys[jp.meta.Key] {
		return fmt.Errorf("key %q 与内置插件冲突", jp.meta.Key)
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
			RegisterInfo(Info{Name: jp.meta.Key, Type: "source", Icon: jp.meta.Key, Version: jp.meta.Version, Desc: jp.meta.Name, Order: jp.meta.Order})
		case "lyric":
			if jp.lyricFn == nil {
				return fmt.Errorf("capabilities 含 lyric 但缺 lyric 函数")
			}
			RegisterLyric(&jsLyric{jp: jp})
			RegisterInfo(Info{Name: jp.meta.Key, Type: "lyric", Icon: jp.meta.Key, Version: jp.meta.Version, Desc: jp.meta.Name, Order: jp.meta.Order})
		default:
			return fmt.Errorf("未知 capability %q", cap)
		}
	}
	return nil
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
