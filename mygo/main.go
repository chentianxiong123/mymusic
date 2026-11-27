//go:build native

package main

import (
	"database/sql"
	"homecast/model"
	"homecast/service/widget"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	_ "github.com/egoist/mygo/audio/aac" // mp4a (AAC) 解码器
	_ "github.com/egoist/mygo/audio/mp3" // mp3 解码（网易云等音源返回 MP3，缺它则无声）
	_ "github.com/egoist/mygo/audio/mp4" // m4a / fMP4 解码
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
	"mymusic-mygo/widgetoverlay"
)

// track 当前曲目。
type track struct {
	bvid, cid, title, author, url, pic, duration string
	src                                          string // 歌来源名（typename：bilibili/netease/migu/...）
}

// app 是窗口显示的状态。view 在主线程从它构建界面；
// goroutine 只通过 win.Update / RunOnMain 改它。
type app struct {
	win      *mygo.Window // 主窗口
	lyricWin *mygo.Window // 歌词挂件窗口（可空）
	bili     *bilibili.Client

	// 主线 SSOT 接入：业务状态/队列/模式/持久化全在 playback 域包（MyGo 只渲染+发命令）
	pb                *playback.PlaybackState
	mygoOut           *MyGoOutput
	link              plugin.Link // 稳定链路 API（搜索→播放→歌词统一入口，多端复用核心）
	db                *sql.DB
	svc               *services              // 主线域包句柄（fav/lyric/cast/speaker）
	src               source.MusicSource     // 当前音源（阶段 2 解耦：UI 只认接口，不认具体源）
	neteaseSrc        *source.Netease        // Go 内置网易云源（登录态热更新用）
	castOn            string                 // 正在投屏的设备名（空=未投屏，对齐主线 d-stop 显示）
	currentRef        model.SongRef          // 1.2.5 当前歌曲的跨源引用（输出层统一传它）
	supplier          *Supplier              // 统一取流供给（本机播放/HTTP/DLNA 共用）
	localPausedByCast bool                   // 投送远程时本机被暂停（切回本机时恢复）
	castDevices       []cast.DeviceDTO       // DLNA 设备列表（cast 域包）
	castStatus        map[string]string      // udn → 设备状态（点「查状态」后填，默认「在线」，对齐主线 cast-status span）
	speakers          []xiaoai.SpeakerDevice // 小米音箱设备（speaker 域包）
	recent            []bilibili.SearchItem  // 最近播放（对齐主线 hc:recent，最多 30）
	lastVolSave       time.Time              // 音量落库节流时间戳
	qrStatus          string                 // 音箱扫码登录状态文案
	lastLyricIdx      int                    // 上次高亮歌词行（KTV 滚动跟随去重）
	lyrDrawer         int                    // 窄档歌词抽屉：0 关 1 候选 2 工具（对齐主线 ly-drawer-*）
	searched          bool                   // 是否已执行过搜索（区分空结果态）
	eqPhase           int                    // dock 封面 EQ 频谱相位（播放中推进，对齐主线 d-eq 动画）
	qrImage           string                 // 二维码 data URI（渲染用）
	tokenInput        string                 // 音箱 token 输入框值（粘贴 token 登录）
	ctxItem           *bilibili.SearchItem   // 右键菜单目标（非空=显示）
	ctxX, ctxY        float32                // 菜单弹出位置（窗口坐标）
	clipboard         string                 // 待复制链接（mygo 暂无线程安全 clipboard API，先字段+状态提示）

	// 搜索
	query   string
	results []bilibili.SearchItem
	// 搜索结果按源分组（插件分组：每个音源一个独立分区，保持注册顺序）
	resultsBySource []srcResult
	loading         atomic.Bool
	status          string
	// 渠道选择：搜索栏下拉框选一个源——一次只搜一个、只显示一个源的结果
	activeSource     string // 当前选中的源名（空=默认 bilibili）
	lastSearchSource string // 上次搜索用的源（切换源时清空旧结果）
	// 插件拖拽排序：官方 ui.List + ListState.Reorder（三类各一个状态，跨帧保持）
	pluginLists map[string]*ui.ListState // key=kind(source/lyric/target)
	// 推荐页（内容靠插件实现）：每插件缓存推荐歌曲 + 加载态 + 详情页开关
	recommendData       map[string][]source.Song
	recommendLoaded     map[string]bool
	recommendDetailOpen bool   // 推荐详情页是否打开（来源页预览 → 点击进详情）
	activeRecommend     string // 当前选中的推荐插件名（一次只显示一个，对齐搜索栏音源下拉）
	activeChart         string // 当前选中的排行平台（网易云/酷狗）

	// 分页（各列表页当前页，每页 20 条；搜索结果每源独立分页）
	pg          map[string]int // 列表页当前页：queue/recent/fav/detail（本地切片分页）
	searchPage  int            // 搜索结果当前页（兼容保留）
	searchTotal int            // 搜索结果总条数
	searchPages map[string]int // 每源独立分页（网易云/bilibili 各自，源名→page）

	// 播放（主线程访问；播放器由 mygoOut 持有，audio.Player 自身安全）
	current        *track
	playing        bool
	progress       float64 // 0..1
	scrubbing      bool    // 进度条拖动中（暂停 pbLoop 覆盖 a.progress，避免 thumb 跳回）
	posText        string
	durText        string
	lyricText      string
	lyricShown     bool
	lastPBPlaying  bool                // pbLoop 上次播放态（progress 帧只在状态变/播放中重绘，省电）
	widgetState    *widget.WidgetState // 挂件状态机（安卓浮窗 SSE 数据源）
	androidLyricOn bool                // 安卓悬浮歌词开关状态
	// 桌面挂件聚焦（纯事件驱动，无轮询）：SetInputRegion 只让歌词带接收鼠标，
	// Hovered() 变化（鼠标进出）触发展开/收起控制条 + 切换 input region。
	lyricHot bool

	// 封面缓存（主线程；异步加载经 win.Update 写）
	covers       map[string]ui.ImageSource
	coverLoading map[string]bool
	coverOrder   []string                  // LRU 顺序：封面缓存超过上限时驱逐最旧的
	blurCovers   map[string]ui.ImageSource // 模糊版封面（降采样 12px 放大铺满=背景模糊）

	// 播放控制面板 / 队列 / 收藏
	showPlayer      bool
	activeTab       string         // search|queue|fav|cast|settings
	restoreCfg      restoreConfig  // 启动恢复配置（所有退出记忆开关）
	showRestoreCfg  bool           // 启动恢复配置弹窗
	showLimitDlg    bool           // 上限确认弹窗（所有档位都走它）
	limitKind       string         // "cache"/"lyric"：当前改哪个上限
	limitPendingIdx int            // 待确认档位（0-3 预置，4=自定义）
	limitPendingVal int            // 待确认/输入数值
	limitBuf        string         // 弹窗输入框 buffer
	lyricBindSel    string         // 歌词源下拉选中值（持久，避免每帧重建抖动）
	lyricBindOwn    string         // 当前下拉归属的音源（切换时重新同步）
	qrLoginSession  *qrSession     // 音源扫码登录会话（非空=弹窗显示）
	qrLoginImg      ui.ImageSource // 二维码图片（解码后）
	qrLoginPNG      []byte         // 二维码原始 PNG（复制用：粘贴到微信/QQ发手机扫）
	qrSpeakerPNG    []byte         // 音箱二维码原始 PNG（复制用）
	qrLoginStop     chan struct{}  // 停止扫码轮询
	showLoginDialog bool           // 登录方式弹窗
	loginDialogSrc  string         // 登录方式弹窗对应的音源
	loginCookieBuf  string         // 登录方式弹窗 Cookie 输入框
	lastSavedSource string         // 音源下拉保存去重（避免每帧写盘）
	settingsTab     int            // 设置页子 tab：0=系统设置 1=插件 2=日志
	pluginTab       int            // 插件页子 tab：0=音源 1=歌词 2=输出
	pluginDetail    string         // 插件配置子页：当前编辑的插件 "type/name"（空=插件列表页）
	// 日志页筛选/交互状态
	logLevelFilter    int           // 0=全部 1=DEBUG 2=INFO 3=WARN 4=ERROR
	logCatFilter      string        // 分类筛选（""=全部）
	logSearch         string        // 关键词搜索
	logAutoScroll     bool          // 自动刷新（默认开；仅日志页可见时生效）
	logDetail         *LogEntry     // 点日志行展开全文（nil=关）
	logLimit          int           // 显示最近 N 条（0=全部；默认 300 防卡顿）
	logConfirmClear   bool          // 清空确认弹窗
	statsConfirmClear bool          // 清空播放统计确认弹窗
	logWatching       bool          // 日志页当前是否可见（控制刷新 goroutine 启停）
	logWatchStop      chan struct{} // 停止刷新信号
	playlist          []bilibili.SearchItem
	playIndex         int
	posSec            float64 // 当前播放秒（歌词定位用）
	volume            float64
	loopMode          int // 0 顺序 1 列表循环 2 单曲 3 随机（对齐主线 4 态）
	pointPlayMode     int // 点单曲方式：0 替换播放列表（网易云默认） 1 仅追加到播放列表
	favorites         []bilibili.SearchItem
	favSet            map[string]bool
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
	lyricSongPick       bool                // 歌级歌词绑定弹窗（ctxItem 为目标歌）

	quality         int            // 0=64k 1=128k 2=192k（对齐主线 d-quality 三档循环）
	lyricSize       int            // 0-4 歌词字号档（对齐主线 14/16/18/22/26px）
	themeMode       int            // 0 自动 1 明亮 2 暗黑（对齐主线 THEMES 三态）
	lyricBg         int            // 0 封面模糊 1 纯色（对齐主线 set-lybg）
	lyricTime       bool           // 歌词行显示时间戳（对齐主线 set-lytime）
	lyricCtrlBottom bool           // 歌词挂件控制条位置：true=下方 false=上方
	cacheLimit      int            // 歌曲缓存档位：0关 1=200MB 2=500MB 3=1GB 4=自定义
	cacheCustom     int            // 自定义歌曲缓存上限 MB
	lyricLimit      int            // 歌词缓存档位：0关 1=100 2=300 3=500 4=自定义
	lyricCustom     int            // 自定义歌词缓存上限 首
	historyScroll   ui.ScrollState // 搜索历史横滑状态（超出一行时 < > 按钮控制）
	recentScroll    ui.ScrollState // 最近播放横滑状态（< > 按钮控制）
	searchHistory   []string       // 搜索历史（对齐主线 清空历史）
	// 歌词（复用主线 lyric 域包；对齐 lyric-panel）
	lyricLines    []lyric.LyricLine
	lyricWords    [][]lyric.WordSpan // 逐字时间片（YRC 才有；与 lyricLines 一一对应）
	lyricCands    []lyric.LyricResult
	lyricSID      int
	lyricSrcName  string // 实际出词的歌词插件名（来源行显示）
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
			case "stats":
				a.statsPage(c)
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
	// 歌级歌词绑定弹窗
	if a.lyricSongPick && a.ctxItem != nil {
		a.lyricSongPickDialog(c)
	}
	// 新建歌单弹窗（不占布局空间）
	if a.songlistNewOpen {
		a.songlistNewDialog(c)
	}
	// 更新设置弹窗
	if updateDetail {
		a.updateDetailDialog(c)
	}
	// 启动恢复配置弹窗
	a.restoreConfigDialog(c)
	// 自定义上限弹窗
	a.customLimitDialog(c)
	// 音源扫码登录弹窗
	a.qrLoginDialog(c)
	// 登录方式弹窗（扫码 / Cookie）
	a.loginMethodDialog(c)
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
		// 上排：收藏 + 歌词开关 + 循环模式 + 静音 + 音量（对齐主线 dock-tools：全部右对齐）
		// 音质按钮已删（用户：高中低删掉）；♥ 和歌词移到本行（跟音量条同一行）
		ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(igap).Children(func() {
			ui.Spacer(c).Grow(1) // 右对齐（对齐主线工具行在进度条右上方）
			// 收藏（对齐主线 d-fav）
			if heartButton(c, a.isFav(), a.resp(24, 20), func() {}).Clicked() {
				a.toggleFav()
			}
			// 外部歌词开关（桌面=桌面挂件，安卓=悬浮歌词 Service）
			if lyricToggleButton(c, a.resp(24, 20), a.lyricOn()).Clicked() {
				a.toggleLyric()
			}
			if iconButton2(c, icRepeat, a.resp(24, 20), accent(a.loopMode > 0)).Clicked() {
				a.toggleLoop()
			}
			// 静音时换 🔇（带叉）图标 + 高亮，有声时用 🔊——一眼能看出静音态
			volIcon := icVol
			if a.muted || a.volume <= 0 {
				volIcon = icVolMute
			}
			if iconButton2(c, volIcon, a.resp(24, 20), accent(a.muted || a.volume <= 0)).Clicked() {
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
			// 收藏/歌词已移到上排（用户：♥ 和歌词跟音量条同一行）；下排只留播放控制
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
	// 日志系统最先初始化：捕获全部 log.Printf（含 db 打开失败）到内存环形缓冲+文件
	if dir, err := os.UserConfigDir(); err == nil {
		appLogDir := filepath.Join(dir, "mymusic-mygo")
		os.MkdirAll(appLogDir, 0o755)
		initAppLog(filepath.Join(appLogDir, "data.db"), logConfig{Capacity: 1000, MinLevel: logInfo, WriteFile: true})
	}
	a := &app{
		bili: bilibili.NewClient(
			"https://api.bilibili.com",
			"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
			"https://search.bilibili.com/", // 对齐原项目：www.bilibili.com 会触发风控返 HTML
			30*time.Second,
		),
		covers:          map[string]ui.ImageSource{},
		coverLoading:    map[string]bool{},
		blurCovers:      map[string]ui.ImageSource{},
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
	// 封面不落盘（只在内存 LRU 缓存 + InView 懒加载）——旧 coverDir 已移除。
	// ★必须先设 HC_PORT 再构造 svc：NewSpeakerService 构造时读 HC_PORT 拼音箱拉流 URL。
	// 之前 startStreamServer（下一行）才 set，导致 speaker 读到默认 28976，而供给端点监听
	// 28977 → 音箱拉 28976 无服务 → 无声。这里提前 set，使音箱拉流端口=供给端点端口。
	if os.Getenv("HC_PORT") == "" {
		os.Setenv("HC_PORT", "28977")
	}
	a.svc = newServices(a.db, a.bili, audioDir)
	// B站登录 Cookie（可选）：有则用自己的（登录态/高清），空=匿名（仅 buvid3）。
	a.bili.Cookie = plugin.GetCookie("bilibili")
	// 一切皆插件：注册内置插件（音源/歌词）。第三方插件 = 各自 init() 注册，主程序空导入。
	if a.svc != nil && a.svc.music != nil {
		plugin.RegisterSystemSource(source.NewBilibili(a.svc.music)) // B站音乐源（系统级）
		plugin.RegisterSettings("source", "bilibili", []plugin.Setting{{
			Key: "cookie", Name: "登录 Cookie", Type: plugin.SettingText, Default: "",
			Desc: "粘贴 B站登录态 Cookie（浏览器 www.bilibili.com 登录后复制），空=匿名（仅 buvid3）",
		}})
	}
	plugin.RegisterSystemLyric(lyricplug.NewNetease()) // 网易云歌词源（系统级兜底）
	// 网易云音源（Go 内置：明文 API + 扫码登录；JS 版同 key 自动跳过）
	a.neteaseSrc = source.NewNetease(plugin.GetCookie("netease"))
	plugin.RegisterSystemSource(a.neteaseSrc)
	plugin.RegisterSettings("source", "netease", []plugin.Setting{
		{Key: "quality", Name: "音质", Type: plugin.SettingSelect, Options: []string{"128k", "320k"}, Default: "320k", Desc: "播放音质"},
		{Key: "searchSize", Name: "搜索条数", Type: plugin.SettingSelect, Options: []string{"20", "40", "60"}, Default: "40", Desc: "每次搜索条数"},
		{Key: "cookie", Name: "登录 Cookie", Type: plugin.SettingText, Default: "", Desc: "扫码登录或粘贴网易云 Cookie；空=免登录"},
	})
	// 插件元数据（插件页展示）
	plugin.RegisterInfo(plugin.Info{Name: "bilibili", DisplayName: "B站", Capabilities: []string{"source"}, Icon: "bilibili", Version: "1.0", Desc: "B站音乐 · 搜索、播放、歌词", Order: 10})
	plugin.RegisterInfo(plugin.Info{Name: "netease", DisplayName: "网易云", Capabilities: []string{"source", "lyric"}, Icon: "netease", Version: "1.0", Desc: "网易云音乐 · 搜索、取词、歌词", Order: 20})
	// B站音源插件设置：搜索条数/分页自控（插件声明，系统统一存储 SettingStore）
	plugin.RegisterSettings("source", "bilibili", []plugin.Setting{
		{Key: "searchSize", Name: "搜索条数", Type: plugin.SettingSelect, Options: []string{"20", "40", "60"}, Default: "40", Desc: "每次搜索显示的条数"},
		{Key: "quality", Name: "音质", Type: plugin.SettingSelect, Options: []string{"64k", "128k", "192k"}, Default: "192k", Desc: "播放音质（64/128/192k）"},
	})
	// 本地文件源（扫目录听本机歌，免登录零版权、无网络）：目录从设置读（空=无搜索结果）。
	localDir := plugin.GetSetting("source", "local", "dir", "")
	plugin.RegisterSystemSource(source.NewLocal(localDir))
	plugin.RegisterSettings("source", "local", []plugin.Setting{
		{Key: "dir", Name: "本地音乐目录", Type: plugin.SettingText, Default: "", Desc: "本机歌曲目录（mp3/flac/m4a/ogg/wav）"},
	})
	// 本地音源与本机输出是两个独立插件：一个读文件系统路径，一个选系统输出声卡，不混用。
	plugin.RegisterInfo(plugin.Info{Name: "local", DisplayName: "本地音乐", Capabilities: []string{"source"}, Icon: "local", Version: "1.0", Desc: "本地文件 · 扫描目录播放", Order: 20})
	plugin.RegisterInfo(plugin.Info{Name: "localout", DisplayName: "本机声卡", Capabilities: []string{"target"}, Icon: "vol", Version: "1.0", Desc: "本机输出 · 选择系统输出声卡", Order: 30})
	plugin.RegisterInfo(plugin.Info{Name: "cast", DisplayName: "投屏", Capabilities: []string{"target"}, Icon: "cast", Version: "1.0", Desc: "DLNA 投屏目标（同网段发现播放器/电视）", Order: 40})
	plugin.RegisterInfo(plugin.Info{Name: "speaker", DisplayName: "小爱音箱", Capabilities: []string{"target"}, Icon: "speaker", Version: "1.0", Desc: "小爱音箱目标（米家扫码登录，投送当前歌）", Order: 60})
	// JS 沙箱插件：扫描可执行文件旁的 plugins/（相对路径会随 cwd 变化导致插件时多时少）。
	// 单个插件失败不拖垮启动（errs 收集，日志列出原因）。
	pluginDirs := []string{}
	if exe, err := os.Executable(); err == nil {
		pluginDirs = append(pluginDirs, filepath.Join(filepath.Dir(exe), "plugins"))
	}
	if dir, err := os.UserConfigDir(); err == nil {
		pluginDirs = append(pluginDirs, filepath.Join(dir, "mymusic-mygo", "plugins"))
	}
	pluginDirs = append(pluginDirs, "plugins")
	for _, pluginDir := range pluginDirs {
		if _, err := os.Stat(pluginDir); err != nil {
			continue
		}
		if errs, err := plugin.LoadFromDir(pluginDir); err != nil {
			log.Printf("js plugin dir: %v", err)
		} else {
			for _, e := range errs {
				log.Printf("js plugin load: %v", e)
			}
		}
		break // 找到第一个存在的目录就停，避免重复注册
	}
	// 恢复用户自定义插件顺序（影响 SourceNames/插件页/搜索下拉框）——必须在
	// SourceNames() 首次调用前，否则默认源取的还是未排序的第一个。
	a.restorePluginOrder()
	a.restorePluginEnabled()
	a.restoreLogConfig()
	a.restoreCfg = a.appLoadRestoreConfig()
	// 当前源 = 排序后首个（默认 B站）；配置开启时优先用上次搜索音源
	if names := plugin.SourceNames(); len(names) > 0 {
		a.src = plugin.Sources()[names[0]]
		if a.restoreCfg.LastSource {
			if last := a.loadPrefStr("mygo_lastsource", ""); last != "" {
				if s := plugin.Sources()[last]; s != nil {
					a.src = s
					a.activeSource = last
				}
				a.lastSavedSource = last
			}
		}
	}
	if a.restoreCfg.LastTab {
		if tab := a.loadPrefStr("mygo_lasttab", ""); tab != "" {
			a.activeTab = tab
		}
	}
	a.startStreamServer()    // 音箱供给端点（/api/v1/music/stream，transcode mp3 秒开 + seek）
	a.listenWidgetCommands() // 浮窗控制命令（toggle/prev/next）→ mygo 播放器
	a.favSet = map[string]bool{}
	a.refreshFavs()      // 收藏恢复自 fav 域包（SQLite 持久化）
	a.refreshSonglists() // 歌单列表恢复自 songlist 域包（SQLite 持久化）
	a.restorePrefs()     // 恢复主题/音质/缓存上限（settings 表）
	// 安卓悬浮歌词：恢复上次开启状态（进程重启后 Service 不自动恢复）。
	// 必须在 App.WhenReady 里调：mygo.Native().Call 在 App.Run 之前会直接 panic
	// （实测一启动就 SIGABRT：NativePlugin.Call called before App.Run）。
	if runtime.GOOS == "android" && a.androidLyricOn {
		mygo.App.WhenReady(func() { _, _ = widgetoverlay.Start() })
	}
	if a.restoreCfg.Recent {
		a.restoreRecent()
	}
	if a.restoreCfg.History {
		a.restoreHistory()
	}
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
	plugin.RegisterSystemTarget(target.NewLocal(a.mygoOut))
	a.mygoOut.cache = a.svc.cache // m4s 源缓存（与音箱/供给端点共用）
	if a.svc != nil && a.svc.xcoder != nil {
		a.mygoOut.setXcoder(a.svc.xcoder) // mp3 转码缓存（本地播放也共用）
	}
	// 1.2.5 统一取流供给：本机播放/HTTP 端点/DLNA 共用一份（音源表按函数注入）。
	a.supplier = &Supplier{
		sources: func() map[string]source.MusicSource { return plugin.Sources() },
		cache:   a.svc.cache,
		xcoder:  a.svc.xcoder,
		client:  a.bili,
	}
	a.mygoOut.setSupplier(a.supplier)
	// B站音质设置（插件设置）：64k/128k/192k → 音质档 0/1/2（默认 192k）。
	switch plugin.GetSetting("source", "bilibili", "quality", "192k") {
	case "64k":
		a.quality = 0
	case "128k":
		a.quality = 1
	default:
		a.quality = 2
	}
	a.mygoOut.qualFn = func() int { return a.quality } // 音质档实时读取（60k/128k/192k）
	a.mygoOut.onErr = func(msg string) {               // 取流失败 UI 提示（付费/VIP/需登录）
		a.win.Update(func() { a.status = msg })
	}
	initVol := a.volume
	if a.muted {
		initVol = 0 // 静音恢复：输出静音但 mygo_volume 保持原值（音量条不归零）
	}
	a.mygoOut.SetVolume(int(initVol * 100))
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
			// 音箱播完也算一次播放（与本机 ended 同口径）
			mygo.RunOnMain(func() {
				if cur := a.current; cur != nil {
					src := cur.src
					if src == "" {
						src = "bilibili"
					}
					a.statsPlay(src, cur.bvid, cur.title, cur.author, cur.pic)
				}
			})
			if nextBVID == "" {
				a.pb.SpeakerFinished(did)
				return
			}
			a.pb.Next()
		}
	}
	// 播完自动连播（对齐主线 audio ended：single 前端 currentTime=0 重播当前歌，
	// 其余 cmd('next') 后端顺序环绕）。single 用 seek(0)+Resume 重播（MyGo 本地 audio）。
	// 播完统计：只在真正播完（audio ended）时计数 +1，切歌/暂停不算。
	a.mygoOut.SetFinishedHook(func() {
		mygo.RunOnMain(func() {
			cur := a.current
			if cur == nil {
				return
			}
			src := cur.src
			if src == "" {
				src = "bilibili"
			}
			a.statsPlay(src, cur.bvid, cur.title, cur.author, cur.pic)
		})
	})
	a.mygoOut.SetEndedHook(func() {
		if a.loopMode == 2 { // 单曲循环：重播当前歌
			a.seek(0)
			_ = a.mygoOut.Resume()
			return
		}
		a.pb.Next()
	})

	mygo.App.WhenReady(func() {
		// 恢复上次的窗口位置/大小（settings 表 mygo_windowstate）；无记录用默认
		var ws *winState
		if a.restoreCfg.Window {
			ws = a.restoreWindowState()
		}
		wx, wy, ww, wh := 0, 0, 980, 680
		if ws != nil {
			wx, wy, ww, wh = ws.X, ws.Y, ws.W, ws.H
		}
		a.win = mygo.NewWindow(mygo.WindowOptions{
			Title:     "MyMusic",
			X:         wx,
			Y:         wy,
			Width:     ww,
			Height:    wh,
			MinWidth:  int(500 * uiScale),
			MinHeight: int(480 * uiScale),
			Content:   ui.View(a.view),
		})
		// 🎵 应用图标：Linux/Windows 标题栏 + 任务栏（不给就是裸 X11 默认图标）
		applyAppIcon(a.win)
		// 登录态校验（异步，B站/网易云有可靠端点；有 Cookie 才查，不阻塞启动）
		a.verifyAllLogins()
		// 恢复最大化状态（WindowOptions 没有该字段，只能就绪后调 Maximize）
		if ws != nil && ws.Max {
			a.win.Maximize()
		}
		// 分辨率适配：窗口尺寸变化时更新宽度感知（断点切换 compact），仅重绘不重建
		b := a.win.Bounds()
		a.winW, a.winH = b.Width, b.Height
		a.win.OnResize(func() {
			if nb := a.win.Bounds(); nb.Width != a.winW || nb.Height != a.winH {
				a.winW, a.winH = nb.Width, nb.Height
				log.Printf("[resize] %dx%d tier=%d(%s)", a.winW, a.winH, a.tier(), map[int]string{0: "narrow", 1: "compact", 2: "normal"}[a.tier()])
				a.win.Invalidate()
			}
			a.saveWindowStateThrottled(a.win) // 记住窗口大小（节流）
		})
		// 窗口移动 + 关闭：记住位置（节流）/ 退出时无条件落盘
		a.win.OnMove(func() { a.saveWindowStateThrottled(a.win) })
		a.win.OnClose(func(e *mygo.CloseEvent) { a.saveWindowState(a.win) })
		// 恢复队列（window 就绪后再恢复，自动播放的进度上报不会撞未创建窗口）
		a.pb.RestoreWithOptions(a.db, a.restoreCfg.Position, a.restoreCfg.AutoPlay)
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

// dialogRect 居中弹窗的响应式矩形：窄屏左右留 16px、高度不超屏幕 90%（手机不溢出），
// 宽屏用设计尺寸。返回 lx, ly, w, h。所有居中 Overlay 弹窗统一用它。
func (a *app) dialogRect(dw, dh float32) (float32, float32, float32, float32) {
	pw, ph := dw, dh
	if a.winW > 0 {
		if maxW := float32(a.winW) - 32*uiScale; pw > maxW {
			pw = maxW
		}
	}
	if a.winH > 0 {
		if maxH := float32(a.winH) * 0.9; ph > maxH {
			ph = maxH
		}
	}
	lx, ly := float32(a.winW)/2-pw/2, float32(a.winH)/2-ph/2
	if a.winW <= 0 {
		lx = 100
	}
	if a.winH <= 0 {
		ly = 100
	}
	if lx < 16*uiScale {
		lx = 16 * uiScale
	}
	if ly < 16*uiScale {
		ly = 16 * uiScale
	}
	return lx, ly, pw, ph
}
