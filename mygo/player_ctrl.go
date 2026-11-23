//go:build native

package main

import (
	"fmt"
	"homecast/bilibili"
	"time"

	"github.com/egoist/mygo/ui"
)

func (a *app) setTab(t string) {
	a.activeTab = t
	a.win.Invalidate()
}

// accent 高亮色：条件真→粉紫，否则灰。
func accent(b bool) ui.Color {
	if b {
		return clrPink
	}
	return clrMuted
}

// next 下一首。对齐主线 d-next / cmd('next')：后端 Next 纯环绕顺序
// （(QIdx+1)%len），random 只是 UI 图标（主线后端 Next 不随机，前端也直接 cmd('next')），
// 不自创随机跳转。
func (a *app) next() {
	a.pb.Next()
}

// prev 上一首。对齐主线 d-prev / cmd('prev')：后端 Prev 纯环绕顺序
// （(QIdx-1+len)%len），无 ">3s 回到开头" 自创逻辑。
func (a *app) prev() {
	a.pb.Prev()
}

// seek 按比例跳转进度（走 SSOT）。
// 音箱模式：playback.Seek → SpeakerOutput.Seek → speaker.SeekTo → 投带 start=N 的新 URL
// （ffmpeg 切剩余段，代价 1~2s）。本地模式：MyGoOutput.Seek 直接在解码器定位。
func (a *app) seek(frac float64) {
	if frac < 0 {
		frac = 0
	}
	if frac > 1 {
		frac = 1
	}
	dur := a.pb.Snapshot().Duration
	if dur <= 0 {
		return // 时长未知不 seek（否则 seek 到 0 变成从头播）
	}
	a.pb.Seek(frac * dur)
}

// setVolume 设置音量 0..1 并持久化偏好（音量条拖动用）。
// 音箱模式（source=speaker）走主线 playback.SetVolume
// （out=SpeakerOutput 真调音箱 Control，复用主线不造轮子）；web 模式走 MyGo 本地 audio。
func (a *app) setVolume(v float64) {
	a.applyVolume(v)
	now := time.Now()
	if now.Sub(a.lastVolSave) > 500*time.Millisecond {
		a.savePrefFloat("mygo_volume", v)
		a.lastVolSave = now
	}
}

// applyVolume 只改当前音量+输出端，不写偏好。静音/取消静音用——
// 静音是临时状态，持久化会导致下次启动音量条归 0（踩过：mygo_volume=0）。
func (a *app) applyVolume(v float64) {
	if v < 0 {
		v = 0
	}
	if v > 1 {
		v = 1
	}
	a.volume = v
	if a.spkMode {
		// 音箱模式：复用主线（SpeakerOutput.SetVolume → speaker.Control volume）
		a.pb.SetVolume(int(v * 100))
	} else if a.mygoOut != nil {
		a.mygoOut.SetVolume(int(v * 100))
	}
	a.win.Invalidate()
}

// toggleLoop 循环模式：列表循环→单曲→随机（同步 SSOT）。
func (a *app) toggleLoop() {
	a.loopMode = (a.loopMode + 1) % 4
	a.pb.SetMode(intToMode(a.loopMode))
	a.savePref("mygo_loopmode", a.loopMode)
}

// loopLabel 当前循环模式名。
func (a *app) loopLabel() string {
	switch a.loopMode {
	case 1:
		return "列表循环"
	case 2:
		return "单曲循环"
	case 3:
		return "随机播放"
	default:
		return "顺序播放"
	}
}

// isFav 当前曲目是否已收藏（SSOT：fav 域包）。
func (a *app) isFav() bool {
	return a.current != nil && a.favHasSafe(a.current.bvid)
}

// favHasSafe 收藏查询（nil 安全：svc 未初始化返回 false）。
func (a *app) favHasSafe(bvid string) bool {
	if a.svc == nil {
		return false
	}
	return a.svc.favHas(bvid)
}

// toggleFav 收藏/取消当前曲目（SSOT：fav 域包 Add/Remove）。
func (a *app) toggleFav() {
	if a.current == nil || a.svc == nil {
		return
	}
	if a.svc.favHas(a.current.bvid) {
		a.svc.fav.Remove(a.current.bvid)
	} else {
		a.svc.fav.Add(favNewEntry(a.current.bvid, a.current.title, a.current.author, a.current.pic, durationSec(0, "")))
	}
	a.refreshFavs()
	a.win.Invalidate()
}

