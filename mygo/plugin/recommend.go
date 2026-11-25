package plugin

// 推荐插件接口：推荐页是壳，内容 + UI 全由插件自己实现。
// 插件实现 Render（自绘 UI），宿主推荐页只遍历调用；宿主通过 RecommendHost
// 注入能力（依赖反转，插件不认具体 app 类型，可独立实现/复用）。

import (
	"github.com/egoist/mygo/ui"
)

// Recommend 推荐插件：内容 + UI 全由插件实现。
// 宿主按 preview 调 Render：preview=true（来源页预览，插件画 1-2 行精简吸引点击）
// / false（推荐详情页，插件画全部，分插件展示）。插件自控行数。
type Recommend interface {
	Name() string                                    // 推荐插件名（展示）
	Render(c *ui.Context, h RecommendHost, preview bool) // 插件自绘推荐 UI（preview 决定行数）
}

// RecommendHost 推荐页宿主能力（由主程序 app 实现，注入推荐插件）。
// 插件只通过这个接口访问宿主（依赖反转），不认具体类型（第三方插件可复用）。
type RecommendHost interface {
	// Play 播放歌曲（songID=音源 id，title=标题；插件调）
	Play(songID, title, cover string)
	// PlayRef 按真实来源播放（榜单/跨源插件用；source 是音源 key，id 是该音源内 id）。
	// 旧 Play 把 songID 塞进 BVID 只适合 B站；榜单是跨源的，必须带 source。
	PlayRef(source, id, title, artist, cover string)
	// Invalid 触发重绘（插件异步取数据后调）
	Invalidate()
	// Resp 分辨率适配（插件布局用）
	Resp(a, b float32) float32
}

// recommendRegistry 推荐插件注册表（按注册顺序）。
var recommendRegistry []Recommend

// RegisterRecommend 注册推荐插件（init 里调；重复注册去重）。
func RegisterRecommend(r Recommend) {
	for _, exist := range recommendRegistry {
		if exist.Name() == r.Name() {
			return // 同名不重复注册
		}
	}
	recommendRegistry = append(recommendRegistry, r)
}

// Recommends 已注册推荐插件（按注册顺序）。
func Recommends() []Recommend { return recommendRegistry }
