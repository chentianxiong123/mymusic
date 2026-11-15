//go:build native

package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	_ "github.com/egoist/mygo/audio/aac" // mp4a (AAC) 解码器
	_ "github.com/egoist/mygo/audio/mp4" // m4a / fMP4 解码
	"github.com/egoist/mygo/ui"

	"homecast/service/cast"
	"homecast/service/lyric"
	"homecast/service/playback"

	"homecast/bilibili"
	"homecast/xiaoai"
)

// track 当前曲目。
type track struct {
	bvid, cid, title, author, url, pic, duration string
}

// app 是窗口显示的状态。view 在主线程从它构建界面；
// goroutine 只通过 win.Update / RunOnMain 改它。
type app struct {
	win      *mygo.Window // 主窗口
	lyricWin *mygo.Window // 歌词挂件窗口（可空）
	bili     *bilibili.Client

	// 主线 SSOT 接入：业务状态/队列/模式/持久化全在 playback 域包（MyGo 只渲染+发命令）
	pb           *playback.PlaybackState
	mygoOut      *MyGoOutput
	db           *sql.DB
	svc          *services              // 主线域包句柄（fav/lyric/cast/speaker）
	castOn       string                 // 正在投屏的设备名（空=未投屏，对齐主线 d-stop 显示）
	castDevices  []cast.DeviceDTO       // DLNA 设备列表（cast 域包）
	speakers     []xiaoai.SpeakerDevice // 小米音箱设备（speaker 域包）
	recent       []bilibili.SearchItem  // 最近播放（对齐主线 hc:recent，最多 30）
	lastVolSave  time.Time              // 音量落库节流时间戳
	qrStatus     string                 // 音箱扫码登录状态文案
	lastLyricIdx int                    // 上次高亮歌词行（KTV 滚动跟随去重）
	lyrDrawer    int                    // 窄档歌词抽屉：0 关 1 候选 2 工具（对齐主线 ly-drawer-*）
	searched     bool                   // 是否已执行过搜索（区分空结果态）
	eqPhase      int                    // dock 封面 EQ 频谱相位（播放中推进，对齐主线 d-eq 动画）
	qrImage      string                 // 二维码 data URI（渲染用）
	ctxItem      *bilibili.SearchItem   // 右键菜单目标（非空=显示）
	ctxX, ctxY   float32                // 菜单弹出位置（窗口坐标）
	clipboard    string                 // 待复制链接（mygo 暂无线程安全 clipboard API，先字段+状态提示）

	// 搜索
	query   string
	results []bilibili.SearchItem
	loading atomic.Bool
	status  string

	// 播放（主线程访问；播放器由 mygoOut 持有，audio.Player 自身安全）
	current    *track
	playing    bool
	progress   float64 // 0..1
	posText    string
	durText    string
	lyricText  string
	lyricShown bool

	// 封面缓存（主线程；异步加载经 win.Update 写）
	covers       map[string]ui.ImageSource
	coverLoading map[string]bool
	coverOrder   []string // LRU 顺序：封面缓存超过上限时驱逐最旧的

	// 播放控制面板 / 队列 / 收藏
	showPlayer bool
	activeTab  string // search|queue|fav|cast|settings
	playlist   []bilibili.SearchItem
	playIndex  int
	posSec     float64 // 当前播放秒（歌词定位用）
	volume     float64
	loopMode   int // 0 顺序 1 列表循环 2 单曲 3 随机（对齐主线 4 态）
	favorites  []bilibili.SearchItem
	favSet     map[string]bool

	quality       int      // 0=64k 1=128k 2=192k（对齐主线 d-quality 三档循环）
	lyricSize     int      // 0-4 歌词字号档（对齐主线 14/16/18/22/26px）
	capSize       int      // 0-4 字幕字号档（对齐主线 16/20/24/28/32px）
	themeMode     int      // 0 自动 1 明亮 2 暗黑（对齐主线 THEMES 三态）
	lyricBg       int      // 0 封面模糊 1 纯色（对齐主线 set-lybg）
	lyricTime     bool     // 歌词行显示时间戳（对齐主线 set-lytime）
	cacheLimit    int      // 0 关闭 1 200MB 2 500MB 3 1GB 4 2GB（对齐主线缓存上限）
	searchHistory []string // 搜索历史（对齐主线 清空历史）
	// 歌词（复用主线 lyric 域包；对齐 lyric-panel）
	lyricLines    []lyric.LyricLine
	lyricCands    []lyric.LyricResult
	lyricSID      int
	lyricOffset   float64
	lyricLoading  bool
	expandedTitle string // 搜索结果标题展开态 bvid（对齐主线 hx-title）
	// 静音 / 字幕条（学主线结构）
	muted        bool
	lastVolume   float64
	captionShown bool

	// 响应式：导航抽屉（对齐主线 nav.js side-nav-panel：汉堡 toggle/点外关/选后收）
	navOpen bool

	// 分辨率适配：三档断点（对齐主线 Tailwind sm/md 语义）
	winW int
	winH int
}

