// Package lyric 歌词域：网易云词源（解析/搜索/偏移/优选存储）。
// 全链路：入口 internal/api/lyric.go；前端 lyric.js + subtitle.js；表：lyric_sels（LyricSchema 本包）
package lyric

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/yanmingcao/opencc-go"
	"github.com/yanmingcao/opencc-go/pkg/embeddata"
)

// ── 歌词（对齐桌面播放器：网易云免费源，无需登录/cookie） ──

// neteaseNMTID 匿名设备 id：网易云 web 搜索接口（api/search/get/web）缺 NMTID
// cookie 会返回 code 405「操作频繁，请稍候再试」（风控限流）；带随机 NMTID
// + appver/os 即解除。对齐桌面播放器无需登录，用匿名 id 即可。
var neteaseNMTID = func() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err == nil {
		return hex.EncodeToString(b)
	}
	return "0" + fmt.Sprintf("%x", time.Now().UnixNano())
}()

// LyricTable lyric_sels 表名
const LyricTable = "lyric_sels"

// LyricSchema lyric_sels 表定义（表结构归业务域；建表执行在 store.Migrate 统一跑）
const LyricSchema = `CREATE TABLE IF NOT EXISTS lyric_sels (
	bvid TEXT PRIMARY KEY, sid INTEGER NOT NULL DEFAULT 0,
	name TEXT DEFAULT '', artist TEXT DEFAULT '', cover TEXT DEFAULT '',
	duration INTEGER DEFAULT 0, offset REAL DEFAULT 0,
	updated_at INTEGER DEFAULT 0)`

const (
	lyricSearchURL = "https://music.163.com/api/search/get/web"
	lyricFetchURL  = "https://music.163.com/api/song/lyric"
)

var lyricSkipPrefixes = []string{
	"作词", "作曲", "编曲", "制作", "录音", "混音", "监制",
	"OP:", "SP:", "和声", "配唱",
}

var lrcTimeRe = regexp.MustCompile(`\[(\d+):(\d+(?:.\d+)?)\]`)

// offsetTagRe LRC/增强歌词内嵌的整体偏移标签：`[offset:-500]` / `[offset,-500]`
// 语义对齐 bili-music（desktop_lyrics_controller）：标签值负=歌词整体提前。
// 解析时折进每行时间（t' = t + offsetMs/1000），前端零感知。
var offsetTagRe = regexp.MustCompile(`(?i)^\[offset[:,](-?\d+)\]$`)

// titleBracketRe 歌名归一用：各类型括号及其内容（版本标记）。（）()【】[]《》「」以及方括号。
var titleBracketRe = regexp.MustCompile(`[（(【\[《「][^）)】\]》」]*[）)】\]》」]`)

// titleVersionWords 歌名归一时剥离的版本/音质标记词（对齐 musicxx 版本字段权重思路：
// 版本是独立维度，不应让“枫(钢琴版)”被误判成另一首歌）。
var titleVersionWords = []string{
	"钢琴版", "原唱", "伴奏", "演奏版", "粤语版", "国语版", "完整版", "现场版", "无损版",
	"封面", "翻唱", "cover", "live", "dj", "remix", "rnb", "纯享", "无损", "hi-res", "flac",
	"wav", "4k", "8k", "超清", "48k", "192k", "320k",
}

// ── 搜索关键词清洗（musicxx 智能识别规则子集 + bili-music 正则回退同款） ──
// 对齐 bili-music meting_logic.extractSearchKeyword：《》「」优先提取，剥【】；
// 再按 musicxx 智能识别思路去 B 站标题噪音词（4K/无损/纯享…）：
// B 站标题混冗余是歌词匹配错的第一大原因（musicxx 文档原话）。
var (
	bookTitleRe = regexp.MustCompile(`[《「〈]([^》」〉]+)[》」〉]`)
	bracketRe   = regexp.MustCompile(`[【\[(][^】\])]*[】\])]`)
	sepRe       = regexp.MustCompile(`[·•|/\\]`)
	seqPrefixRe = regexp.MustCompile(`^\s*\d+[.、．]\s*`) // 分P序号前缀：“001.周杰伦-晴天”→“周杰伦-晴天”
	// 画质/格式/版本噪音词（不杀“翻唱/现场/Live”——那是版本信息，musicxx 归为版本字段权重，非删）
	noiseWords = []string{"1080P", "4K", "720P", "超清", "高清", "无损", "FLAC", "HIRES", "HQ", "MV", "官方", "哔哩哔哩", "BILI", "纯享", "完整版", "现场版"}
)

