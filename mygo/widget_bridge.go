//go:build native

// widget_bridge：把主线 go/service/widget 的 SSE 挂件状态机接到 mygo 的 stream_server。
// 安卓悬浮歌词 Service（Kotlin native module）通过 127.0.0.1:28977 的
// /api/v1/widget/state + /api/v1/widget/events 拿歌词行快照 + 订阅推送，
// 与桌面挂件/主线 web 同源（逻辑住后端 PickLine，显示层纯渲染）。
package main

import (
	"homecast/api"
	"homecast/service/lyric"
	"homecast/service/widget"
	"log"
	"math"
)

// mygoLyricSource 适配器：复用 go/server/widget_lyricsource.go 同款逻辑，
// 把 "bvid → 歌词行 + offset" 封装成 widget.LyricSource（mygo 侧重建，因 server 包是私有）。
type mygoLyricSource struct {
	handler *api.LyricHandler
	state   *widget.WidgetState
}

var _ widget.LyricSource = (*mygoLyricSource)(nil)

func (l *mygoLyricSource) Lines(bvid string) ([][3]any, float64, bool) {
	if bvid == "" {
		return nil, 0, false
	}
	// ① 用户已选歌词源优先（与歌词页 /music/lyric 同一份 + offset）
	if sel := l.handler.Lyric.SelectedSID(bvid); sel > 0 {
		if res := lyric.GetLyricLines(nil, sel, 0); res != nil {
			off := float64(0)
			if rec, ok := l.handler.Lyric.Get(bvid); ok {
				off = rec.Offset
			}
			return toWidgetLrcLines(res.Lines), off, true
		}
	}
	// ② 无已选源：跟歌词页同款（LyricKeywords + GetLyricLines 自动选）
	title, artist := "", ""
	if l.state != nil {
		_, title, artist, _, _, _ = l.state.Get()
	}
	var kws []string
	if l.handler.Music != nil {
		kws = l.handler.Music.LyricKeywords(bvid, title+" "+artist)
	}
	if len(kws) == 0 {
		if k := lyric.CleanSearchKeyword(title + " " + artist); k != "" {
			kws = []string{k}
		}
	}
	if len(kws) > 0 {
		dur := 0
		if l.handler.Music != nil {
			dur = l.handler.Music.LyricDuration(bvid)
		}
		if res := lyric.GetLyricLines(kws, 0, dur); res != nil {
			return toWidgetLrcLines(res.Lines), 0, true
		}
	}
	return nil, 0, false
}

func toWidgetLrcLines(in []lyric.LyricLine) [][3]any {
	out := make([][3]any, 0, len(in))
	for _, x := range in {
		out = append(out, x)
	}
	return out
}

// initWidgetState 建 widget 状态机 + 注入 lyric source（mygo 侧等价主线 server.New）。
func (a *app) initWidgetState(lh *api.LyricHandler) *widget.WidgetState {
	st := widget.NewWidgetState()
	st.SetLineSrc(&mygoLyricSource{handler: lh, state: st})
	return st
}

// pushWidgetState 播放状态变化 → 喂 widget state（触发 PickLine + SSE 广播 lyric-state）。
// 由 progressReporter 每 250ms 调；bvid/title/artist 取 a.current。
func (a *app) pushWidgetState() {
	if a.widgetState == nil {
		return
	}
	bvid, title, artist := "", "", ""
	if a.current != nil {
		bvid = a.current.bvid
		title = a.current.title
		artist = a.current.author
	}
	dur := a.posSec
	if a.durText != "" {
		// duration 用比例反推（a.progress = pos/dur）；更准的是存一份 dur，这里够用
		if a.progress > 0 {
			dur = a.posSec / a.progress
		}
	}
	// 去重：暂停静止时 progressReporter 上报的 posSec 不变，但每 250ms 仍在调 Set
	// （内部广播 SSE + PickLine 歌词定位）。状态没变就跳过，省掉这些空转。
	// 时间用 0.01s 容差避浮点抖动（切歌/拖动/播放态变化都必然越过容差）。
	ob, ot, oa, oct, odur, oplay := a.widgetState.Get()
	if ob == bvid && ot == title && oa == artist && oplay == a.playing &&
		math.Abs(oct-a.posSec) < 0.01 && math.Abs(odur-dur) < 0.01 {
		return
	}
	a.widgetState.Set(bvid, title, artist, a.posSec, dur, a.playing)
}

// listenWidgetCommands 订阅挂件控制事件（toggle/prev/next）：浮窗按钮 → mygo 播放器。
// 主线 widget 是"广播给前端"，mygo 没前端 JS，所以这里直接消费 SSE 内部事件。
func (a *app) listenWidgetCommands() {
	if a.widgetState == nil {
		return
	}
	ch := a.widgetState.Subscribe()
	go func() {
		for msg := range ch {
			switch msg.Name {
			case "toggle":
				a.pb.Toggle()
			case "prev":
				a.pb.Prev()
			case "next":
				a.pb.Next()
			}
		}
	}()
	log.Printf("[widget] 挂件命令监听已启动")
}