// tier 布局档位（对齐主线 Tailwind 断点语义）：
//
//	narrow(<768)  = 主线 md: 以下 → 移动导航（汉堡+侧滑面板，主线 md:hidden 同款）
//	compact(768-1024) = 主线 sm/md → 桌面导航紧凑（列表/间距收紧）
//	normal(>=1024) = 主线 lg  → 桌面完整
//
// 主线 style.css 断点模式：默认=移动，sm:(≥640)=桌面，md:(≥768)=完整导航。
func (a *app) tier() int {
	if a.winW <= 0 {
		return 2 // 窗口未知 → 按正常渲染
	}
	if a.winW < 768 {
		return 0
	}
	if a.winW < 1024 {
		return 1
	}
	return 2
}

// compact 紧凑模式（tier<=1，兼容旧断点语义）：缩小字号/间距/封面。
func (a *app) compact() bool {
	return a.tier() <= 1
}

// narrow 窄模式（tier==0，对齐主线手机档 sm）：结构切换（导航图标化/隐藏次要控件）。
func (a *app) narrow() bool {
	return a.tier() == 0
}

// resp 响应式尺寸：正常值 / 紧凑值按当前窗口宽度选。
func (a *app) resp(normal, narrow float32) float32 {
	// 跟随系统文字缩放（对齐主线 GUI 150%：渲染 px 乘 uiScale）
	v := normal
	if a.compact() {
		v = narrow
	}
	return v * uiScale
}

func (a *app) view(c *ui.Context) {
	applyTheme(a.themeMode) // 主题三态（对齐主线 THEMES）
	a.handleShortcuts(c)    // 键盘快捷键（对齐主线 shortcuts.js）
	ui.Column(c).Fill().Background(clrBg).Children(func() {
		a.navBar(c)
		// 页面内容：Grow 占剩余（navBar 后 - playerBar），playerBar 固定底部。
		// 之前 switch 内容直接 Column.Fill() 撑满整窗（含 playerBar 区），
		// playerBar 被挤出窗口不可见（队列/设置/投屏无控制面板）。
		ui.Column(c).Grow(1).Fill().Children(func() {
			switch a.activeTab {
			case "queue":
				a.queuePage(c)
			case "fav":
				a.favPage(c)
			case "cast":
				a.castPage(c)
			case "settings":
				a.settingsPage(c)
			case "recent":
				a.recentPage(c)
			default:
				if len(a.results) > 0 {
					// 搜索条常驻（对齐主线 content_search.html：搜索条始终保留在顶部）
					a.searchBar(c)
					a.searchChips(c)
					a.resultsHeader(c)
					ui.Scroll(c).Grow(1).PaddingX(a.resp(24, 12)).Gap(8 * uiScale).Children(func() {
						ui.Column(c).Gap(8 * uiScale).Children(func() {
							for _, r := range a.results {
								a.listRow(c, r)
							}
						})
					})
				} else if a.searched {
					a.searchBar(c)
					a.searchChips(c)
					a.searchNotFound(c)
				} else {
					a.searchEmpty(c)
				}
			}
		})
		a.playerBar(c)
	})
	if a.showPlayer {
		a.nowPlaying(c)
	}
	// 右键上下文菜单（对齐主线 contextmenu.js）
	if a.ctxItem != nil {
		a.ctxMenu(c)
	}
}