// ── 繁简折叠（OpenCC t2s，内嵌数据免外部文件）──
// 华语歌词匹配错的一大来源：标题/歌手是繁体（港台版、官方繁体标签）。
// 折叠后统一走简体匹配。
var (
	simplifierOnce sync.Once
	simplifier     *opencc.SimpleConverter
)

// toSimplified 繁体→简体（t2s）；转换器初始化失败时原样返回（不阻断清洗）。
// opencc-go v1.0.0 的 MaxMatchSegmentation 遇到末尾截断的多字节 UTF-8 会越界 panic
// （nextUTF8CharLength 返回 4 但剩余不足 4 字节 → text[position:position+4]）。
// 先洗掉非法序列从根上避开，再 recover 兜底任何上游 panic：搜索关键词不能让应用崩。
func toSimplified(s string) (out string) {
	if s == "" {
		return s
	}
	defer func() {
		if recover() != nil {
			out = s // 上游 panic 时返回原文，不中断歌词搜索
		}
	}()
	s = strings.ToValidUTF8(s, "") // 根因：截尾多字节字符 → 非法 UTF-8
	simplifierOnce.Do(func() {
		if data, err := embeddata.GetConfig("t2s"); err == nil {
			if c, err := opencc.NewSimpleConverterFromData(data); err == nil {
				simplifier = c
			}
		}
	})
	if simplifier == nil {
		return s
	}
	return simplifier.Convert(s)
}

// CleanSearchKeyword 搜索关键词清洗：歌名优先（《》内），繁→简，去噪音词，折叠空白。
func CleanSearchKeyword(title string) string {
	s := strings.TrimSpace(title)
	if s == "" {
		return ""
	}
	s = toSimplified(s)
	if m := bookTitleRe.FindStringSubmatch(s); m != nil {
		return strings.TrimSpace(m[1])
	}
	s = bracketRe.ReplaceAllString(s, " ")
	up := strings.ToUpper(s)
	for _, w := range noiseWords {
		up = strings.ReplaceAll(up, w, " ")
	}
	s = sepRe.ReplaceAllString(up, " ")
	s = seqPrefixRe.ReplaceAllString(s, "")
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 40 {
		s = s[:40] // 防超长关键词
	}
	return strings.TrimSpace(s)
}

// timedLineRe 增强格式行：`[startMs,durMs](字级时间戳…) 文本`（yrc/qrc/krc）
var timedLineRe = regexp.MustCompile(`^\[(\d+),(\d+)(?:,\d+)?\]\s*(.*)$`)

// timedWordRe 字级时间戳 `(ms,ms,dur)` / `<ms,ms>`（逐字卡拉OK），解析时剥离
var timedWordRe = regexp.MustCompile(`(?:\(-?\d+,-?\d+(?:,-?\d+)?\)|<-?\d+,-?\d+(?:,-?\d+)?>)`)

// timedWordNumRe 从单个字级戳里取数字：(startMs, durMs, flag) 或 <startMs, durMs>。
// 网易云 YRC/qrc 字级戳第 2 个数是「时长」（durMs），不是结束时间——实测
// (1000,500,0) 表示 1.0s 起持续 0.5s。渲染用 start+dur 得结束。
var timedWordNumRe = regexp.MustCompile(`-?\d+`)

