//go:build native

package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	_ "github.com/egoist/mygo/audio/aac" // mp4a (AAC) 解码器
	_ "github.com/egoist/mygo/audio/mp4" // m4a / fMP4 解码
	"github.com/egoist/mygo/ui"

	"homecast/service/playback"

	"mymusic-mygo/bilibili"
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
	pb      *playback.PlaybackState
	mygoOut *MyGoOutput
	db      *sql.DB

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
	volume     float64
	loopMode   int // 0 列表循环 1 单曲 2 随机
	favorites  []bilibili.SearchItem
	favMap     map[string]bool

	// 静音 / 字幕条（学主线结构）
	muted        bool
	lastVolume   float64
	captionShown bool
}

func (a *app) view(c *ui.Context) {
	ui.Column(c).Fill().Background(clrBg).Children(func() {
		a.navBar(c)
		switch a.activeTab {
		case "queue":
			a.queuePage(c)
		case "fav":
			a.favPage(c)
		case "cast":
			a.castPage(c)
		case "settings":
			a.settingsPage(c)
		default:
			if len(a.results) > 0 {
				a.resultsHeader(c)
				ui.Scroll(c).Grow(1).PaddingX(24).Gap(8).Children(func() {
					ui.Column(c).Gap(8).Children(func() {
						for _, r := range a.results {
							a.listRow(c, r)
						}
					})
				})
			} else {
				a.searchEmpty(c)
			}
		}
		a.playerBar(c)
	})
	if a.showPlayer {
		a.nowPlaying(c)
	}
}

// navBar 顶部导航：logo + 导航 tab（复刻 wails3 的信息架构）。
func (a *app) navBar(c *ui.Context) {
	ui.Row(c).AlignSelf(ui.Stretch).Height(64).PaddingX(20).Gap(6).AlignItems(ui.Center).Children(func() {
		icon(c, icMusic, 24, clrText)
		ui.Spacer(c).Width(10)
		if navTab(c, icList, "队列", a.activeTab == "queue").Clicked() {
			a.setTab("queue")
		}
		if navTab(c, icSearch, "搜索", a.activeTab == "search").Clicked() {
			a.setTab("search")
		}
		if navTab(c, icFav, "收藏", a.activeTab == "fav").Clicked() {
			a.setTab("fav")
		}
		if navTab(c, icCast, "投屏", a.activeTab == "cast").Clicked() {
			a.setTab("cast")
		}
		if navTab(c, icSet, "设置", a.activeTab == "settings").Clicked() {
			a.setTab("settings")
		}
		ui.Spacer(c).Grow(1)
		ly := icList
		if a.lyricShown {
			ly = icList
		}
		if iconButton2(c, ly, 32, clrMuted).Clicked() {
			a.toggleLyric()
		}
	})
	ui.Box(c).AlignSelf(ui.Stretch).Height(1).Draw(func(p *ui.Painter, r ui.Rect) {
		p.Fill(r, clrCard, 0)
	})
}

// navTab 导航项：当前高亮为紫渐变胶囊；hover 淡高亮。
func navTab(c *ui.Context, s *ui.SVG, label string, active bool) ui.Element {
	box := ui.Box(c).Height(40).Radius(20).PaddingX(16)
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
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			icon(c, s, 18, col)
			ui.Text(c, label).FontSize(15).TextColor(col)
		})
	})
	return box
}

// searchEmpty 空状态：居中大搜索框 + 热词标签 + 提示。
func (a *app) searchEmpty(c *ui.Context) {
	ui.Column(c).Fill().Center().Gap(16).Children(func() {
		ui.Row(c).WidthPercent(60).Height(52).Radius(26).Background(clrField).PaddingX(20).Gap(12).AlignItems(ui.Center).Children(func() {
			icon(c, icSearch, 20, clrMuted)
			ui.TextInput(c, &a.query).Grow(1).Background(ui.RGBA(0, 0, 0, 0)).
				Placeholder("搜索喜欢的音乐…").TextColor(clrText)
			if gradButton(c, "搜索").Clicked() {
				go a.search()
			}
		})
		// 热词/历史标签（对齐原项目）
		ui.Row(c).Gap(8).Children(func() {
			for _, tag := range []string{"周杰伦", "晴天", "发如雪", "花海"} {
				t := tag
				if chip(c, tag).Clicked() {
					a.query = t
					go a.search()
				}
			}
		})
		ui.Text(c, "输入关键词开始搜索").TextColor(clrMuted).FontSize(15)
	})
}

// chip 小圆角标签。
func chip(c *ui.Context, label string) ui.Element {
	box := ui.Box(c).Height(30).Radius(15).PaddingX(12)
	box.Draw(func(p *ui.Painter, r ui.Rect) {
		bg := clrCard
		if box.Hovered() {
			bg = clrCardHov
		}
		p.Fill(r, bg, r.H/2)
	})
	box.Children(func() {
		ui.Text(c, label).FontSize(13).TextColor(clrMuted)
	})
	return box
}