// navBar 顶部导航——结构随档位切换（对齐主线：桌面完整导航 vs md:hidden 汉堡+侧滑面板）。
func (a *app) navBar(c *ui.Context) {
	if a.narrow() {
		a.navBarMobile(c)
	} else {
		a.navBarDesktop(c)
	}
	ui.Box(c).AlignSelf(ui.Stretch).Height(1 * uiScale).Draw(func(p *ui.Painter, r ui.Rect) {
		p.Fill(r, clrCard, 0)
	})
	if a.navOpen {
		a.navPanel(c)
	}
}

// navBarDesktop 桌面导航（主线 hidden md:flex 同款）：logo + 5 tab 整行。
func (a *app) navBarDesktop(c *ui.Context) {
	px := a.resp(20, 12)
	navGap := a.resp(6, 3)
	ui.Row(c).AlignSelf(ui.Stretch).Height(64 * uiScale).PaddingX(px).Gap(navGap).AlignItems(ui.Center).Children(func() {
		icon(c, icMusic, a.resp(24, 20), clrText)
		ui.Spacer(c).Width(a.resp(10, 4))
		if navTab(c, icList, "队列", a.activeTab == "queue", false).Clicked() {
			a.setTab("queue")
		}
		if navTab(c, icSearch, "搜索", a.activeTab == "search", false).Clicked() {
			a.setTab("search")
		}
		if navTab(c, icFav, "收藏", a.activeTab == "fav", false).Clicked() {
			a.setTab("fav")
		}
		if navTab(c, icCast, "投屏", a.activeTab == "cast", false).Clicked() {
			a.setTab("cast")
		}
		if navTab(c, icSet, "设置", a.activeTab == "settings", false).Clicked() {
			a.setTab("settings")
		}
	})
}

// navBarMobile 移动导航（主线 md:hidden 同款）：logo + 汉堡按钮（右），无多余开关。
func (a *app) navBarMobile(c *ui.Context) {
	ui.Row(c).AlignSelf(ui.Stretch).Height(56 * uiScale).PaddingX(12 * uiScale).Gap(10 * uiScale).AlignItems(ui.Center).Children(func() {
		icon(c, icMusic, 20, clrText)
		ui.Text(c, "myMusic").FontSize(15 * uiScale).Bold().TextColor(clrText)
		ui.Spacer(c).Grow(1)
		if iconButton2(c, icMenu, 34, clrMuted).Clicked() {
			a.navOpen = !a.navOpen
			a.win.Invalidate()
		}
	})
}

// navPanel 侧滑导航面板（主线 side-nav-panel 同款）：点背景关闭，选 tab 后收起。
func (a *app) navPanel(c *ui.Context) {
	ui.Overlay(c, func() {
		// 点外部关闭（透明遮罩，对齐 nav.js 的 click-outside）
		bg := ui.Box(c).Absolute().Left(0).Top(0).Right(0).Bottom(0)
		bg.Draw(func(p *ui.Painter, r ui.Rect) {
			p.Fill(r, ui.RGBA(0, 0, 0, 0.35), 0)
		})
		if bg.Clicked() {
			a.navOpen = false
		}
		// 面板：左上浮层（w-48 风格）
		panel := ui.Box(c).Absolute().Left(12).Top(64).Width(200 * uiScale).Radius(16 * uiScale).PaddingX(10 * uiScale).PaddingY(10 * uiScale).Background(clrCard)
		panel.Draw(func(p *ui.Painter, r ui.Rect) {
			p.Shadow(r, 16, 0, 12, 40, 0, ui.RGBA(0, 0, 0, 0.45))
		})
		panel.Children(func() {
			ui.Column(c).Gap(4 * uiScale).Children(func() {
				items := []struct {
					tab   string
					label string
					icon  *ui.SVG
				}{
					{"queue", "队列", icList},
					{"search", "搜索", icSearch},
					{"fav", "收藏夹", icFav},
					{"cast", "投屏", icCast},
					{"settings", "设置", icSet},
				}
				for _, it := range items {
					it := it
					a.navPanelRow(c, it.icon, it.label, a.activeTab == it.tab, func() {
						a.setTab(it.tab)
						a.navOpen = false
					})
				}
			})
		})
	})
}