// WordSpan 逐字时间片（秒，绝对时间轴，与 LyricLine[0] 同基准）。
// Start 是该字开始，End 是结束；web 端用它做 background-clip:text 的逐字渐变。
type WordSpan struct {
	S float64 `json:"s"` // start 秒
	E float64 `json:"e"` // end 秒
}

// LyricLine [秒, 行, 行时长秒]（前端 LyricPanel 用 l[0]/l[1]，l[2] 行时长：0=未知，前端用下一行差估算进度）
type LyricLine [3]any

// LyricResult 歌词返回（对齐 Python get_lyric_lines）
type LyricResult struct {
	ID       int         `json:"id"`
	Name     string      `json:"name"`
	Artist   string      `json:"artist"`
	Album    string      `json:"album"`
	Duration int         `json:"duration"` // 秒（候选行展示 mm:ss）
	Lines    []LyricLine `json:"lines"`
	// Words 与 Lines 一一对应：YRC 逐字歌词才有值（每行的时间片），LRC 为 nil。
	// 独立字段而非塞进 LyricLine：widget/mygo 的 [3]any 消费者零改动，web 端按需读。
	Words  [][]WordSpan `json:"words,omitempty"`
	Offset float64      `json:"offset,omitempty"` // 本歌已存档的对齐偏移（秒，bvid 模式返回）
}

// parseLRC LRC 文本 → [][秒,行,时长]，时间升序；过滤元信息行；识别 [offset:] 标签折入行时间；无词返回 nil
func parseLRC(text string) []LyricLine {
	var lines []LyricLine
	offsetMs := 0
	for _, raw := range strings.Split(text, "\n") {
		ln := strings.TrimSpace(raw)
		if m := offsetTagRe.FindStringSubmatch(ln); m != nil {
			offsetMs, _ = strconv.Atoi(m[1]) // 文件内整体偏移（bili-music 同款语义）
			continue
		}
		m := lrcTimeRe.FindStringSubmatch(ln)
		if m == nil {
			continue
		}
		mm, _ := strconv.Atoi(m[1])
		ss, _ := strconv.ParseFloat(m[2], 64)
		t := float64(mm)*60 + ss
		txt := strings.TrimSpace(ln[len(m[0]):])
		if txt == "" || hasPrefixAny(txt, lyricSkipPrefixes) {
			continue
		}
		lines = append(lines, LyricLine{t + float64(offsetMs)/1000, txt, 0.0})
	}
	sort.Slice(lines, func(i, j int) bool {
		return lines[i][0].(float64) < lines[j][0].(float64)
	})
	if len(lines) == 0 {
		return nil
	}
	return lines
}

// parseYRC 增强格式（yrc 逐字卡拉OK / qrc / krc）：`[start,dur](字级…) 文本`
// 行时长是自带的（LRC 没有，只能靠下一行差值估算）→ 逐行进度直接用；剥离字级时间戳；过滤元信息行
func parseYRC(text string) []LyricLine {
	var lines []LyricLine
	offsetMs := 0
	for _, raw := range strings.Split(text, "\n") {
		ln := strings.TrimSpace(raw)
		if m := offsetTagRe.FindStringSubmatch(ln); m != nil {
			offsetMs, _ = strconv.Atoi(m[1])
			continue
		}
		m := timedLineRe.FindStringSubmatch(ln)
		if m == nil {
			continue
		}
		start, _ := strconv.Atoi(m[1])
		dur, _ := strconv.Atoi(m[2])
		txt := strings.TrimSpace(timedWordRe.ReplaceAllString(m[3], ""))
		if txt == "" || hasPrefixAny(txt, lyricSkipPrefixes) {
			continue
		}
		lines = append(lines, LyricLine{
			float64(start+offsetMs) / 1000,
			txt,
			float64(dur) / 1000,
		})
	}
	sort.Slice(lines, func(i, j int) bool {
		return lines[i][0].(float64) < lines[j][0].(float64)
	})
	if len(lines) == 0 {
		return nil
	}
	return lines
}