// resultsHeader 结果标题行：数量 + 状态 + 播放全部。
func (a *app) resultsHeader(c *ui.Context) {
	ui.Row(c).AlignSelf(ui.Stretch).PaddingX(24).PaddingY(14).AlignItems(ui.Center).Gap(10).Children(func() {
		ui.Text(c, "搜索结果").FontSize(18).Bold().TextColor(clrText)
		ui.Text(c, fmt.Sprintf("%d 首", len(a.results))).FontSize(13).TextColor(clrMuted)
		ui.Spacer(c).Grow(1)
		if a.status != "" {
			ui.Text(c, a.status).FontSize(13).TextColor(clrMuted).MaxLines(1).MaxWidth(260)
		}
		if gradButton(c, "播放全部").Clicked() {
			if len(a.results) > 0 {
				a.playAll(a.results, 0)
			}
		}
	})
}

// playerBar 完整播放条（学主线 htmx 结构）：字幕条 + 上排（音质/循环/静音/音量）+ 进度 + 下排（封面/收藏/字幕/上一首/播放/下一首）。
func (a *app) playerBar(c *ui.Context) {
	ui.Column(c).AlignSelf(ui.Stretch).Background(clrBar).PaddingX(20).PaddingY(14).Gap(10).Children(func() {
		if a.current == nil {
			ui.Text(c, "未在播放").TextColor(clrMuted).FontSize(15).PaddingY(24)
			return
		}
		// 字幕条（对齐主线：当前歌词行 + 关闭）
		a.lyricCaption(c)
		// 上排：音质 + 循环模式 + 静音 + 音量
		ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(12).Children(func() {
			ui.Text(c, "192k").FontSize(13).TextColor(clrMuted)
			if iconButton2(c, icRepeat, 24, accent(a.loopMode > 0)).Clicked() {
				a.toggleLoop()
			}
			ui.Text(c, a.loopLabel()).FontSize(13).TextColor(clrMuted)
			ui.Spacer(c).Grow(1)
			if iconButton2(c, icVol, 24, accent(a.muted)).Clicked() {
				a.toggleMute()
			}
			a.volumeBar(c, 120)
		})
		// 进度行：时间 + 滑块 + 时长
		ui.Row(c).AlignSelf(ui.Stretch).Gap(12).AlignItems(ui.Center).Children(func() {
			ui.Text(c, a.posText).FontSize(13).TextColor(clrMuted).Width(44).TextAlign(ui.End)
			a.progressBar(c, a.progress).Grow(1)
			ui.Text(c, a.durText).FontSize(13).TextColor(clrMuted).Width(44)
		})
		// 下排：封面 + 标题 | 收藏 + 字幕开关 + 上一首 + 播放 + 下一首
		ui.Row(c).AlignSelf(ui.Stretch).Gap(16).AlignItems(ui.Center).Children(func() {
			info := ui.Row(c).Grow(1).Gap(12).AlignItems(ui.Center)
			info.Children(func() {
				a.cover(c, a.current.bvid, a.current.pic, 60)
				ui.Column(c).Grow(1).Gap(3).Children(func() {
					ui.Text(c, a.current.title).FontSize(16).Bold().TextColor(clrText).MaxLines(1)
					ui.Text(c, a.current.author).FontSize(13).TextColor(clrMuted).MaxLines(1)
				})
			})
			if info.Clicked() {
				a.showPlayer = true
				a.win.Invalidate()
			}
			if iconButton2(c, icHeart, 40, accent(a.isFav())).Clicked() {
				a.toggleFav()
			}
			if iconButton2(c, icMsg, 40, accent(a.captionShown)).Clicked() {
				a.toggleCaption()
			}
			if iconButton2(c, icPrev, 40, clrMuted).Clicked() {
				go a.prev()
			}
			if playCircle(c, a.playing, 60).Clicked() {
				a.togglePlay()
			}
			if iconButton2(c, icNext, 40, clrMuted).Clicked() {
				go a.next()
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
		favMap:       map[string]bool{},
		activeTab:    "search",
		volume:       0.8,
		captionShown: true,
	}
	// 主线 playback 接入：SSOT 状态源 + MyGo 出声后端 + SQLite 持久化
	a.db = ensureDB()
	a.pb = playback.NewPlaybackState()
	a.mygoOut = newMyGoOutput(a.bili)
	a.pb.SetOutput(a.mygoOut)
	a.pb.SetDB(a.db)
	a.pb.Restore(a.db)
	// 播完自动连播（MyGoOutput.OnEnd → 核心 Next）
	a.mygoOut.SetEndedHook(func() { a.pb.Next() })

	mygo.App.WhenReady(func() {
		a.win = mygo.NewWindow(mygo.WindowOptions{
			Title:     "mymusic-mygo",
			Width:     980,
			Height:    680,
			MinWidth:  720,
			MinHeight: 520,
			Content:   ui.View(a.view),
		})
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