// navPanelRow 侧滑面板里的一个导航项。
func (a *app) navPanelRow(c *ui.Context, s *ui.SVG, label string, active bool, onPick func()) {
	box := ui.Box(c).Fill().Height(42 * uiScale).Radius(10 * uiScale).PaddingX(12 * uiScale)
	if active {
		box.Background(clrField)
	} else if box.Hovered() {
		box.Background(clrField)
	}
	box.Children(func() {
		ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(12 * uiScale).Children(func() {
			col := clrMuted
			if active {
				col = clrPink
			}
			icon(c, s, 18, col)
			ui.Text(c, label).FontSize(14 * uiScale).TextColor(col)
		})
	})
	if box.Clicked() {
		onPick()
	}
}

// navTab 导航项：当前高亮为紫渐变胶囊；hover 淡高亮。iconOnly 时只显示图标。
func navTab(c *ui.Context, s *ui.SVG, label string, active bool, iconOnly bool) ui.Element {
	box := ui.Box(c).Height(40 * uiScale).Radius(20 * uiScale).PaddingX(16 * uiScale).AlignItems(ui.Center).Justify(ui.Center)
	if iconOnly {
		box = ui.Box(c).Size(40, 40).Radius(20 * uiScale).AlignItems(ui.Center).Justify(ui.Center)
	}
	if active {
		box.Draw(func(p *ui.Painter, r ui.Rect) {
			p.FillGradient(r, grad(), r.H/2)
		})
	} else {
		box.Draw(func(p *ui.Painter, r ui.Rect) {
			if box.Hovered() {
				p.Fill(r, ui.RGBA(0xff, 0xff, 0xff, 0.06), r.H/2)
			}
		})
	}
	box.Children(func() {
		col := clrMuted
		if active {
			col = ui.RGB(0xff, 0xff, 0xff)
		} else if box.Hovered() {
			col = clrText
		}
		ui.Row(c).Gap(8 * uiScale).AlignItems(ui.Center).Children(func() {
			icon(c, s, 18, col)
			if !iconOnly {
				ui.Text(c, label).FontSize(15 * uiScale).TextColor(col)
			}
		})
	})
	return box
}

// searchEmpty 空状态：居中大搜索框 + 热词标签 + 提示。
// searchBar 常驻胶囊搜索框（对齐主线 content_search.html：搜索结果页顶部也保留）。
func (a *app) searchBar(c *ui.Context) {
	// 响应式（对齐主线 search_bar：搜索框 flex-grow 弹性 + 按钮固定，
	// 窗口拉窄时搜索框缩小、按钮不溢出；全宽随窗口拉伸）
	ui.Row(c).WidthPercent(100).Height(52 * uiScale).Radius(26 * uiScale).Background(clrField).PaddingX(20 * uiScale).Gap(12 * uiScale).AlignItems(ui.Center).AlignSelf(ui.Center).Children(func() {
		icon(c, icSearch, 20, clrMuted)
		ui.TextInput(c, &a.query).Grow(1).Background(ui.RGBA(0, 0, 0, 0)).
			Placeholder("搜索喜欢的音乐…").TextColor(clrText)
		if gradButton(c, "搜索").Clicked() {
			go a.search()
		}
	})
}

// searchChips 搜索历史 / 默认热词（对齐主线 search-history）。
func (a *app) searchChips(c *ui.Context) {
	if len(a.searchHistory) > 0 {
		ui.Row(c).AlignSelf(ui.Stretch).Gap(8 * uiScale).Wrap().Justify(ui.Center).Children(func() {
			for _, h := range a.searchHistory {
				h := h
				if chip(c, h).Clicked() {
					a.query = h
					go a.search()
				}
			}
			ui.Column(c).Center().Children(func() {
				if chip(c, "清空").Clicked() {
					a.searchHistory = nil
					a.saveHistory()
					a.win.Invalidate()
				}
			})
		})
	} else {
		// 无历史 → 默认热词
		ui.Row(c).Gap(8 * uiScale).Wrap().Justify(ui.Center).Children(func() {
			for _, tag := range []string{"周杰伦", "晴天", "发如雪", "花海"} {
				t := tag
				if chip(c, tag).Clicked() {
					a.query = t
					go a.search()
				}
			}
		})
	}
}

