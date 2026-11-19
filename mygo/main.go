//go:build native

package main

import (
	"database/sql"
	"log"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	_ "github.com/egoist/mygo/audio/aac" // mp4a (AAC) 解码器
	_ "github.com/egoist/mygo/audio/mp4" // m4a / fMP4 解码
	_ "github.com/egoist/mygo/audio/mp3" // mp3 解码（网易云等音源返回 MP3，缺它则无声）
	"github.com/egoist/mygo/ui"

	"homecast/service/cast"
	"homecast/service/lyric"
	"homecast/service/playback"
	"homecast/service/songlist"
	spk "homecast/service/speaker"

	"homecast/bilibili"

	"homecast/xiaoai"
	lyricplug "mymusic-mygo/lyric"
	"mymusic-mygo/plugin"
	"mymusic-mygo/source"
	"mymusic-mygo/target"
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
	link         plugin.Link // 稳定链路 API（搜索→播放→歌词统一入口，多端复用核心）
	db           *sql.DB
	svc          *services              // 主线域包句柄（fav/lyric/cast/speaker）
	src          source.MusicSource     // 当前音源（阶段 2 解耦：UI 只认接口，不认具体源）
	castOn       string                 // 正在投屏的设备名（空=未投屏，对齐主线 d-stop 显示）
	castDevices  []cast.DeviceDTO       // DLNA 设备列表（cast 域包）
	btDevices    []target.BtDevice      // 蓝牙设备列表（bluetoothctl 扫描）
	castStatus   map[string]string      // udn → 设备状态（点「查状态」后填，默认「在线」，对齐主线 cast-status span）
	speakers     []xiaoai.SpeakerDevice // 小米音箱设备（speaker 域包）
	recent       []bilibili.SearchItem  // 最近播放（对齐主线 hc:recent，最多 30）
	lastVolSave  time.Time              // 音量落库节流时间戳
	qrStatus     string                 // 音箱扫码登录状态文案
	lastLyricIdx int                    // 上次高亮歌词行（KTV 滚动跟随去重）
	lyrDrawer    int                    // 窄档歌词抽屉：0 关 1 候选 2 工具（对齐主线 ly-drawer-*）
	searched     bool                   // 是否已执行过搜索（区分空结果态）
	eqPhase      int                    // dock 封面 EQ 频谱相位（播放中推进，对齐主线 d-eq 动画）
	qrImage      string                 // 二维码 data URI（渲染用）
	tokenInput   string                 // 音箱 token 输入框值（粘贴 token 登录）
	ctxItem      *bilibili.SearchItem   // 右键菜单目标（非空=显示）
	ctxX, ctxY   float32                // 菜单弹出位置（窗口坐标）
	clipboard    string                 // 待复制链接（mygo 暂无线程安全 clipboard API，先字段+状态提示）

	// 搜索
	query   string
	results []bilibili.SearchItem
	// 搜索结果按源分组（插件分组：每个音源一个独立分区，保持注册顺序）
	resultsBySource []srcResult
	loading         atomic.Bool
	status          string
	// 推荐页（内容靠插件实现）：每插件缓存推荐歌曲 + 加载态 + 详情页开关
	recommendData       map[string][]source.Song
	recommendLoaded     map[string]bool
	recommendDetailOpen bool // 推荐详情页是否打开（来源页预览 → 点击进详情）

	// 分页（各列表页当前页，每页 20 条；搜索结果每源独立分页）
	pg          map[string]int // 列表页当前页：queue/recent/fav/detail（本地切片分页）
	searchPage  int            // 搜索结果当前页（兼容保留）
	searchTotal int            // 搜索结果总条数
	searchPages map[string]int // 每源独立分页（网易云/bilibili 各自，源名→page）

	// 播放（主线程访问；播放器由 mygoOut 持有，audio.Player 自身安全）
	current    *track
	playing    bool
	progress   float64 // 0..1
	scrubbing  bool    // 进度条拖动中（暂停 pbLoop 覆盖 a.progress，避免 thumb 跳回）
	posText    string
	durText    string
	lyricText  string
	lyricShown bool
	// 桌面挂件聚焦（事件驱动，无轮询）：默认窗口只有左边缘把手区接收鼠标
	// （其余穿透），鼠标进把手 1 秒（AfterFunc）→ 全窗口可交互，移出恢复穿透
	lyricHot        bool
	lyricHoverStart time.Time
	lyricLooping    bool // 桌面挂件聚焦轮询 goroutine 在跑（防重复启动）

	// 封面缓存（主线程；异步加载经 win.Update 写）
	covers       map[string]ui.ImageSource
	coverLoading map[string]bool
	coverOrder   []string // LRU 顺序：封面缓存超过上限时驱逐最旧的

	// 播放控制面板 / 队列 / 收藏
	showPlayer   bool
	activeTab    string // search|queue|fav|cast|settings
	settingsTab  int    // 设置页子 tab：0=系统设置 1=插件
	pluginDetail string // 插件配置子页：当前编辑的插件 "type/name"（空=插件列表页）
	playlist     []bilibili.SearchItem
	playIndex    int
	posSec       float64 // 当前播放秒（歌词定位用）
	volume       float64
	loopMode     int // 0 顺序 1 列表循环 2 单曲 3 随机（对齐主线 4 态）
	favorites    []bilibili.SearchItem
	favSet       map[string]bool
	// 歌单（用户自建、可命名、可多份；与 playlist=播放队列 区分）
	songlists           []songlist.Songlist // 歌单列表（卡片用）
	songlistOpen        int64               // 当前打开的歌单 id（0=列表页）
	songlistCur         songlist.Songlist   // 当前打开的详情
	songlistItems       []songlist.Item     // 当前打开的详情歌曲
	songlistNewOpen     bool                // 新建歌单输入行是否展开
	songlistNameInput   string              // 新建歌单输入值
	songlistRenameOpen  bool                // 重命名输入行是否展开
	songlistRenameInput string              // 重命名输入值
	addToPick           bool                // 「添加到歌单」浮层是否显示（ctxItem 为目标歌）
	ctxPosSet           bool                // 右键菜单位置是否已记录（打开那刻记一次，之后固定不跟鼠标）

	quality       int      // 0=64k 1=128k 2=192k（对齐主线 d-quality 三档循环）
	lyricSize     int      // 0-4 歌词字号档（对齐主线 14/16/18/22/26px）
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
	muted      bool
	lastVolume float64
	// 音箱模式（source=speaker）：控制面（音量/暂停/进度）复用主线 playback（out 自动分派音箱）。
	spkMode bool   // source=speaker
	spkDid  string // 目标音箱 did（对齐主线 State.SpkDID）

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
				// 搜索页：搜索框 + 热词 + 结果（未搜索时显示风格分类）
				ui.Column(c).Grow(1).Fill().Padding(4*uiScale, a.resp(24, 12), 0, a.resp(24, 12)).Gap(8 * uiScale).Children(func() {
					// 搜索页整页滚动（对齐 recent 页模式）：搜索框+热词+结果一起滚，不做组件内部滚动条
					ui.Scroll(c).Grow(1).Scrollbars(false).Children(func() {
						ui.Column(c).Gap(8 * uiScale).Children(func() {
							if len(a.results) > 0 {
								a.searchBar(c)
								a.searchChips(c)
								a.resultsHeader(c)
								// 搜索结果按源分区（插件分组：每个音源一个独立区块 + 每源独立分页按钮在下方）
								for _, grp := range a.resultsBySource {
									a.searchSourceGroup(c, grp)
								}
							} else if a.searched {
								a.searchBar(c)
								a.searchChips(c)
								a.searchNotFound(c)
							} else if a.recommendDetailOpen {
								// 推荐详情页（分插件完整展示；从来源页预览点击进入）
								a.recommendDetail(c)
							} else {
								// 来源页空态：推荐预览（1-2 行精简，吸引点击）在上 + 风格分类在下
								a.searchEmpty(c)
							}
						})
					})
				})
			}
		})
		a.playerBar(c)
	})
	if a.showPlayer {
		a.nowPlaying(c)
	}
	// 右键上下文菜单（对齐主线 contextmenu.js）
	if a.ctxItem != nil && !a.addToPick {
		a.ctxMenu(c)
	}
	// 「添加到歌单」浮层（ctxItem 为目标歌）
	if a.addToPick && a.ctxItem != nil {
		a.addToSonglistSheet(c)
	}
}

