package plugin

import (
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/grafana/sobek"
)

// jsHost 宿主能力注入：JS 插件唯一的 IO 出口。
//
// 安全模型（学 new-api 三层）：
//   - 能力沙箱：并发上限 + 单次调用超时（超时见 jsplugin.go）
//   - 网络限制：allowedHosts 白名单（meta 声明，未声明=全拒）
//   - 数据只走 JSON：不传 Go 对象/接口/slice/map
type jsHost struct {
	vm           *sobek.Runtime
	allowedHosts []string
	client       *http.Client
	sem          chan struct{}

	// 插件设置持久化（系统统一管理）：
	// pluginType = meta.capabilities[0]（插件类型）；pluginKey = meta.key。
	// config 读写走 SettingStore（注入），未注入=内存态。
	pluginType string
	pluginKey  string
}

// withPlugin 绑定插件身份（设置持久化命名空间用；NewJSPlugin 时调）。
func (h *jsHost) withPlugin(pluginType, key string) {
	h.pluginType = pluginType
	h.pluginKey = key
}

// maxConcurrentJSHTTP JS 插件并发 HTTP 上限（能力沙箱层）。
const maxConcurrentJSHTTP = 8

// newJSHost 创建宿主注入对象。
func newJSHost(vm *sobek.Runtime) *jsHost {
	return &jsHost{
		vm:     vm,
		client: &http.Client{Timeout: 15 * time.Second},
		sem:    make(chan struct{}, maxConcurrentJSHTTP),
	}
}

// hostHTTPRequest host.http 请求契约（JS 传 JSON，camelCase）。
type hostHTTPRequest struct {
	Method    string            `json:"method"`
	URL       string            `json:"url"`
	Headers   map[string]string `json:"headers"`
	Body      string            `json:"body"`
	TimeoutMs int               `json:"timeoutMs"`
}

// inject 注入 host 全局对象（host.http/log/config/setting/now）。
func (h *jsHost) inject(vm *sobek.Runtime) error {
	hostObj := vm.NewObject()
	if err := hostObj.Set("http", h.httpFunc); err != nil {
		return fmt.Errorf("host.http: %w", err)
	}
	if err := hostObj.Set("log", h.logFunc); err != nil {
		return fmt.Errorf("host.log: %w", err)
	}
	if err := hostObj.Set("now", func() int64 { return time.Now().UnixMilli() }); err != nil {
		return fmt.Errorf("host.now: %w", err)
	}
	// config：插件设置（系统统一管理，持久化走 SettingStore）。
	// 读=GetSetting 默认值，写=SetSetting 落库。注入后插件可 host.config.get/set。
	if err := hostObj.Set("config", h.configObject(vm)); err != nil {
		return fmt.Errorf("host.config: %w", err)
	}
	// setting：底层设置 API（config 的封装，key 不带命名空间前缀）。
	if err := hostObj.Set("setting", h.settingObject(vm)); err != nil {
		return fmt.Errorf("host.setting: %w", err)
	}
	// b64encode：UTF-8→base64（QQ/酷我歌词名 base64 编码；sobek 无内置 btoa）。
	if err := hostObj.Set("b64encode", func(call sobek.FunctionCall) sobek.Value {
		return vm.ToValue(base64.StdEncoding.EncodeToString([]byte(call.Argument(0).String())))
	}); err != nil {
		return fmt.Errorf("host.b64encode: %w", err)
	}
	// b64decode：base64→UTF-8（酷狗歌词 Content base64；sobek 无内置 atob）。
	if err := hostObj.Set("b64decode", func(call sobek.FunctionCall) sobek.Value {
		b, err := base64.StdEncoding.DecodeString(call.Argument(0).String())
		if err != nil {
			return vm.ToValue("")
		}
		return vm.ToValue(string(b))
	}); err != nil {
		return fmt.Errorf("host.b64decode: %w", err)
	}
	// md5：hex MD5（千千/百度音乐 sign 算法；sobek 无 crypto）。
	if err := hostObj.Set("md5", func(call sobek.FunctionCall) sobek.Value {
		sum := md5.Sum([]byte(call.Argument(0).String()))
		return vm.ToValue(hex.EncodeToString(sum[:]))
	}); err != nil {
		return fmt.Errorf("host.md5: %w", err)
	}
	return vm.Set("host", hostObj)
}

// configObject host.config：读=默认值（声明未存），写=落库。
// 提供 get(key[,def]) / set(key,val) / all() 三个方法。
func (h *jsHost) configObject(vm *sobek.Runtime) *sobek.Object {
	obj := vm.NewObject()
	_ = obj.Set("get", func(call sobek.FunctionCall) sobek.Value {
		if len(call.Arguments) == 0 {
			return sobek.Undefined()
		}
		key := call.Argument(0).String()
		def := ""
		if len(call.Arguments) > 1 {
			def = call.Argument(1).String()
		}
		v := GetSetting(h.pluginType, h.pluginKey, key, def)
		return vm.ToValue(v)
	})
	_ = obj.Set("set", func(call sobek.FunctionCall) sobek.Value {
		if len(call.Arguments) < 2 {
			return vm.ToValue(false)
		}
		key := call.Argument(0).String()
		val := call.Argument(1).String()
		if err := SetSetting(h.pluginType, h.pluginKey, key, val); err != nil {
			return vm.ToValue(false)
		}
		return vm.ToValue(true)
	})
	// all：读全部已声明设置（含默认值），返回对象。
	_ = obj.Set("all", func(call sobek.FunctionCall) sobek.Value {
		out := map[string]string{}
		for _, it := range SettingsOf(h.pluginType, h.pluginKey) {
			out[it.Key] = GetSetting(h.pluginType, h.pluginKey, it.Key, it.Default)
		}
		return vm.ToValue(out)
	})
	return obj
}

