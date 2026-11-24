//go:build native

package main

import (
	"fmt"
	"io"
	"log"
	"mymusic-mygo/plugin"
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
	"homecast/service/transcode"

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
	curSrc  string              // 当前播放歌的来源名（分开搜索：歌可能来自网易云/插件源）
	cache   *cache.CacheService // 主线缓存域包（可空：命中→秒开）
	xcoder  *transcode.Transcoder // 主线 mp3 转码缓存（与音箱/供给端点共用，本地播放也走它）
	qualFn  func() int          // 当前音质档（0=64k 1=128k 2=192k，默认 192k）
	qualMu  sync.Mutex
	quality int // Locked by qualMu：默认 192k

	mu    sync.Mutex
	p     *audio.Player
	path  string // 当前播放文件路径（共享缓存/本地源/临时文件）
	temp  bool   // path 是否自建临时文件（仅此类切歌时删除；共享缓存永不删）
	bvid  string
	title string
	gen   int // 切歌代际：切歌后旧 goroutine 的下载放弃（防竞态）
	ended func()
	onErr  func(string) // 取流/播放错误回调 → UI 提示（付费/VIP/需登录等，main 注入）
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

// setXcoder 注入主线 mp3 转码缓存（本地播放与音箱共用同一份基础设施）。
func (o *MyGoOutput) setXcoder(x *transcode.Transcoder) {
	o.xcoder = x
}

// setSrcName 设置当前播放歌的来源名（分开搜索：歌可能来自网易云/插件源）。
// play() 时 app 传入 item.Typename；空=当前默认源。
func (o *MyGoOutput) setSrcName(name string) {
	o.mu.Lock()
	o.curSrc = name
	o.mu.Unlock()
}

// srcName 当前播放歌来源名（加锁读；空=默认源）。
func (o *MyGoOutput) srcName() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.curSrc
}

// sourceByName 按歌来源名路由到对应音源（分开搜索：每源独立，取流用对应源）。
// name=歌的 Typename（"bilibili"/"netease"/...）；空或未知回退当前 src。
func (o *MyGoOutput) sourceByName(name string) source.MusicSource {
	if name != "" {
		if s := plugin.Sources()[name]; s != nil {
			return s
		}
	}
	return o.src
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
		o.mu.Lock()
		srcName := o.curSrc
		o.mu.Unlock()
		st, serr := o.sourceByName(srcName).GetStream(bvid, o.qualFnQuality())
		if serr != nil {
			log.Printf("[mygo-output] 取流失败 %s: %v", bvid, serr)
			if o.onErr != nil {
				o.onErr(serr.Error()) // UI 提示（VIP/付费/需登录等）
			}
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
			if o.onErr != nil {
				o.onErr("取流失败：" + ierr.Error())
			}
			return
		}
		stream, err = o.bili.GetBestAudioURL(bvid, info.CID, qn)
	}
	if err != nil {
		log.Printf("[mygo-output] 取流失败 %s: %v", bvid, err)
		if o.onErr != nil {
			o.onErr("取流失败：" + err.Error())
		}
		return
	}

	// 本地文件源（file://）：已是 mp3/flac/m4a 等可解码格式，无需转码/缓存，直接解码。
	if strings.HasPrefix(stream.URL, "file://") {
		o.openAndPlay(bvid, opts, gen, strings.TrimPrefix(stream.URL, "file://"), false)
		return
	}

	// 共用基础设施：本地播放与音箱/供给端点共用同一套 m4s 源缓存 + mp3 转码缓存。
	// 不再自建临时文件 / 重复下载 / 重复回写缓存。
	// 音质统一用车厘子 qn 值（30280/30232/30216），与主线 cache/xcoder 键一致。
	if o.xcoder != nil {
		if _, ok := o.xcoder.Get(bvid, qn); ok {
			path := o.xcoder.FilePath(bvid, qn)
			if fi, statErr := os.Stat(path); statErr == nil && fi.Size() > 0 {
				o.openAndPlay(bvid, opts, gen, path, false) // mp3 缓存命中 → 秒开
				return
			}
		}
	}

	// 确保源 m4s 落主线 cache（唯一源缓存；未命中才下载一次）。
	m4sPath, err := o.ensureM4S(bvid, qn, stream.URL)
	if err != nil {
		log.Printf("[mygo-output] 取源失败 %s: %v", bvid, err)
		if o.onErr != nil {
			o.onErr("下载失败：" + err.Error())
		}
		return
	}

	// 转码 mp3 落共享缓存（与音箱/供给端点共用），再从 mp3 解码播放。
	// xcoder 不可用（ffmpeg 缺失）→ 降级直接解码源 m4s（mygo 已注册 mp4/aac 解码器）。
	if o.xcoder == nil {
		o.openAndPlay(bvid, opts, gen, m4sPath, false)
		return
	}
	mp3Path, err := o.xcoder.EnsureMP3(m4sPath, bvid, qn)
	if err != nil {
		log.Printf("[mygo-output] 转码失败 %s（降级直接解码 m4s）: %v", bvid, err)
		o.openAndPlay(bvid, opts, gen, m4sPath, false)
		return
	}
	o.openAndPlay(bvid, opts, gen, mp3Path, false)
}