// navBar 顶部导航——结构随档位切换（对齐主线：桌面完整导航 vs md:hidden 汉堡+侧滑面板）。
func chip(c *ui.Context, label string) ui.Element {
	box := ui.Box(c).Height(30 * uiScale).Radius(8 * uiScale).PaddingX(12 * uiScale).AlignItems(ui.Center).Justify(ui.Center)
	box.Draw(func(p *ui.Painter, r ui.Rect) {
		bg := clrCard
		if box.Hovered() {
			bg = clrCardHov
		}
		p.Fill(r, bg, 8)
	})
	box.Children(func() {
		ui.Text(c, label).FontSize(13 * uiScale).TextColor(clrMuted)
	})
	return box
}

// resultsHeader 结果标题行：数量 + 状态 + 播放全部。
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
			if heartButton(c, a.isFav(), a.resp(32, 28), func() {}).Clicked() {
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
		covers:          map[string]ui.ImageSource{},
		coverLoading:    map[string]bool{},
		favSet:          map[string]bool{},
		pg:              map[string]int{},
		searchPages:     map[string]int{},
		recommendData:   map[string][]source.Song{},
		recommendLoaded: map[string]bool{},
		activeTab:       "search",
		volume:          0.8,
	}
	// 主线 playback 接入：SSOT 状态源 + MyGo 出声后端 + SQLite 持久化
	a.db = ensureDB()
	// 插件设置统一存储（系统统一管理）：注入 SettingStore，JS/Go 插件设置落 settings 表。
	plugin.SetSettingStore(newSettingStore(a.db))
	audioDir := ""
	if dir, err := os.UserConfigDir(); err == nil {
		audioDir = filepath.Join(dir, "mymusic-mygo", "cache", "audio")
		os.MkdirAll(audioDir, 0o755)
	}
	// ★必须先设 HC_PORT 再构造 svc：NewSpeakerService 构造时读 HC_PORT 拼音箱拉流 URL。
	// 之前 startStreamServer（下一行）才 set，导致 speaker 读到默认 28976，而供给端点监听
	// 28977 → 音箱拉 28976 无服务 → 无声。这里提前 set，使音箱拉流端口=供给端点端口。
	if os.Getenv("HC_PORT") == "" {
		os.Setenv("HC_PORT", "28977")
	}
	a.svc = newServices(a.db, a.bili, audioDir)
	// B站登录 Cookie（可选）：有则用自己的（登录态/高清），空=匿名（仅 buvid3）。
	a.bili.Cookie = plugin.GetSetting("source", "bilibili", "cookie", "")
	// 一切皆插件：注册内置插件（音源/歌词）。第三方插件 = 各自 init() 注册，主程序空导入。
	if a.svc != nil && a.svc.music != nil {
		plugin.RegisterSource(source.NewBilibili(a.svc.music)) // B站音乐源
		plugin.RegisterSettings("source", "bilibili", []plugin.Setting{{
			Key: "cookie", Name: "登录 Cookie", Type: plugin.SettingText, Default: "",
			Desc: "粘贴 B站登录态 Cookie（浏览器 www.bilibili.com 登录后复制），空=匿名（仅 buvid3）",
		}})
	}
	plugin.RegisterLyric(lyricplug.NewNetease()) // 网易云歌词源
	// 插件元数据（插件页展示）
	plugin.RegisterInfo(plugin.Info{Name: "bilibili", Type: "source", Icon: "bilibili", Version: "1.0", Desc: "B站音乐音源（搜索/取流/歌词，复用主线 music 域）", Order: 10})
	// B站音源插件设置：搜索条数/分页自控（插件声明，系统统一存储 SettingStore）
	plugin.RegisterSettings("source", "bilibili", []plugin.Setting{
		{Key: "searchSize", Name: "搜索条数", Type: plugin.SettingSelect, Options: []string{"20", "40", "60"}, Default: "40", Desc: "每次搜索请求条数（插件自控，分页同尺寸）"},
	})
	plugin.RegisterInfo(plugin.Info{Name: "netease", Type: "lyric", Icon: "netease", Version: "1.0", Desc: "网易云歌词源（复用主线 lyric 域 GetLyricLines）", Order: 20})
	plugin.RegisterInfo(plugin.Info{Name: "local", Type: "target", Icon: "local", Version: "1.0", Desc: "本机声卡播放（默认输出路径）", Order: 30})
	plugin.RegisterInfo(plugin.Info{Name: "cast", Type: "target", Icon: "cast", Version: "1.0", Desc: "DLNA 投屏目标（同网段发现播放器/电视）", Order: 40})
	plugin.RegisterInfo(plugin.Info{Name: "bluetooth", Type: "target", Icon: "bluetooth", Version: "1.0", Desc: "蓝牙目标（bluetoothctl 扫描，音频路由到蓝牙 sink）", Order: 50})
	plugin.RegisterInfo(plugin.Info{Name: "speaker", Type: "target", Icon: "speaker", Version: "1.0", Desc: "小爱音箱目标（米家扫码登录，投送当前歌）", Order: 60})
	// JS 沙箱插件：扫描 plugins/ 加载第三方 .js 音源/歌词（sobek 纯 Go，无 cgo）。
	// 单个插件失败不拖垮启动（errs 收集，日志列出原因）。
	if errs, err := plugin.LoadFromDir("plugins"); err != nil {
		log.Printf("js plugin dir: %v", err)
	} else {
		for _, e := range errs {
			log.Printf("js plugin load: %v", e)
		}
	}
	// 当前源 = 注册表首个（B站）；未来网易云/本地/插件源都注册进表，UI 可选
	if names := plugin.SourceNames(); len(names) > 0 {
		a.src = plugin.Sources()[names[0]]
	}
	a.startStreamServer() // 音箱供给端点（/api/v1/music/stream，transcode mp3 秒开 + seek）
	a.favSet = map[string]bool{}
	a.refreshFavs()      // 收藏恢复自 fav 域包（SQLite 持久化）
	a.refreshSonglists() // 歌单列表恢复自 songlist 域包（SQLite 持久化）
	a.restorePrefs()     // 恢复主题/音质/缓存上限（settings 表）
	a.restoreRecent()    // 恢复最近播放
	a.restoreHistory()   // 恢复搜索历史
	a.pb = playback.NewPlaybackState()
	a.link = NewLink(a.pb) // 稳定链路 API 实现（播放核心注入）
	a.mygoOut = newMyGoOutput(a.bili)
	if a.src != nil {
		a.mygoOut.setSource(a.src) // 音源接口注入（阶段 2 解耦：取流走接口）
	}
	if a.svc != nil && a.svc.music != nil {
		a.mygoOut.setMusic(a.svc.music) // 取流复用主线（view 缓存）
	}
	// 一切皆插件：本机声卡也注册为 target 插件（默认播放路径）
	plugin.RegisterTarget(target.NewLocal(a.mygoOut))
	a.mygoOut.cache = a.svc.cache                      // 缓存域包注入（命中秒开）
	a.mygoOut.qualFn = func() int { return a.quality } // 音质档实时读取（60k/128k/192k）
	a.mygoOut.onErr = func(msg string) { // 取流失败 UI 提示（付费/VIP/需登录）
		a.win.Update(func() { a.status = msg })
	}
	a.mygoOut.SetVolume(int(a.volume * 100))           // 恢复音量（restorePrefs 后 mygoOut 才存在）
	a.pb.SetOutput(a.mygoOut)
	a.pb.SetDB(a.db)
	// 音箱输出工厂：SwitchSpeaker(did) 时创建绑定该 did 的 SpeakerOutput（对齐主线
	// server.go SetOutputFactory）。缺失时 SwitchSpeaker 的 outFactory 分支直接跳过
	// → 核心 out 不换，progressLoop 读不到音箱进度、切回 web 双响（主线注释同款）。
	// SpeakerOutput.Position 从 svc.Progress(did) 读音箱本地模拟进度（startProgress）。
	if a.svc != nil && a.svc.speaker != nil {
		a.pb.SetOutputFactory(func(did string) playback.Output {
			out := spk.NewSpeakerOutput(a.svc.speaker, did)
			out.SetEndedHook(func() { a.pb.Next() }) // 核心自动连播（对齐主线）
			return out
		})
		// 音箱播完钩子（对齐主线 server.go EndedHook）：定时器到点唯一出口——
		// 有下一首 → 核心自动连播；无下一首（单曲/最后一首）→ 核心落停播态。
		// 缺此钩子：播完什么都不做，核心一直“播放中”，设备卡永远撒谎。
		a.svc.speaker.EndedHook = func(did, nextBVID string) {
			if nextBVID == "" {
				a.pb.SpeakerFinished(did)
				return
			}
			a.pb.Next()
		}
	}
	// 播完自动连播（对齐主线 audio ended：single 前端 currentTime=0 重播当前歌，
	// 其余 cmd('next') 后端顺序环绕）。single 用 seek(0)+Resume 重播（MyGo 本地 audio）。
	a.mygoOut.SetEndedHook(func() {
		if a.loopMode == 2 { // 单曲循环：重播当前歌
			a.seek(0)
			_ = a.mygoOut.Resume()
			return
		}
		a.pb.Next()
	})

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
		// restore 只写后端 p.cur 不广播（主线 GUI 页面载人时自行拉状态，不依赖 push）；
		// mygo 靠事件流渲染 → 主动拉一次快照喂 applyState，否则队列页启动显示 0，
		// 要等用户播一首触发广播才补上（bug）。
		a.win.Update(func() { a.applyState(a.pb.Snapshot()) })
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
