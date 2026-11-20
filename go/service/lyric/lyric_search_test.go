package lyric

import (
	"strings"
	"testing"
)

// CleanSearchKeyword 关键词清洗：对齐 musicxx 智能识别（去 B 站标题噪音）+ bili-music 正则回退
func TestCleanSearchKeyword(t *testing.T) {
	cases := []struct{ in, want string }{
		// 《》优先提取
		{"【4K超清】周杰伦《晴天》MV", "晴天"},
		{"周杰伦「晴天」官方", "晴天"},
		{"周杰伦《晴天》杰伦作曲", "晴天"},
		// 无书名号：剥【】，去噪音词
		{"【4K】周杰伦 - 晴天 (现场版)", "周杰伦 - 晴天"},
		{"周杰伦 晴天 无损 FLAC", "周杰伦 晴天"},
		{"晴 天 1080P 纯享", "晴 天"},
		// 正常合集名（无【】无噪音词）不被误伤
		{"周杰伦50首精选", "周杰伦50首精选"},
		// 空/空白
		{"", ""},
		{"   ", ""},
		// 分隔符归一 + 分P序号前缀（合集场景）
		{"A·B", "A B"},
		{"001.周杰伦-晴天", "周杰伦-晴天"},
		{"12、夜曲", "夜曲"},
		// 繁→简折叠（港台版/繁体标签）
		{"周杰倫《晴天》", "晴天"},
		{"【4K】周杰倫 - 晴天", "周杰伦 - 晴天"},
		{"海闊天空", "海阔天空"},
	}
	for _, c := range cases {
		got := CleanSearchKeyword(c.in)
		if got != c.want {
			t.Errorf("CleanSearchKeyword(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	// 超长截断
	long := strings.Repeat("歌", 60)
	if got := CleanSearchKeyword(long); len(got) > 40 {
		t.Errorf("超长未截断: len=%d", len(got))
	}
}

// SearchLyricCandidatesByTitles 空输入直接 nil（不触发网络）
func TestSearchLyricCandidatesByTitlesEmpty(t *testing.T) {
	if got := SearchLyricCandidatesByTitles(nil, 8, 0); got != nil {
		t.Errorf("空标题应 nil: %v", got)
	}
	if got := SearchLyricCandidatesByTitles([]string{"", "  "}, 8, 0); got != nil {
		t.Errorf("空白标题应 nil: %v", got)
	}
}

// TestRankByDuration 时长重排：版本差异大的候选（如海阔天空 239s vs 326s）按期望时长
// 重排，匹配版本提前；无期望时长或全不匹配时保持原序（网易云相关度）。
func TestRankByDuration(t *testing.T) {
	// 期望 326s：239s 翻录应沉底，326s/323s 提前（标题全相同→标题不干扰）
	cands := []LyricResult{
		{ID: 1, Name: "海阔天空", Duration: 239}, {ID: 2, Name: "海阔天空", Duration: 239},
		{ID: 3, Name: "海阔天空", Duration: 326}, {ID: 4, Name: "海阔天空", Duration: 323},
	}
	rankCandidates(cands, 326, "海阔天空")
	if cands[0].ID != 3 {
		t.Errorf("期望 326s 版本排第一，got ID=%d dur=%d", cands[0].ID, cands[0].Duration)
	}
	if cands[0].Duration != 326 || cands[1].Duration != 323 {
		t.Errorf("前两名应为 326/323，got %d/%d", cands[0].Duration, cands[1].Duration)
	}
	// 无期望时长：不按时长重排，保持原序
	orig := []LyricResult{{ID: 1, Name: "海阔天空", Duration: 239}, {ID: 3, Name: "海阔天空", Duration: 326}}
	rankCandidates(orig, 0, "海阔天空")
	if orig[0].ID != 1 {
		t.Errorf("无期望时长应保持原序，got ID=%d", orig[0].ID)
	}
	// 全不匹配：保持原序
	cands2 := []LyricResult{{ID: 1, Name: "海阔天空", Duration: 239}, {ID: 2, Name: "海阔天空", Duration: 300}}
	rankCandidates(cands2, 326, "海阔天空")
	if cands2[0].ID != 1 {
		t.Errorf("全不匹配应保持原序，got ID=%d", cands2[0].ID)
	}
	// 未知时长（0）与不匹配：都不提前
	cands3 := []LyricResult{{ID: 1, Name: "海阔天空", Duration: 0}, {ID: 2, Name: "海阔天空", Duration: 500}, {ID: 3, Name: "海阔天空", Duration: 326}}
	rankCandidates(cands3, 326, "海阔天空")
	if cands3[0].ID != 3 {
		t.Errorf("唯一匹配应排第一，got ID=%d", cands3[0].ID)
	}
}

func TestRankCandidatesTitleDemotesOtherSong(t *testing.T) {
	// 搜《枫》时网易云把《默》（含周杰伦）排进来 → 标题无重叠应沉底，即使时长更近
	cands := []LyricResult{
		{ID: 1, Name: "默 (Live)", Duration: 133},
		{ID: 2, Name: "枫", Duration: 283},
		{ID: 3, Name: "枫 (钢琴版) [原唱: 周杰伦]", Duration: 282},
	}
	rankCandidates(cands, 275, "枫 周杰伦")
	if cands[0].ID == 1 || cands[1].ID == 1 {
		t.Errorf("不同歌《默》应沉底，got 顺序 %d,%d,%d", cands[0].ID, cands[1].ID, cands[2].ID)
	}
	if cands[0].ID != 3 && cands[0].ID != 2 {
		t.Errorf("两首枫应提前，got 首位 ID=%d", cands[0].ID)
	}
}

func TestTitleScore(t *testing.T) {
	cases := []struct {
		name, expect string
		min                    int
	}{
		{"枫", "枫", 100},
		{"枫 (钢琴版) [原唱: 周杰伦]", "枫 周杰伦", 80},
		{"枫 (Cover 吉拉朵)", "枫", 80},
		{"晴天", "周杰伦 晴天", 80},
		{"默 (Live)", "枫 周杰伦", 0},
		{"", "枫", 0},
	}
	for _, c := range cases {
		if got := titleScore(c.name, c.expect); got < c.min {
			t.Errorf("titleScore(%q, %q)=%d，期望≥%d", c.name, c.expect, got, c.min)
		}
	}
}

func TestCoreTitle(t *testing.T) {
	if got := coreTitle("枫 (钢琴版) [原唱: 周杰伦]"); !strings.HasPrefix(got, "枫") {
		t.Errorf("coreTitle 应以枫开头，got %q", got)
	}
	if got := coreTitle("海阔天空"); got != "海阔天空" {
		t.Errorf("coreTitle(海阔天空)=%q", got)
	}
}
