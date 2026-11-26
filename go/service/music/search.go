package music

import (
	"strconv"
	"strings"
)

// MusicItem 播放列表条目（前端 MusicItem 对齐）
type MusicItem struct {
	BVID        string `json:"bvid"`
	Title       string `json:"title"`
	Artist      string `json:"artist"`
	Cover       string `json:"cover"`
	Duration    string `json:"duration"`
	DurationSec int    `json:"duration_sec"`
	PlayCount   int    `json:"play_count"`
	Typename    string `json:"typename"`
}

// MusicSearchResult 搜索返回
type MusicSearchResult struct {
	Total int         `json:"total"`
	List  []MusicItem `json:"list"`
}

// ParseDurationSec "3:43" / "1:02:33" → 秒（对齐 Python _parse_duration_sec）
func ParseDurationSec(s string) int {
	parts := []int{}
	for _, p := range strings.Split(s, ":") {
		if p == "" {
			continue
		}
		n, err := strconv.Atoi(p)
		if err != nil {
			return 0
		}
		parts = append(parts, n)
	}
	switch len(parts) {
	case 3:
		return parts[0]*3600 + parts[1]*60 + parts[2]
	case 2:
		return parts[0]*60 + parts[1]
	case 1:
		return parts[0]
	}
	return 0
}

// SongRank 单页排序分（导出供 UI 呈现层用）：越低越靠前。
// 标题包装度（符号/包装词）越多越沉底；过长(>600s)降权。
// 注意：这是排序信号不是过滤——过滤在 filter.go（IsSong），两者都在 UI 单页做，不进数据接口。
func SongRank(typename, title string, durationSec int) int {
	return searchRank(MusicItem{Title: title, Typename: typename, DurationSec: durationSec})
}

// searchRank 加权分（对齐 X11 python 版 _rank）：越低越靠前。
// 标题包装度（符号/包装词）越多越沉底；过长(>600s)降权。
// 注意：这是"同是歌"内部的排序信号，不是过滤——过滤在 filter.go（IsSong）。
func searchRank(it MusicItem) int {
	pkg := 0
	for _, ch := range "[]【】（）()" {
		pkg += strings.Count(it.Title, string(ch))
	}
	low := strings.ToLower(it.Title)
	for _, w := range []string{"4k", "无损", "hi-res", "hires", "爷青回", "修复", "极致", "超清", "高清", "最高音质", "官方", "精彩", "现场"} {
		if strings.Contains(low, w) {
			pkg++
		}
	}
	if it.Typename == "音乐教学" {
		pkg += 2
	}
	if it.DurationSec > 0 && (it.DurationSec < 90 || it.DurationSec > 600) {
		pkg += 2
	}
	return pkg
}

// Search 搜索（纯 API 透传）——与 SearchRaw 相同，不过滤、不排序。
// 过滤/排序是 UI 单页的呈现行为（调用 IsSong / SongRank），不污染数据接口；
// Total 原样取 API 的 numResults，分页严格按 API。
func (m *MusicService) Search(keyword string, page, pageSize int) (*MusicSearchResult, error) {
	return m.SearchRaw(keyword, page, pageSize)
}

// SearchRaw 搜索原始结果——纯 API 透传（类型转换 + 标题清洗），不过滤、不排序。
// 过滤（filter.go IsSong）和排序（SongRank）都是 UI 单页的呈现行为，不污染数据接口。
// Total 原样取 API 的 numResults，与 List 同口径；分页严格按 API。
func (m *MusicService) SearchRaw(keyword string, page, pageSize int) (*MusicSearchResult, error) {
	res, err := m.Client.Search(keyword, page, pageSize)
	if err != nil {
		return nil, err
	}
	items := make([]MusicItem, 0, len(res.Result))
	for _, it := range res.Result {
		items = append(items, MusicItem{
			BVID:        it.BVID,
			Title:       it.Title,
			Artist:      it.Author,
			Cover:       it.Pic,
			Duration:    it.Duration,
			DurationSec: ParseDurationSec(it.Duration),
			PlayCount:   it.Play,
			Typename:    it.Typename,
		})
	}
	return &MusicSearchResult{Total: res.NumResults, List: items}, nil
}
