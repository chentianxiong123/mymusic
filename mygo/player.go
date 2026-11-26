//go:build native

package main

import (
	"bytes"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	_ "golang.org/x/image/webp" // WebP 解码（x/image，image.Decode 自动识别）
	"io"
	"mymusic-mygo/plugin"
	"mymusic-mygo/source"
	"net/http"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"homecast/service/playback"

	"homecast/bilibili"
)

// search 在 goroutine 里搜，结果经 win.Update 回到主线程。
// srcResult 一个音源的一次搜索结果（保持注册顺序，UI 按源分组渲染）。
// songs 为音源原始结果，items 为转换后的 UI 行。
type srcResult struct {
	name  string
	songs []source.Song
	total int
}

func (a *app) search() {
	a.loading.Store(true)
	var kw string
	mygo.RunOnMain(func() { kw = a.query; a.status = "搜索中…" })
	// 一次只搜一个源（搜索栏下拉框选中的）：不再并发搜 10 个源拉一堆结果+封面卡死。
	// 每次搜索只发 1 个请求、只显示 1 个源的结果；分页仍是该源独立分页。
	name := a.currentSource()
	mygo.RunOnMain(func() { a.lastSearchSource = name })
	src := plugin.Sources()[name]
	var perSrc []srcResult
	if src != nil {
		page := a.sourcePage(name)
		size := srcSearchSize(name)
		songs, total, err := src.Search(kw, page, size)
		if err == nil {
			perSrc = append(perSrc, srcResult{name: name, songs: songs, total: total})
		} else {
			perSrc = append(perSrc, srcResult{name: name})
		}
	}
	a.loading.Store(false)
	// 搜索历史（对齐主线 search-history.js MAX=8：去重置顶，最多 8 条）
	mygo.RunOnMain(func() {
		hist := make([]string, 0, len(a.searchHistory)+1)
		hist = append(hist, kw)
		for _, h := range a.searchHistory {
			if h != kw {
				hist = append(hist, h)
			}
		}
		if len(hist) > 8 {
			hist = hist[:8]
		}
		a.searchHistory = hist
		a.saveHistory()
	})
	a.win.Update(func() {
		// 每源独立结果 → 按源分组存 resultsBySource（UI 分区展示，插件分组）；
		// 同时拍平存 a.results（兼容 playAll/总数/清空逻辑）。
		a.results = a.results[:0]
		a.resultsBySource = a.resultsBySource[:0]
		var total int
		for _, r := range perSrc {
			grp := srcResult{name: r.name, songs: r.songs, total: r.total}
			for _, s := range r.songs {
				a.results = append(a.results, bilibili.SearchItem{
					BVID: s.ID, Title: s.Title, Author: s.Artist, Pic: s.Cover,
					Duration: s.Duration, Typename: s.Typename, Play: 0, Src: r.name,
				})
			}
			// 跳过无结果的源（不渲染空分区，但总数统计保留）
			if len(r.songs) > 0 {
				a.resultsBySource = append(a.resultsBySource, grp)
			}
			total += r.total
		}
		a.searchTotal = total
		a.searched = true
		// 换页清理：保留当前播放曲目的封面，其余驱逐（防止跨页堆积）
		keep := ""
		if a.current != nil {
			keep = a.current.bvid
		}
		for b := range a.covers {
			if b != keep {
				delete(a.covers, b)
				delete(a.coverLoading, b)
			}
		}
		a.coverOrder = a.coverOrder[:0]
		if keep != "" {
			a.coverOrder = append(a.coverOrder, keep)
		}
		a.status = fmt.Sprintf("找到 %d 条", len(a.results))
	})
}

// play 播放一首：交给主线 playback（SSOT）入队+定位+出声（MyGoOutput 取流）。
// 状态/队列/进度全由 pbLoop 从状态流渲染，这里只发命令。
func (a *app) play(item bilibili.SearchItem) {
	a.recordRecent(item)
	// 播放统计在 audio 播完回调里 +1（放完才算一次；切歌/暂停不算），不在这里记
	// 分开搜索：记录歌来源（Typename），MyGoOutput 取流按来源路由到对应源
	a.mygoOut.setSrcName(item.Typename)
	a.win.Update(func() {
		a.loading.Store(false)
		a.status = "播放: " + bilibili.CleanHTML(item.Title)
	})
	a.pb.Play(toSong(item))
}

