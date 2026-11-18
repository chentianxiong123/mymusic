//go:build native

package main

import (
	"io"
	"log"
	"mymusic-mygo/source"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo/audio"

	"homecast/service/cache"
	"homecast/service/music"
	"homecast/service/playback"
	"homecast/service/playlist"

	"homecast/bilibili"
)

// MyGoOutput 实现主线 playback.Output：MyGo 本机出声后端。
// 区域：只管"怎么真正出声 + 上报进度"（audio.Player），不持有业务状态——
// 状态/队列/模式/持久化全在主线 playback 域包（SSOT）。
//
// 语义对齐主线"web 前端 audio"：MyGo 是本机 audio 出声（等价 HTMLAudio），
// source=web，进度由 MyGo 上报（等价 timeupdate）。与 web 不同的是 MyGo 直接
// 在这里取流+解码出声（不走浏览器），这是"原生前端"的天然接缝。
type MyGoOutput struct {
	bili    *bilibili.Client
	music   *music.MusicService // 主线音乐域（GetAudioStream：view 缓存 + 取流，取流自创的替代）
	src     source.MusicSource  // 音源接口（阶段 2 解耦：取流走接口，不认具体源）
	cache   *cache.CacheService // 主线缓存域包（可空：命中→秒开）
	qualFn  func() int          // 当前音质档（0=64k 1=128k 2=192k，默认 192k）
	qualMu  sync.Mutex
	quality int // Locked by qualMu：默认 192k

	mu    sync.Mutex
	p     *audio.Player
	path  string // 当前临时音频文件（Stop/切歌时清理）
	bvid  string
	title string
	gen   int // 切歌代际：切歌后旧 goroutine 的下载放弃（防竞态）
	ended func()
}

func newMyGoOutput(bili *bilibili.Client) *MyGoOutput {
	return &MyGoOutput{bili: bili}
}

// setMusic 注入主线音乐域（main 初始化 svc 后调用；取流走 GetAudioStream）。
func (o *MyGoOutput) setMusic(m *music.MusicService) {
	o.music = m
}

// setSource 注入音源接口（阶段 2 解耦：取流走 MusicSource）。
func (o *MyGoOutput) setSource(s source.MusicSource) {
	o.src = s
}

// Play 异步：立即返回（UI 不卡），后台取流→下载→解码→出声。
// gen 递增使旧下载任务作废（快速切歌时 A 下载中切 B，A 完成后自动丢弃）。
func (o *MyGoOutput) Play(bvid string, opts playback.PlayOpts) error {
	o.mu.Lock()
	o.gen++
	gen := o.gen
	o.mu.Unlock()
	go o.loadAndPlay(bvid, opts, gen)
	return nil
}

// qualFnQuality 当前音质档（0/1/2，MusicSource.GetStream 语义）。
func (o *MyGoOutput) qualFnQuality() int {
	if o.qualFn == nil {
		return 2
	}
	return o.qualFn()
}

