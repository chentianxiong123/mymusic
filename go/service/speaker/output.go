// SpeakerOutput 音箱播放后端：小爱能力（miio/ubus 投送 + 暂停/恢复/切歌/音量）的唯一对接点。
// 实现 playback.Output 接口——播放核心只认 Output，不再 source 分支判断（对接归位）。
//
// 契约（"一切以我为准"）：
//   - 状态完全本地自洽：进度 = 本地模拟时钟（SpeakerProgress），不查音箱（音箱不报真实状态）
//   - 音箱是哑执行器：只接收播放/暂停/停止/音量命令，不读它的状态
//   - 暂停/恢复命令必须真送到音箱（实测 player_stop 能停；player_pause 见实测）
//   - 进度：本地时钟 + 暂停补偿（pauseProgress/resumeProgress 已有）
package speaker

import (
	"fmt"
	"sync"

	"homecast/service/playback"
)

// SpeakerOutput 绑定某台音箱（did），实现 playback.Output
type SpeakerOutput struct {
	svc       *SpeakerService
	did       string
	mu        sync.Mutex
	pendBVID  string // 当前该播的 bvid（无播放 URL 时 Resume 回退到 Play 用）
	pendTitle string
}

// NewSpeakerOutput 构造绑定 did 的音箱输出
func NewSpeakerOutput(svc *SpeakerService, did string) *SpeakerOutput {
	return &SpeakerOutput{svc: svc, did: did}
}

// Play 投送/切歌：bvid → 代理流 → 音箱播放（复用 SpeakerService 全链路），并设自动连播钩子
func (o *SpeakerOutput) Play(bvid string, opts playback.PlayOpts) error {
	q := opts.Quality
	if q == 0 {
		q = 30280 // 音箱默认 30280（B站 DASH 音频，xiaomusic 同款）
	}
	res, err := o.svc.Play(bvid, o.did, q, PlayOpts{
		Title:    opts.Title,
		Duration: opts.Duration,
		NextBVID: opts.NextBVID,
	})
	if err != nil {
		return err
	}
	if c, _ := res["code"].(int); c != 0 {
		msg, _ := res["message"].(string)
		return fmt.Errorf("音箱投送失败 code=%d %s", c, msg)
	}
	// 记住当前歌：若后续 svc.playing[did] 丢了（进程重启后首次恢复），Resume 能重投
	o.mu.Lock()
	o.pendBVID = bvid
	o.pendTitle = opts.Title
	o.mu.Unlock()
	return nil
}

// Pause 暂停（真调音箱 player_pause；"一切以我为准"，状态本地已由核心设 false）
func (o *SpeakerOutput) Pause() error {
	res, err := o.svc.Control(o.did, "pause", 0)
	if err != nil {
		return err
	}
	if c, _ := res["code"].(int); c != 0 {
		return fmt.Errorf("音箱暂停失败 code=%d", c)
	}
	return nil
}

// SetPending 记住「当前该播的 bvid」（不立即投送）。
// 启动恢复（pb restore 成 speaker 模式）时 ss.playing[did] 为空，
// 首次点播放 Control("resume") 会因「无已保存的播放 URL」失败 → 没声音。
// 同步 bvid 后 Resume 可回退到 Play(bvid) 重新投送。
func (o *SpeakerOutput) SetPending(bvid, title string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.pendBVID = bvid
	o.pendTitle = title
}

// Resume 恢复（真调音箱 player_resume + 本地进度补偿）。
// 若本机（进程）从未向该音箱投送过（启动恢复场景，svc.playing[did] 为空），
// Control("resume") 会返回「无已保存的播放 URL」→ 降级用 pendBVID 重新 Play。
func (o *SpeakerOutput) Resume() error {
	res, err := o.svc.Control(o.did, "resume", 0)
	if err == nil {
		if c, _ := res["code"].(int); c == 0 {
			return nil
		}
	}
	// resume 失败：启动恢复后首次播放（无播放 URL）→ 用记住的 bvid 重新投送
	o.mu.Lock()
	bvid, title := o.pendBVID, o.pendTitle
	o.mu.Unlock()
	if bvid == "" {
		if err != nil {
			return err
		}
		if c, _ := res["code"].(int); c != 0 {
			msg, _ := res["message"].(string)
			return fmt.Errorf("音箱恢复失败 code=%d %s", c, msg)
		}
		return nil
	}
	return o.Play(bvid, playback.PlayOpts{Title: title})
}