// recordRecent 记录最近播放（对齐主线 hc:recent：去重置顶，最多 30）。
func (a *app) recordRecent(item bilibili.SearchItem) {
	mygo.RunOnMain(func() {
		out := make([]bilibili.SearchItem, 0, len(a.recent)+1)
		out = append(out, bilibili.SearchItem{
			BVID: item.BVID, Title: bilibili.CleanHTML(item.Title),
			Author: item.Author, Pic: item.Pic, Duration: item.Duration,
		})
		for _, r := range a.recent {
			if r.BVID != item.BVID {
				out = append(out, r)
			}
		}
		if len(out) > 30 {
			out = out[:30]
		}
		a.recent = out
		a.saveRecent()
		a.win.Invalidate()
	})
}

// indexOfBVID 返回 bvid 在列表中的位置，未找到返回 -1。
func indexOfBVID(list []bilibili.SearchItem, bvid string) int {
	for i, it := range list {
		if it.BVID == bvid {
			return i
		}
	}
	return -1
}

// playRow 列表行点歌：有列表上下文（playFn）时按全局 pointPlayMode 决定替换/追加；
// 无上下文（推荐/榜单/最近等单曲）保持 a.play 的追加/定位语义。
// pointPlayMode=0（默认，对齐网易云）：用当前列表替换播放队列并从点的那首开始；
// pointPlayMode=1：仅把单曲追加到队尾，不打断当前播放。
func (a *app) playRow(item bilibili.SearchItem, playFn func()) {
	if playFn != nil && a.pointPlayMode == 0 {
		playFn()
		return
	}
	go a.play(item)
}

// playFrom 带列表上下文点歌（SearchItem 版）：按 pointPlayMode 走替换或追加。
func (a *app) playFrom(list []bilibili.SearchItem, idx int) {
	if idx < 0 || idx >= len(list) {
		return // 调用方应先定位；找不到时不应点歌（用 play(item) 走单曲兜底）
	}
	a.playSongs(toSongList(list), idx)
}

// playSourceFrom 带列表上下文点歌（source.Song 版，推荐/榜单/统计用）。
// srcName 是这些歌所属音源 key（source.Song 自身不带来源字段）。
func (a *app) playSourceFrom(list []source.Song, idx int, srcName string) {
	songs := make([]playback.Song, 0, len(list))
	for _, s := range list {
		songs = append(songs, playback.Song{
			BVID: s.ID, Title: s.Title, Artist: s.Artist, Cover: s.Cover,
			Duration: float64(s.DurationSec), Source: srcName, ID: s.ID,
		})
	}
	a.playSongs(songs, idx)
}

// playSongs 所有点歌入口的唯一语义实现：
// 替换模式 → 整队 SetQueue(list, idx)；追加模式 → 只把这一首 Play 加到队尾。
// 一切列表（搜索/歌单/收藏/队列/最近/推荐/榜单/统计）最终都走这里，
// 因为它们本质都是队列的操作入口，没有例外。
func (a *app) playSongs(list []playback.Song, idx int) {
	if idx < 0 || idx >= len(list) {
		return
	}
	if a.pointPlayMode == 0 {
		a.win.Update(func() { a.loading.Store(false) })
		a.pb.SetQueue(list, idx)
		return
	}
	go a.play(toSearchItems(list[idx:idx+1])[0])
}

// playAll 播放整个队列（搜索结果/收藏夹）：SetQueue 整队+定位。
func (a *app) playAll(list []bilibili.SearchItem, startIdx int) {
	songs := make([]playback.Song, 0, len(list))
	for _, it := range list {
		songs = append(songs, toSong(it))
	}
	a.win.Update(func() { a.loading.Store(false) })
	a.pb.SetQueue(songs, startIdx)
}

// togglePlay 播放/暂停（走主线 SSOT，统一调度）。
func (a *app) togglePlay() {
	a.pb.Toggle()
}

// coverLimit 封面缓存数量上限（LRU 驱逐）。
const coverLimit = 60

// coverThumbMax 封面缩略图长边上限（px）。显示最多 ~64px，2.5x 保证高分屏清晰。
const coverThumbMax = 160

// blurCoverMax 模糊版降采样长边（px）：小图放大铺满=背景模糊（低成本，无高斯卷积）。
const blurCoverMax = 12