func (o *MyGoOutput) loadAndPlay(bvid string, opts playback.PlayOpts, gen int) {
	// 音质档：0=64k 1=128k 2=192k（默认）
	qn := bilibili.PreferredQuality
	if o.qualFn != nil {
		switch o.qualFn() {
		case 0:
			qn = bilibili.Audio64K
		case 1:
			qn = bilibili.Audio128K
		default:
			qn = bilibili.Audio192K
		}
	}
	// 取流走音源接口（阶段 2 解耦）：MusicSource.GetStream 内部映射音质档 → B站 qn。
	// 未来换网易云/本地源 = 换 src 实现，这里零改动。回退：无 src 用主线 music/bili。
	var stream *bilibili.AudioStreamResult
	var err error
	if o.src != nil {
		st, serr := o.src.GetStream(bvid, o.qualFnQuality())
		if serr != nil {
			log.Printf("[mygo-output] 取流失败 %s: %v", bvid, serr)
			return
		}
		// 接口返回 URL（B站 DASH baseUrl）；包装成主线 AudioStreamResult 供下载复用
		stream = &bilibili.AudioStreamResult{URL: st.URL, Quality: qn}
	} else if o.music != nil {
		stream, err = o.music.GetAudioStream(bvid, qn)
	} else {
		info, ierr := o.bili.GetVideoInfo(bvid)
		if ierr != nil {
			log.Printf("[mygo-output] 取视频信息失败 %s: %v", bvid, ierr)
			return
		}
		stream, err = o.bili.GetBestAudioURL(bvid, info.CID, qn)
	}
	if err != nil {
		log.Printf("[mygo-output] 取流失败 %s: %v", bvid, err)
		return
	}

	// 缓存命中（对齐主线 cache 域包：再次播放秒开）
	if o.cache != nil {
		if _, _, ok := o.cache.Get(bvid); ok {
			path := o.cache.FilePath(bvid)
			if fi, err := os.Stat(path); err == nil && fi.Size() > 0 {
				o.openAndPlay(bvid, opts, gen, path)
				return
			}
		}
	}
	// 下载到临时文件（B站 CDN 多节点，拒连重试）
	f, err := os.CreateTemp("", "mymusic-mygo-*.m4a")
	if err != nil {
		log.Printf("[mygo-output] 临时文件失败: %v", err)
		return
	}
	path := f.Name()
	var resp *http.Response
	for attempt := 0; attempt < 5; attempt++ {
		resp, err = o.bili.GetRaw(stream.URL, nil)
		if err == nil {
			break
		}
		time.Sleep(600 * time.Millisecond)
	}
	if err != nil {
		f.Close()
		os.Remove(path)
		log.Printf("[mygo-output] 下载失败 %s: %v", bvid, err)
		return
	}
	_, err = io.Copy(f, resp.Body)
	resp.Body.Close()
	f.Close()
	if err != nil {
		os.Remove(path)
		log.Printf("[mygo-output] 下载失败 %s: %v", bvid, err)
		return
	}
	// 回写缓存域包（SQLite 元数据 + 磁盘）；失败不影响播放
	if o.cache != nil {
		if data, rerr := os.ReadFile(path); rerr == nil {
			_ = o.cache.Put(bvid, bilibili.PreferredQuality, data)
		}
	}

	o.openAndPlay(bvid, opts, gen, path)
}

// openAndPlay 打开音频文件并提交到 audio.Player（缓存命中/下载后共用）。
func (o *MyGoOutput) openAndPlay(bvid string, opts playback.PlayOpts, gen int, path string) {
	// 解码
	r, err := os.Open(path)
	if err != nil {
		os.Remove(path)
		log.Printf("[mygo-output] 打开音频失败: %v", err)
		return
	}
	p, err := audio.OpenStream(r)
	if err != nil {
		r.Close()
		os.Remove(path)
		log.Printf("[mygo-output] 解码失败 %s: %v", bvid, err)
		return
	}

	// 提交（若期间已切歌 gen 变了 → 放弃这次，清掉）
	o.mu.Lock()
	if o.gen != gen {
		o.mu.Unlock()
		p.Stop()
		os.Remove(path)
		return
	}
	old := o.p
	oldPath := o.path
	o.p = p
	o.path = path
	o.bvid = bvid
	o.title = opts.Title
	hook := o.ended
	o.mu.Unlock()
	if old != nil {
		old.Stop()
	}
	if oldPath != "" && !strings.HasSuffix(oldPath, ".m4s") {
		os.Remove(oldPath)
	}
	// 播完回调核心（自动连播：核心填 Next）
	if hook != nil {
		p.OnEnd(func() { hook() })
	}
	p.Play()
}

func (o *MyGoOutput) Pause() error {
	o.mu.Lock()
	p := o.p
	o.mu.Unlock()
	if p != nil {
		p.Pause()
	}
	return nil
}