func (a *app) searchEmpty(c *ui.Context) {
	ui.Column(c).Fill().Center().Gap(16 * uiScale).PaddingX(16 * uiScale).Children(func() {
		a.searchBar(c)
		a.searchChips(c)
		ui.Text(c, "输入关键词开始搜索").TextColor(clrMuted).FontSize(15 * uiScale)
	})
}

// chip 小圆角标签。
func chip(c *ui.Context, label string) ui.Element {
	box := ui.Box(c).Height(30 * uiScale).Radius(15 * uiScale).PaddingX(12 * uiScale).AlignItems(ui.Center).Justify(ui.Center)
	box.Draw(func(p *ui.Painter, r ui.Rect) {
		bg := clrCard
		if box.Hovered() {
			bg = clrCardHov
		}
		p.Fill(r, bg, r.H/2)
	})
	box.Children(func() {
		ui.Text(c, label).FontSize(13 * uiScale).TextColor(clrMuted)
	})
	return box
}

// resultsHeader 结果标题行：数量 + 状态 + 播放全部。
func (a *app) resultsHeader(c *ui.Context) {
	ui.Row(c).AlignSelf(ui.Stretch).PaddingX(a.resp(24, 12)).PaddingY(a.resp(14, 10)).AlignItems(ui.Center).Gap(10 * uiScale).Children(func() {
		ui.Text(c, "搜索结果").FontSize(a.resp(18, 16)).Bold().TextColor(clrText)
		ui.Text(c, fmt.Sprintf("%d 首", len(a.results))).FontSize(13 * uiScale).TextColor(clrMuted)
		ui.Spacer(c).Grow(1)
		if a.status != "" && !a.compact() {
			ui.Text(c, a.status).FontSize(13 * uiScale).TextColor(clrMuted).MaxLines(1).MaxWidth(260)
		}
		if a.compact() {
			ui.Text(c, a.status).FontSize(12 * uiScale).TextColor(clrMuted).MaxLines(1).MaxWidth(120)
		}
		// 播放全部（对齐主线 hidden sm:inline：窄屏只图标）
		if a.compact() {
			if iconButton2(c, icPlayAll, a.resp(34, 30), clrText).Clicked() {
				if len(a.results) > 0 {
					a.playAll(a.results, 0)
				}
			}
		} else if gradButton(c, "播放全部").Clicked() {
			if len(a.results) > 0 {
				a.playAll(a.results, 0)
			}
		}
	})
}