// settingObject host.setting：底层设置 API（同 config，key 直接传）。
func (h *jsHost) settingObject(vm *sobek.Runtime) *sobek.Object {
	obj := vm.NewObject()
	_ = obj.Set("get", func(call sobek.FunctionCall) sobek.Value {
		if len(call.Arguments) == 0 {
			return sobek.Undefined()
		}
		key := call.Argument(0).String()
		def := ""
		if len(call.Arguments) > 1 {
			def = call.Argument(1).String()
		}
		return vm.ToValue(GetSetting(h.pluginType, h.pluginKey, key, def))
	})
	_ = obj.Set("set", func(call sobek.FunctionCall) sobek.Value {
		if len(call.Arguments) < 2 {
			return vm.ToValue(false)
		}
		if err := SetSetting(h.pluginType, h.pluginKey, call.Argument(0).String(), call.Argument(1).String()); err != nil {
			return vm.ToValue(false)
		}
		return vm.ToValue(true)
	})
	return obj
}

// configGet 读插件设置（供测试/内部用；等价 host.config.get）。
func (h *jsHost) configGet(key, def string) string {
	return GetSetting(h.pluginType, h.pluginKey, key, def)
}

// configSet 写插件设置（供测试/内部用；等价 host.config.set）。
func (h *jsHost) configSet(key, val string) error {
	return SetSetting(h.pluginType, h.pluginKey, key, val)
}

// httpFunc 实现 host.http(req) → resp（错误时 resp.error 非空，插件可判断）。
func (h *jsHost) httpFunc(call sobek.FunctionCall) sobek.Value {
	// 解析请求（JS object → map → JSON struct）
	var req hostHTTPRequest
	if err := h.decode(call.Argument(0), &req); err != nil {
		return h.errResp(fmt.Sprintf("http 请求解析: %v", err))
	}
	if req.URL == "" {
		return h.errResp("http: url 为空")
	}
	// 并发限（能力沙箱）
	select {
	case h.sem <- struct{}{}:
		defer func() { <-h.sem }()
	default:
		return h.errResp("http: 并发上限")
	}
	// 白名单校验（网络限制）
	u, err := url.Parse(req.URL)
	if err != nil || u.Hostname() == "" {
		return h.errResp("http: url 非法")
	}
	if !h.hostAllowed(u.Hostname()) {
		return h.errResp("http: host " + u.Hostname() + " 不在白名单")
	}
	// 发起请求
	method := strings.ToUpper(req.Method)
	if method == "" {
		method = http.MethodGet
	}
	var body io.Reader
	if req.Body != "" {
		body = strings.NewReader(req.Body)
	}
	reqObj, err := http.NewRequest(method, req.URL, body)
	if err != nil {
		return h.errResp("http: " + err.Error())
	}
	for k, v := range req.Headers {
		reqObj.Header.Set(k, v)
	}
	resp, err := h.client.Do(reqObj)
	if err != nil {
		return h.errResp("http: " + err.Error())
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	headers := map[string]string{}
	for k := range resp.Header {
		headers[k] = resp.Header.Get(k)
	}
	return h.vm.ToValue(map[string]interface{}{
		"status":  resp.StatusCode,
		"body":    string(respBody),
		"headers": headers,
	})
}

// hostAllowed 校验 host 是否在白名单（全字匹配；支持 *.example.com 前缀通配）。
func (h *jsHost) hostAllowed(host string) bool {
	if len(h.allowedHosts) == 0 {
		return false // 未声明 allowedHosts = 全拒
	}
	for _, a := range h.allowedHosts {
		a = strings.TrimSpace(a)
		if a == "" {
			continue
		}
		if strings.HasPrefix(a, "*.") {
			if strings.HasSuffix(host, strings.TrimPrefix(a, "*")) {
				return true
			}
			continue
		}
		if host == a {
			return true
		}
	}
	return false
}

// logFunc host.log(msg)。
func (h *jsHost) logFunc(call sobek.FunctionCall) sobek.Value {
	if len(call.Arguments) > 0 {
		jsLog.Printf("%v", call.Argument(0))
	}
	return sobek.Undefined()
}

// errResp 返回 {status:0, body:"", error:msg}（插件可判断并自行处理）。
func (h *jsHost) errResp(msg string) sobek.Value {
	return h.vm.ToValue(map[string]interface{}{"status": 0, "body": "", "error": msg})
}

// decode JS value → Go struct（JSON roundtrip：object→map→json→struct）。
// 用 json tag 控制字段映射，绕开 sobek ExportTo 的 struct 匹配不确定性。
func (h *jsHost) decode(v sobek.Value, target interface{}) error {
	if v == nil || sobek.IsUndefined(v) || sobek.IsNull(v) {
		return fmt.Errorf("参数为空")
	}
	var m map[string]interface{}
	if err := h.vm.ExportTo(v, &m); err != nil {
		return err
	}
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, target)
}