// ensureM4S 确保源音频落主线 cache（与音箱/供给端点共用同一份 m4s 缓存），返回缓存路径。
// 命中直接返回；未命中带 referer 取流（B站 CDN 多节点，拒连重试）→ 完整性校验 → Put。
func (o *MyGoOutput) ensureM4S(bvid string, qn int, url string) (string, error) {
	if o.cache != nil {
		if _, _, ok := o.cache.Get(bvid); ok {
			path := o.cache.FilePath(bvid)
			if fi, err := os.Stat(path); err == nil && fi.Size() > 0 {
				return path, nil
			}
		}
	}
	if url == "" {
		return "", fmt.Errorf("empty audio url")
	}
	var resp *http.Response
	var err error
	for attempt := 0; attempt < 5; attempt++ {
		resp, err = o.bili.GetRaw(url, nil)
		if err == nil {
			break
		}
		time.Sleep(600 * time.Millisecond)
	}
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.ContentLength > 0 && int64(len(data)) != resp.ContentLength {
		return "", fmt.Errorf("truncated %d/%d", len(data), resp.ContentLength)
	}
	if o.cache != nil {
		if err := o.cache.Put(bvid, qn, data); err != nil {
			log.Printf("[mygo-output] 回写缓存失败 %s: %v（不影响播放）", bvid, err)
		}
		return o.cache.FilePath(bvid), nil
	}
	// 无缓存域包（降级）：写临时文件供本次播放，播完清理。
	f, err := os.CreateTemp("", "mymusic-mygo-src-*.m4s")
	if err != nil {
		return "", err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", err
	}
	f.Close()
	return f.Name(), nil
}

// openAndPlay 打开音频文件并提交到 audio.Player。
// temp=true 表示本次是自建临时文件（无缓存域包降级），切歌/出错时才删；
// 共享缓存文件（m4s/mp3/本地源）永远不删——音箱与供给端点也在用。
func (o *MyGoOutput) openAndPlay(bvid string, opts playback.PlayOpts, gen int, path string, temp bool) {
	cleanup := func() {
		if temp {
			os.Remove(path)
		}
	}
	// 解码
	r, err := os.Open(path)
	if err != nil {
		cleanup()
		log.Printf("[mygo-output] 打开音频失败: %v", err)
		if o.onErr != nil {
			o.onErr("打开音频失败：" + err.Error())
		}
		return
	}
	p, err := audio.OpenStream(r)
	if err != nil {
		r.Close()
		cleanup()
		log.Printf("[mygo-output] 解码失败 %s: %v", bvid, err)
		if o.onErr != nil {
			o.onErr("解码失败：" + err.Error())
		}
		return
	}

	// 提交（若期间已切歌 gen 变了 → 放弃这次，清掉）
	o.mu.Lock()
	if o.gen != gen {
		o.mu.Unlock()
		p.Stop()
		cleanup()
		return
	}
	old := o.p
	oldPath, oldTemp := o.path, o.temp
	o.p = p
	o.path = path
	o.temp = temp
	o.bvid = bvid
	o.title = opts.Title
	hook := o.ended
	o.mu.Unlock()
	if old != nil {
		old.Stop()
	}
	if oldTemp && oldPath != "" {
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
	temp := o.temp
	o.p = nil
	o.path = ""
	o.temp = false
	o.bvid = ""
	o.mu.Unlock()
	if p != nil {
		p.Stop()
	}
	if temp && path != "" {
		os.Remove(path) // 只删自建临时文件；共享缓存/本地源永不删
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