// playerBar 完整播放条（学主线 htmx 结构）：字幕条 + 上排（音质/循环/静音/音量）+ 进度 + 下排（封面/收藏/字幕/上一首/播放/下一首）。
func (a *app) playerBar(c *ui.Context) {
	px := a.resp(20, 12)   // 横幅内边距
	gap := a.resp(10, 8)   // 区间距
	igap := a.resp(12, 8)  // 行内间距
	brow := a.resp(40, 32) // 下排按钮
	ui.Column(c).AlignSelf(ui.Stretch).Background(clrBar).PaddingX(px).PaddingY(a.resp(14, 10)).Gap(gap).Children(func() {
		// 无歌曲也显示控制面板（对齐主线 dock 始终显示播放控制/音量/进度；
		// 仅歌曲信息区显示"未在播放"占位）——队列/设置/投屏等页都看到控制面板
		has := a.current != nil
		// 页面内歌词栏（dock 字幕条 lyricCaption）已按用户要求移除：
		// 歌词只保留桌面挂件 / 歌词页，播放控制面板不再显示当前歌词行。
		// 上排：音质 + 循环模式 + 静音 + 音量（对齐主线 dock-tools：全部右对齐）
		ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(igap).Children(func() {
			ui.Spacer(c).Grow(1) // 右对齐（对齐主线工具行在进度条右上方）
			// 音质按钮（对齐主线 d-quality：点击循环 64/128/192k；切换后重播当前歌生效）
			if qualityChip(c, a.qualityLabel(), a.quality > 0).Clicked() {
				a.quality = (a.quality + 1) % 3
				a.win.Invalidate()
				if a.current != nil {
					go a.play(*a.currentToItem())
				}
			}
			if iconButton2(c, icRepeat, a.resp(24, 20), accent(a.loopMode > 0)).Clicked() {
				a.toggleLoop()
			}
			if iconButton2(c, icVol, a.resp(24, 20), accent(a.muted)).Clicked() {
				a.toggleMute()
			}
			a.volumeBar(c, a.resp(120, 84))
		})
		// 进度行：时间 + 滑块 + 时长
		ui.Row(c).AlignSelf(ui.Stretch).Gap(igap).AlignItems(ui.Center).Children(func() {
			ui.Text(c, a.posText).FontSize(13 * uiScale).TextColor(clrMuted).Width(a.resp(44, 36)).TextAlign(ui.End)
			a.progressBar(c, a.progress).Grow(1)
			ui.Text(c, a.durText).FontSize(13 * uiScale).TextColor(clrMuted).Width(a.resp(44, 36))
		})
		// 下排：封面 + 标题 | 收藏 + 字幕开关 + 上一首 + 播放 + 下一首
		ui.Row(c).AlignSelf(ui.Stretch).Gap(a.resp(16, 10)).AlignItems(ui.Center).Children(func() {
			info := ui.Row(c).Grow(1).Gap(brow / 3).AlignItems(ui.Center)
			info.Children(func() {
				if has {
					a.coverTagged(c, a.current.bvid, a.current.pic, 56, true)
					ui.Column(c).Grow(1).Gap(3 * uiScale).Children(func() {
						ui.Text(c, a.current.title).FontSize(a.resp(16, 14)).Bold().TextColor(clrText).MaxLines(1)
						ui.Text(c, a.current.author).FontSize(13 * uiScale).TextColor(clrMuted).MaxLines(1)
					})
				} else {
					// 无歌：占位封面 + "未在播放"（控制面板始终显示）
					a.cover(c, "", "", 56)
					ui.Column(c).Grow(1).Gap(3 * uiScale).Children(func() {
						ui.Text(c, "未在播放").FontSize(a.resp(16, 14)).Bold().TextColor(clrMuted).MaxLines(1)
						ui.Text(c, "搜索并播放一首歌开始").FontSize(13 * uiScale).TextColor(clrMuted).MaxLines(1)
					})
				}
			})
			if info.Clicked() {
				a.showPlayer = true
				a.win.Invalidate()
			}
			// 收藏（对齐主线 d-fav 32×32）
			if iconButton2(c, icHeart, a.resp(32, 28), accent(a.isFav())).Clicked() {
				a.toggleFav()
			}
			// 外部歌词开关（歌词图标，控制桌面挂件外部歌词；对齐主线 d-tick 位置）
			if iconButton2(c, icLyric, a.resp(32, 28), accent(a.lyricShown)).Clicked() {
				a.toggleLyric()
			}
			// 上一首（对齐主线 d-prev 40×40）
			if iconButton2(c, icPrev, brow, clrMuted).Clicked() {
				go a.prev()
			}
			if playCircle(c, a.playing, a.resp(56, 44)).Clicked() {
				a.togglePlay()
			}
			if iconButton2(c, icNext, brow, clrMuted).Clicked() {
				go a.next()
			}
			// 停止投屏/音箱（对齐主线 d-stop：仅投屏中显示）
			if a.castOn != "" {
				if iconButton2(c, icCast, brow, clrPink).Clicked() {
					a.stopCast()
				}
			}
		})
	})
}