// parseYRCWords 从增强歌词原文提取逐字时间片，返回与有效歌词行一一对应的 word 列表。
// 只在行有词（与 parseYRC 的过滤条件一致）时 append，保证 Words[i] 对应 Lines[i]。
// LRC 没有字级戳 → 返回 nil，web 端据此回退到行级进度。
func parseYRCWords(text string) [][]WordSpan {
	var out [][]WordSpan
	offsetMs := 0
	for _, raw := range strings.Split(text, "\n") {
		ln := strings.TrimSpace(raw)
		if m := offsetTagRe.FindStringSubmatch(ln); m != nil {
			offsetMs, _ = strconv.Atoi(m[1])
			continue
		}
		m := timedLineRe.FindStringSubmatch(ln)
		if m == nil {
			continue
		}
		txt := strings.TrimSpace(timedWordRe.ReplaceAllString(m[3], ""))
		if txt == "" || hasPrefixAny(txt, lyricSkipPrefixes) {
			continue
		}
		stamps := timedWordRe.FindAllString(m[3], -1)
		if len(stamps) == 0 {
			out = append(out, nil) // 行有效但无字级戳（qrc 变体）→ 该行回退行级进度
			continue
		}
		spans := make([]WordSpan, 0, len(stamps))
		for _, st := range stamps {
			nums := timedWordNumRe.FindAllString(st, -1)
			if len(nums) < 2 {
				continue
			}
			startMs, _ := strconv.ParseFloat(nums[0], 64)
			durMs, _ := strconv.ParseFloat(nums[1], 64)
			// 第 3 个数若存在且 >0，用它作时长（krc 变体 (start,end,dur)）
			if len(nums) >= 3 {
				if d, _ := strconv.ParseFloat(nums[2], 64); d > 0 {
					durMs = d
				}
			}
			startMs += float64(offsetMs)
			spans = append(spans, WordSpan{S: startMs / 1000, E: (startMs + durMs) / 1000})
		}
		out = append(out, spans)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// parseLyrics 自动检测：行首 `[数字,数字]`（增强格式 yrc/qrc/krc）→ 逐字解析；否则 LRC
func parseLyrics(text string) []LyricLine {
	for _, raw := range strings.Split(text, "\n") {
		ln := strings.TrimSpace(raw)
		if ln == "" {
			continue
		}
		if timedLineRe.MatchString(ln) {
			return parseYRC(text)
		}
		if lrcTimeRe.MatchString(ln) {
			break // LRC，走 parseLRC
		}
	}
	return parseLRC(text)
}

func hasPrefixAny(s string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

var neteaseHC = &http.Client{Timeout: 15 * time.Second}

func neteaseGet(u string) (map[string]any, error) {
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Referer", "https://music.163.com/")
	// NMTID 匿名 cookie 免限流（网易云 web 接口 405 风控修复）
	req.Header.Set("Cookie", "NMTID="+neteaseNMTID+"; appver=8.7.01; os=pc; channel=netease")
	resp, err := neteaseHC.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var j map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&j); err != nil {
		return nil, err
	}
	return j, nil
}

// fetchLyricRaw 网易云歌词原始文本（sid=歌曲id）→ (lrc, yrc)
// lv/kv/tv/rv/yv 全开：lrc=主歌词，yrc=逐字卡拉OK（增强格式，带行时长），tlyric=翻译（tv=1 不再主动关）
// 之前 tv=-1 只取 lrc，逐字/翻译白丢（bili-music 同款语义：有 karaokeLyric 优先）
func fetchLyricRaw(sid int) (lrcText, yrcText string) {
	params := url.Values{}
	params.Set("id", strconv.Itoa(sid))
	params.Set("lv", "1")
	params.Set("kv", "1")
	params.Set("tv", "1")
	params.Set("rv", "1")
	params.Set("yv", "1")
	j, err := neteaseGet(lyricFetchURL + "?" + params.Encode())
	if err != nil {
		return
	}
	if code, _ := j["code"].(float64); int(code) == 200 {
		if lrc, ok := j["lrc"].(map[string]any); ok {
			lrcText, _ = lrc["lyric"].(string)
		}
		if yrc, ok := j["yrc"].(map[string]any); ok {
			yrcText, _ = yrc["lyric"].(string)
		}
	}
	return
}

// fetchLyricLines sid → 歌词行 + 逐字时间片（优先 yrc 逐字版，其次 lrc；[offset:] 标签已折入）。
// yrc 同时解析字级戳填 words；lrc 无字级戳 → words=nil（web 端回退行级进度）。
func fetchLyricLines(sid int) ([]LyricLine, [][]WordSpan) {
	lrcT, yrcT := fetchLyricRaw(sid)
	if yrcT != "" {
		if l := parseLyrics(yrcT); l != nil {
			return l, parseYRCWords(yrcT)
		}
	}
	if lrcT != "" {
		return parseLyrics(lrcT), nil
	}
	return nil, nil
}

// SearchLyricCandidates 搜歌名 → 候选。惰性取词（musicxx 两段式 API 同思路）：
// 搜索只回元数据；候选先按期望时长重排（时长匹配的版本优先，避免“海阔天空 239s
// 翻录排第一、326s 原版排第三”这类版本错配），再对重排后第一个 ≥3 行候选验证并带
// Lines（保住“自动选第一有词源、自动跳脏候选”语义），其余候选不拉正文（前端选中
// 时懒拉 /api/v1/music/lyric?sid=）→ 请求数可控。
func SearchLyricCandidates(keyword string, limit, expectDur int) []LyricResult {
	keyword = CleanSearchKeyword(keyword)
	if keyword == "" {
		return nil
	}
	if limit <= 0 || limit > 20 {
		limit = 8
	}
	params := url.Values{}
	params.Set("s", keyword)
	params.Set("type", "1")
	params.Set("limit", strconv.Itoa(limit))
	params.Set("offset", "0")
	j, err := neteaseGet(lyricSearchURL + "?" + params.Encode())
	if err != nil {
		return nil
	}
	result, _ := j["result"].(map[string]any)
	songs, _ := result["songs"].([]any)
	// 先收齐候选元数据（含时长），再按期望时长重排。
	var out []LyricResult
	for _, s := range songs {
		song, ok := s.(map[string]any)
		if !ok {
			continue
		}
		sid := int(toFloat(song["id"]))
		if sid == 0 {
			continue
		}
		artists, _ := song["artists"].([]any)
		artist := ""
		if len(artists) > 0 {
			if a, ok := artists[0].(map[string]any); ok {
				artist, _ = a["name"].(string)
			}
		}
		album := ""
		if al, ok := song["album"].(map[string]any); ok {
			album, _ = al["name"].(string)
		}
		out = append(out, LyricResult{
			ID:       sid,
			Name:     strAny(song["name"]),
			Artist:   artist,
			Album:    album,
			Duration: int(toFloat(song["duration"]) / 1000),
		})
	}
	rankCandidates(out, expectDur, keyword)
	// 重排后逐个拉词，第一个 ≥3 行即选定（带 Lines）；其余仅元数据（懒拉）。
	gotLines := false
	res := out[:0]
	for _, r := range out {
		if !gotLines {
			if lines, words := fetchLyricLines(r.ID); len(lines) >= 3 {
				r.Lines = lines
				r.Words = words
				gotLines = true
			} else {
				continue // 无词/拉词失败：不展示（保持“候选只含有词≥3行”体验）
			}
		}
		res = append(res, r)
	}
	return res
}

// rankCandidates 候选重排：**时长主 + 标题次 + 网易云相关度兜底**（两次稳定排序，
// 字典序：时长匹配优先，同分按标题相似度，再同分保持网易云原序）。
// expectDur<=0 时只做标题重排；两者都无时保持原序，不误伤。
func rankCandidates(cands []LyricResult, expectDur int, expectTitle string) {
	// 次级：标题相似度（先排，稳定）——把“搜“枫”却排进来《默》”这类不同歌沉底
	sort.SliceStable(cands, func(i, j int) bool {
		return titleScore(cands[i].Name, expectTitle) > titleScore(cands[j].Name, expectTitle)
	})
	// 主级：时长匹配（后稳定排，保持标题次序）
	if expectDur > 0 {
		sort.SliceStable(cands, func(i, j int) bool {
			return durScore(cands[i].Duration, expectDur) > durScore(cands[j].Duration, expectDur)
		})
	}
}

// coreTitle 歌名归一（学 musicxx 智能识别的思路，取标题核心）：小写、剥各类型括号内容
// （版本标记：钢琴版/Live/DJ/Remix/Cover/原唱等）、去版本词、去连字符、去重空白。
// 让“枫 (钢琴版) [原唱: 周杰伦]”与“枫”归到同一核心，不被误判成不同歌。
func coreTitle(s string) string {
	if s == "" {
		return ""
	}
	t := strings.ToLower(strings.TrimSpace(s))
	t = titleBracketRe.ReplaceAllString(t, " ")
	for _, w := range titleVersionWords {
		t = strings.ReplaceAll(t, w, " ")
	}
	t = strings.ReplaceAll(t, "-", " ")
	t = strings.Join(strings.Fields(t), " ")
	return strings.TrimSpace(t)
}

// titleScore 候选歌名与期望标题相似度（0-100）：相同 100，包含 80，共享 2 字子串 60，
// 无重叠 0（大概率不同歌，沉底）。expectTitle 通常就是清洗后的搜索关键词（常含标题）。
func titleScore(name, expect string) int {
	n := coreTitle(name)
	e := coreTitle(expect)
	if n == "" || e == "" {
		return 0
	}
	if n == e {
		return 100
	}
	if strings.Contains(n, e) || strings.Contains(e, n) {
		return 80
	}
	if sharesTwoRunes(n, e) {
		return 60
	}
	return 0
}

// sharesTwoRunes 两个字符串是否共享连续 2 个码元（中文歌名核心字）。
func sharesTwoRunes(a, b string) bool {
	ra := []rune(a)
	if len(ra) < 2 {
		return false
	}
	for i := 0; i+1 < len(ra); i++ {
		if strings.Contains(b, string(ra[i:i+2])) {
			return true
		}
	}
	return false
}

// durScore 时长匹配分：±2s 满分，±5s 良，±10s 及格；其余/未知 0（不提前，保持原序）。
func durScore(d, expect int) int {
	if d <= 0 {
		return 0
	}
	diff := d - expect
	if diff < 0 {
		diff = -diff
	}
	switch {
	case diff <= 2:
		return 1000
	case diff <= 5:
		return 600
	case diff <= 10:
		return 300
	default:
		return 0
	}
}

// GetLyricLines 按备选关键词取第一源；或 sid 精确取。找不到返回 nil（业务正常）
func GetLyricLines(keywords []string, sid, expectDur int) *LyricResult {
	if len(keywords) == 0 && sid == 0 {
		return nil // 空关键词不发网易云（挂件 bvid 模式无已选源时）
	}
	if sid > 0 {
		lines, words := fetchLyricLines(sid)
		if lines != nil {
			return &LyricResult{ID: sid, Lines: lines, Words: words}
		}
		return nil
	}
	cands := SearchLyricCandidatesByTitles(keywords, 8, expectDur)
	if len(cands) > 0 && len(cands[0].Lines) >= 3 {
		return &cands[0]
	}
	return nil
}

// SearchLyricCandidatesByTitles 备用标题逐个搜（bili-music lyricSearchTitles 同款）：
// 第一个搜到候选（且第一候选有词）的关键词即用，避免“第一P占位标题”直接翻车。
func SearchLyricCandidatesByTitles(titles []string, limit, expectDur int) []LyricResult {
	for _, kw := range titles {
		if kw == "" {
			continue
		}
		if res := SearchLyricCandidates(kw, limit, expectDur); len(res) > 0 {
			return res
		}
	}
	return nil
}

// ── 已选歌词源记忆（持久化：SQLite lyric_sels 表） ──
// 对齐 music.py 的 LYRIC_MAP（bvid → 已选歌词版本 {id,name,artist,dur,offset}）：
// 选的是哪个候选 + 手动对齐偏移 ±0.5s，都是用户数据，随增删改查持久化，不是缓存。
// 歌源正文缓存（前端 localStorage LRU）与这里互不干扰：缓存负责“秒显歌词行”，
// 这里负责“记住选了什么、调了多少”。

// LyricSel 一条已选歌词记录（bvid → 候选源 + 对齐偏移）
type LyricSel struct {
	BVID     string
	SID      int
	Name     string
	Artist   string
	Cover    string
	Duration int
	Offset   float64
}

// LyricStore SQLite 持久化 + 内存兜底（db==nil 时退化为内存 map，测试/无库环境用）
type LyricStore struct {
	db  *sql.DB
	mu  sync.Mutex
	mem map[string]LyricSel
}

// NewLyricStore 构造；db 可 nil（纯内存模式）
func NewLyricStore(db *sql.DB) *LyricStore {
	return &LyricStore{db: db, mem: map[string]LyricSel{}}
}

// SetSelection 记录/更新歌曲的已选歌词源（sid<=0 清除记录）
func (s *LyricStore) SetSelection(bvid string, sel LyricSel) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if bvid == "" {
		return
	}
	if sel.SID <= 0 {
		delete(s.mem, bvid)
		if s.db != nil {
			_, _ = s.db.Exec(`DELETE FROM lyric_sels WHERE bvid = ?`, bvid)
		}
		return
	}
	s.mem[bvid] = sel
	if s.db != nil {
		_, _ = s.db.Exec(`INSERT INTO lyric_sels (bvid,sid,name,artist,cover,duration,offset,updated_at)
			VALUES (?,?,?,?,?,?,?,?) ON CONFLICT(bvid) DO UPDATE SET
			sid=excluded.sid, name=excluded.name, artist=excluded.artist, cover=excluded.cover,
			duration=excluded.duration, updated_at=excluded.updated_at`,
			sel.BVID, sel.SID, sel.Name, sel.Artist, sel.Cover, sel.Duration, sel.Offset, time.Now().Unix())
	}
}

// SelectedSID 返回 bvid 的已选歌词源 sid（无则 0）
func (s *LyricStore) SelectedSID(bvid string) int {
	v, _ := s.Get(bvid)
	return v.SID
}

// Get 返回 bvid 的完整记录（无则零值 + false）
func (s *LyricStore) Get(bvid string) (LyricSel, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if bvid == "" {
		return LyricSel{}, false
	}
	if s.db == nil {
		v, ok := s.mem[bvid]
		return v, ok
	}
	var v LyricSel
	err := s.db.QueryRow(`SELECT bvid,sid,name,artist,cover,duration,offset FROM lyric_sels WHERE bvid = ?`, bvid).
		Scan(&v.BVID, &v.SID, &v.Name, &v.Artist, &v.Cover, &v.Duration, &v.Offset)
	if err != nil {
		return LyricSel{}, false
	}
	return v, true
}

// SetOffset 调整歌曲的歌词对齐偏移（无记录时建一条空源记录，只记偏移）
func (s *LyricStore) SetOffset(bvid string, offset float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if bvid == "" {
		return
	}
	old := s.mem[bvid]
	old.BVID = bvid
	old.Offset = offset
	s.mem[bvid] = old
	if s.db != nil {
		_, _ = s.db.Exec(`INSERT INTO lyric_sels (bvid,sid,name,artist,cover,duration,offset,updated_at)
			VALUES (?,0,'','','',0,?,?) ON CONFLICT(bvid) DO UPDATE SET offset=excluded.offset, updated_at=excluded.updated_at`,
			bvid, offset, time.Now().Unix())
	}
}

// ── 小工具 ──

func toFloat(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case json.Number:
		f, _ := n.Float64()
		return f
	}
	return 0
}

func strAny(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}