// SetPending 记住「当前该播的 bvid」（不立即加载音源）。
// 启动恢复场景：pb restore 把队列/当前歌写回 cur，但输出端从未 Play 过、
// o.p 与 o.bvid 都是空 → 用户点播放时 Resume 无音源可恢复（旧的 bug）。
// applyState 每次把 cur.BVID 同步过来，Resume 就能在 p==nil 时自行重新加载。
// 仅当当前未加载任何音源时才写入（避免覆盖正在播/下载中的 bvid）。
func (o *MyGoOutput) SetPending(bvid, title string) {
	if bvid == "" {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.bvid == "" && o.p == nil {
		o.bvid = bvid
		o.title = title
	}
}

// Resume 恢复播放。
// 关键：启动恢复（pb restore）后队列里有歌、Playing=false，但输出端从未 Play 过
// → o.p == nil。旧实现 `if p != nil { p.Play() }` 在 p==nil 时直接空转，
// 表现为「刚进去点播放没反应，必须去队列/搜索重新点歌」。
// 修：p==nil 时说明音源未加载，用记住的 bvid 重新 Play（等价用户点歌）。
// 注意 Play 内部会起后台下载并递增 gen，不会与旧任务竞争。
func (o *MyGoOutput) Resume() error {
	o.mu.Lock()
	p := o.p
	bvid := o.bvid
	title := o.title
	o.mu.Unlock()
	if p != nil {
		p.Play()
		return nil
	}
	if bvid == "" {
		return nil // 真无歌可播
	}
	// 音源未加载（启动恢复/上次 Stop 后）：重新取流→下载→播放
	return o.Play(bvid, playback.PlayOpts{Title: title})
}

func (o *MyGoOutput) Stop() error {
	o.mu.Lock()
	p := o.p
	path := o.path
	o.p = nil
	o.path = ""
	o.bvid = ""
	o.mu.Unlock()
	if p != nil {
		p.Stop()
	}
	if path != "" {
		os.Remove(path)
	}
	return nil
}

func (o *MyGoOutput) Seek(sec float64) error {
	o.mu.Lock()
	p := o.p
	o.mu.Unlock()
	if p != nil {
		p.Seek(time.Duration(sec * float64(time.Second)))
	}
	return nil
}

func (o *MyGoOutput) SetVolume(v int) error {
	o.mu.Lock()
	p := o.p
	o.mu.Unlock()
	if p != nil {
		p.SetVolume(float64(v) / 100.0)
	}
	return nil
}

// Position 上报当前进度（真实，audio 自报）。BVID/Title 带当前实际在播（对齐核心）。
func (o *MyGoOutput) Position() playback.ProgressInfo {
	o.mu.Lock()
	p := o.p
	bvid := o.bvid
	title := o.title
	o.mu.Unlock()
	if p == nil {
		return playback.ProgressInfo{}
	}
	return playback.ProgressInfo{
		BVID:    bvid,
		Title:   title,
		Pos:     p.Position().Seconds(),
		Dur:     p.Duration().Seconds(),
		Playing: p.Playing(),
	}
}

func (o *MyGoOutput) Caps() playback.Capabilities {
	return playback.Capabilities{Play: true, Pause: true, Resume: true, Stop: true, Seek: true, Volume: true, Progress: true}
}

func (o *MyGoOutput) SetEndedHook(fn func()) {
	o.mu.Lock()
	o.ended = fn
	o.mu.Unlock()
}

func (o *MyGoOutput) Name() string { return "mygo" }

// fmtDurMygo 秒→"m:ss"（UI 用；与主线 fmtDur 对齐）
// fmtDurMygo 秒 → m:ss。复用主线 playlist.FormatDuration（同公式），sec<=0 给 0:00。
func fmtDurMygo(sec float64) string {
	if sec <= 0 {
		return "0:00"
	}
	return playlist.FormatDuration(int(sec))
}