func main() {
	a := &app{
		bili: bilibili.NewClient(
			"https://api.bilibili.com",
			"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
			"https://search.bilibili.com/", // 对齐原项目：www.bilibili.com 会触发风控返 HTML
			30*time.Second,
		),
		covers:       map[string]ui.ImageSource{},
		coverLoading: map[string]bool{},
		favSet:       map[string]bool{},
		activeTab:    "search",
		volume:       0.8,
		captionShown: true,
	}
	// 主线 playback 接入：SSOT 状态源 + MyGo 出声后端 + SQLite 持久化
	a.db = ensureDB()
	audioDir := ""
	if dir, err := os.UserConfigDir(); err == nil {
		audioDir = filepath.Join(dir, "mymusic-mygo", "cache", "audio")
		os.MkdirAll(audioDir, 0o755)
	}
	a.svc = newServices(a.db, a.bili, audioDir)
	a.favSet = map[string]bool{}
	a.refreshFavs()    // 收藏恢复自 fav 域包（SQLite 持久化）
	a.restorePrefs()   // 恢复主题/音质/缓存上限（settings 表）
	a.restoreRecent()  // 恢复最近播放
	a.restoreHistory() // 恢复搜索历史
	a.pb = playback.NewPlaybackState()
	a.mygoOut = newMyGoOutput(a.bili)
	a.mygoOut.cache = a.svc.cache                      // 缓存域包注入（命中秒开）
	a.mygoOut.qualFn = func() int { return a.quality } // 音质档实时读取（60k/128k/192k）
	a.mygoOut.SetVolume(int(a.volume * 100))           // 恢复音量（restorePrefs 后 mygoOut 才存在）
	a.pb.SetOutput(a.mygoOut)
	a.pb.SetDB(a.db)
	// 播完自动连播（MyGoOutput.OnEnd → 核心 Next）
	a.mygoOut.SetEndedHook(func() { a.pb.Next() })

	mygo.App.WhenReady(func() {
		a.win = mygo.NewWindow(mygo.WindowOptions{
			Title:     "mymusic-mygo",
			Width:     980,
			Height:    680,
			MinWidth:  500,
			MinHeight: 480,
			Content:   ui.View(a.view),
		})
		// 分辨率适配：窗口尺寸变化时更新宽度感知（断点切换 compact），仅重绘不重建
		b := a.win.Bounds()
		a.winW, a.winH = b.Width, b.Height
		a.win.OnResize(func() {
			if nb := a.win.Bounds(); nb.Width != a.winW || nb.Height != a.winH {
				a.winW, a.winH = nb.Width, nb.Height
				log.Printf("[resize] %dx%d tier=%d(%s)", a.winW, a.winH, a.tier(), map[int]string{0: "narrow", 1: "compact", 2: "normal"}[a.tier()])
				a.win.Invalidate()
			}
		})
		// 恢复队列（window 就绪后再恢复，自动播放的进度上报不会撞未创建窗口）
		a.pb.Restore(a.db)
		// 音箱：已有持久化 token（与主线 web 共用 speaker.json）→ 刷新设备
		if a.svc != nil && a.svc.speakerLoggedIn() {
			go a.refreshSpeakers()
		}
		// 订阅主线状态流 → 渲染；MyGo 本地 audio 进度 → 上报 SSOT
		go a.pbLoop()
		go a.progressReporter()
		// 自检演示：MYGO_DEMO=1 时自动搜索+播放+开挂件（试水验证用）。
		if os.Getenv("MYGO_DEMO") != "" {
			go a.demo()
		}
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

// searchNotFound 搜索无结果空态（对齐主线 results.html 空态：未找到相关音乐 + 换关键词）。
func (a *app) searchNotFound(c *ui.Context) {
	ui.Column(c).Fill().Center().Gap(10 * uiScale).Children(func() {
		icon(c, icSearch, 44, ui.RGBA(0xff, 0xff, 0xff, 0.08))
		ui.Text(c, "未找到相关音乐").FontSize(17 * uiScale).Bold().TextColor(clrText)
		ui.Text(c, "换个关键词试试").FontSize(13 * uiScale).TextColor(clrMuted)
		ui.Spacer(c).Height(8 * uiScale)
		if gradButton(c, "换个词继续搜").Clicked() {
			a.query = ""
			a.searched = false
			a.results = nil
			a.win.Invalidate()
		}
	})
}