// Stop 停止
func (o *SpeakerOutput) Stop() error {
	res, err := o.svc.Control(o.did, "stop", 0)
	if err != nil {
		return err
	}
	if c, _ := res["code"].(int); c != 0 {
		return fmt.Errorf("音箱停止失败 code=%d", c)
	}
	return nil
}

// Seek 真 seek：音箱无 seek API，改“投带 start=N 的新 URL 让它重拉”（流端点 mp3
// 分支用 ffmpeg -ss N 切出剩余段）。代价 1~2s（云端指令 + 切分 + 重拉缓冲）。
func (o *SpeakerOutput) Seek(sec float64) error {
	res, err := o.svc.SeekTo(o.did, sec)
	if err != nil {
		return err
	}
	if c, _ := res["code"].(int); c != 0 {
		msg, _ := res["message"].(string)
		return fmt.Errorf("音箱 seek 失败 code=%d %s", c, msg)
	}
	return nil
}

// SetVolume 音量（真调音箱）
func (o *SpeakerOutput) SetVolume(v int) error {
	res, err := o.svc.SetVolume(o.did, v)
	if err != nil {
		return err
	}
	if c, _ := res["code"].(int); c != 0 {
		return fmt.Errorf("音箱音量失败 code=%d", c)
	}
	return nil
}

// Volume 回读音箱当前真实音量（0-100）。切音箱时调一次对齐前端滑块/静音态（否则滑块
// 靠猜 localStorage，与设备实际脱节）。仅切音箱时读一次，非进度轮询（不违反"不读状态"契约——
// 那契约指进度；音量是设备设置项，回读一次对齐 UI 合理）。失败返回 -1（调用方降级用默认）。
func (o *SpeakerOutput) Volume() int {
	status, err := o.svc.GetStatus(o.did)
	if err != nil {
		return -1
	}
	if c, _ := status["code"].(int); c != 0 {
		return -1
	}
	data, _ := status["data"].(map[string]any)
	if data == nil {
		return -1
	}
	v := -1
	if f, ok := data["volume"].(float64); ok {
		v = int(f)
	} else if i, ok := data["volume"].(int); ok {
		v = i
	}
	// volume<=0 视为「回读无效」而非真实音量：xiaoai GetStatus 在音箱尚未开播时
	// info 字段为空 → 返回默认 volume=0。旧实现把 0 当真实值写入 SSOT 并广播 →
	// 首次投送瞬间前端滑块被压成 0（音箱实际音量没变，纯 UI 错）。
	// 真静音由 wantVolume=0（用户点静音）/前端 volMuted 表达，不靠回读。
	if v <= 0 {
		return -1
	}
	return v
}

// Position 音箱当前进度（本地模拟；bvid/title 取自音箱自身进度——核心据此
// 对齐当前歌，自动连播/外部投送后核心与音箱不分裂）
func (o *SpeakerOutput) Position() playback.ProgressInfo {
	d := o.svc.Progress(o.did)
	bvid, _ := d["bvid"].(string)
	title, _ := d["title"].(string)
	pos, _ := d["position"].(float64)
	dur, _ := d["duration"].(float64)
	return playback.ProgressInfo{
		BVID:    bvid,
		Title:   title,
		Pos:     pos,
		Dur:     dur,
		Playing: d["state"] == "playing",
	}
}

// Caps 能力声明（小爱唯一不支持：真 seek；进度为本地模拟非真实）
func (o *SpeakerOutput) Caps() playback.Capabilities {
	return playback.Capabilities{
		Play: true, Pause: true, Resume: true, Stop: true,
		Next: true, Prev: true, Seek: false, Volume: true, Progress: true,
	}
}

// SetEndedHook 注入播完回调（接口要求；音箱播完由 SpeakerService 本地定时器转发，空实现）
func (o *SpeakerOutput) SetEndedHook(fn func()) {}

// Name 输出标识
func (o *SpeakerOutput) Name() string { return "speaker" }