// loadCover 异步下载并缓存封面图（B站图床需 UA/referer）。
// 为避免内存爆炸，缓存的是缩略图（128~160px），不是原图：
// 原图 1280x720 RGBA ≈ 3.7MB/张，20 张列表 ≈ 240MB；缩略 160x160 ≈ 100KB/张，60 张 ≈ 6MB。
// 同时按 LRU 驱逐超限封面，换页时清掉非当前曲目的缓存。
func (a *app) loadCover(bvid, url string) {
	if url == "" {
		return
	}
	// B站图床走 https；搜索返回的是协议相对 URL（//i*.hdslb.com/...）或 http
	if len(url) > 2 && url[:2] == "//" {
		url = "https:" + url
	} else if len(url) > 7 && url[:7] == "http://" {
		url = "https://" + url[7:]
	}
	if _, ok := a.covers[bvid]; ok {
		return
	}
	if a.coverLoading[bvid] {
		return
	}
	// 封面不落盘：只在内存缓存（LRU 60 张）。搜索结果封面懒加载——
	// 列表项滚到可见区才调 loadCover（见 search_view.go 的 cover 懒加载）。
	// 旧版落盘 228 张/75MB 且同步解码原图 → 搜索卡死（pprof 77% JPEG 解码），已废除。
	a.coverLoading[bvid] = true
	go func() {
		defer func() { a.win.Update(func() { a.coverLoading[bvid] = false }) }()
		req, _ := http.NewRequest(http.MethodGet, url, nil)
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/120.0 Safari/537.36")
		req.Header.Set("Referer", "https://www.bilibili.com")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return
		}
		defer resp.Body.Close()
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			return
		}
		// 不落盘：解码后只在内存缓存（LRU evictCovers 管理）。
		a.decodeCover(bvid, data)
	}()
}

// decodeCover 解码 + 缩略 + 模糊版 + 缓存（磁盘命中/下载后共用；bitmap 在 goroutine 建，主线程存 map）。
func (a *app) decodeCover(bvid string, data []byte) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		// 兜底：SVG 等非位图交给 ui 自己处理（不做缩略，量小）
		s, serr := ui.DecodeImage(data)
		if serr != nil {
			return
		}
		a.win.Update(func() { a.covers[bvid] = s })
		return
	}
	bmp := ui.NewBitmap(source.ScaleThumb(img, coverThumbMax))
	blur := ui.NewBitmap(source.ScaleThumb(img, blurCoverMax)) // 降采样模糊版（放大铺满=背景模糊）
	a.win.Update(func() {
		a.covers[bvid] = bmp
		a.blurCovers[bvid] = blur
		a.coverOrder = append(a.coverOrder, bvid)
		a.evictCovers()
		a.win.Invalidate()
	})
}

// evictCovers LRU 驱逐：缓存超限时删最旧，保留当前播放曲目封面。
func (a *app) evictCovers() {
	keep := ""
	if a.current != nil {
		keep = a.current.bvid
	}
	for len(a.coverOrder) > coverLimit {
		oldest := a.coverOrder[0]
		a.coverOrder = a.coverOrder[1:]
		if oldest == keep {
			continue
		}
		delete(a.covers, oldest)
		delete(a.coverLoading, oldest)
	}
}

// demo MYGO_DEMO=1 自检：搜索→播放→开挂件（试水验证用，临时）。
// win.Update 是异步的；同步读状态要用 mygo.RunOnMain。
func (a *app) demo() {
	time.Sleep(800 * time.Millisecond)
	mygo.RunOnMain(func() { a.query = "周杰伦" })
	a.search()
	time.Sleep(2200 * time.Millisecond)
	var first bilibili.SearchItem
	mygo.RunOnMain(func() {
		if len(a.results) > 0 {
			first = a.results[0]
		}
	})
	if first.BVID != "" {
		a.play(first)
	}
	time.Sleep(2600 * time.Millisecond)
	mygo.RunOnMain(func() { a.toggleLyric() })
	time.Sleep(400 * time.Millisecond)
	mygo.RunOnMain(func() { a.win.Invalidate() })
}

// handleShortcuts 键盘快捷键（对齐主线 shortcuts.js：空格播放暂停、
// ←/→ 切歌、M 静音；输入框聚焦时全部忽略）。
func (a *app) handleShortcuts(c *ui.Context) {
	if c.FocusTakesText() {
		return // 正在输入文字，不抢键
	}
	if c.Shortcut(0, ui.KeySpace) {
		if a.current != nil {
			a.pb.Toggle()
		}
		return
	}
	if c.Shortcut(0, ui.KeyLeft) {
		a.pb.Prev()
		return
	}
	if c.Shortcut(0, ui.KeyRight) {
		a.pb.Next()
		return
	}
	if c.Shortcut(0, ui.KeyM) {
		a.toggleMute()
		return
	}
}