// isFavItem 指定曲目是否已收藏（SSOT）。
func (a *app) isFavItem(item bilibili.SearchItem) bool {
	return a.favHasSafe(item.BVID)
}

// toggleFavItem 收藏/取消指定曲目（SSOT）。
func (a *app) toggleFavItem(item bilibili.SearchItem) {
	if a.svc.favHas(item.BVID) {
		a.svc.fav.Remove(item.BVID)
	} else {
		a.svc.fav.Add(favNewEntry(item.BVID, bilibili.CleanHTML(item.Title), item.Author, item.Pic, 0))
	}
	a.refreshFavs()
	a.win.Invalidate()
}

// refreshFavs 从 fav 域包刷新 favorites 快照（UI 渲染用）。
func (a *app) refreshFavs() {
	list := a.svc.fav.List()
	a.favorites = a.favorites[:0]
	a.favSet = make(map[string]bool, len(list))
	for _, f := range list {
		a.favSet[f.BVID] = true
		a.favorites = append(a.favorites, bilibili.SearchItem{
			BVID: f.BVID, Title: f.Title, Author: f.Artist, Pic: f.Cover,
			Duration: f.Duration,
		})
	}
}

// volumeBar 音量滑块：拖动 SetVolume，点击轨道定位。
func (a *app) volumeBar(c *ui.Context, width float32) ui.Element {
	// 对齐主线 d-vol：<input type=range> 原生可拖。改用 mygo 原生 SliderBase
	// （flagDraggable，绑定 &a.volume 双向）：之前自定义 Box+PointerPosition 拖不动。
	// Draw 自定义轨道/填充/thumb（高 6 粉渐变 + 12px 圆 thumb，thumb 始终显示）。
	const h = float32(6)
	pad := float32(10) // 半 thumb 宽 padding，让 thumb 不出界
	s := ui.SliderBase(c, &a.volume, 0, 1).Width(width).Height(h + 2*pad).PaddingX(pad)
	s.Draw(func(p *ui.Painter, r ui.Rect) {
		track := ui.Rect{X: r.X, Y: r.Y + (r.H-h)/2, W: r.W, H: h}
		p.FillRadii(track, [4]float32{3, 3, 3, 3}, clrTrackBar)
		v := a.volume
		if v < 0 {
			v = 0
		}
		if v > 1 {
			v = 1
		}
		cy := track.Y + h/2
		if v > 0 {
			fr := ui.Rect{X: track.X, Y: track.Y, W: track.W * float32(v), H: h}
			p.FillGradient(fr, grad(), 3)
		}
		// 圆形 thumb 始终显示（volume 0 也在最左，对齐主线 range thumb 常驻）
		cx := track.X + track.W*float32(v)
		th := ui.Rect{X: cx - 6, Y: cy - 6, W: 12, H: 12}
		p.FillRadii(th, [4]float32{6, 6, 6, 6}, ui.RGB(0xff, 0xff, 0xff))
		p.FillRadii(ui.Rect{X: th.X + 1, Y: th.Y + 1, W: th.W - 2, H: th.H - 2}, [4]float32{5, 5, 5, 5}, clrPink)
		// hover 百分比气泡
		if s.Hovered() {
			pct := int(a.volume * 100)
			if a.muted {
				pct = 0
			}
			// 纯数字百分比，不带外框（用户：外框气泡有点崩，纯数字即可）
			label := fmt.Sprintf("%d%%", pct)
			p.Text(r.X+r.W/2-6, r.Y-14, label, 11, clrText)
		}
	})
	if s.Changed() {
		a.setVolume(a.volume) // 拖动改 a.volume 后应用音频音量
	}
	return s
}

// nowPlaying 正在播放歌词页：对齐主线 lyric-panel 竖屏歌词面板结构
// （三栏：候选栏 + 中栏歌词/进度 + 工具列；背景封面模糊/纯色；<768 候选/工具收抽屉，歌词全宽）。
