package playback

import "testing"

// TestRandomMode 验证 random 模式：Next/Prev/NextBVID 随机选 ≠ 当前，且不重复同一首。
func TestRandomMode(t *testing.T) {
	p := NewPlaybackState()
	p.Play(Song{BVID: "A"})
	p.Play(Song{BVID: "B"})
	p.Play(Song{BVID: "C"})
	s := p.Snapshot()
	if s.BVID != "C" {
		t.Fatalf("cur=%s", s.BVID)
	}
	p.SetMode("random")
	// Next 多次：每次都必须切到 != 当前，且只在 A/B/C 内
	seen := map[string]bool{}
	cur := "C"
	for i := 0; i < 50; i++ {
		p.Next()
		s = p.Snapshot()
		if s.BVID == cur {
			t.Fatalf("random next 停在当前: %s (i=%d)", s.BVID, i)
		}
		if s.BVID != "A" && s.BVID != "B" && s.BVID != "C" {
			t.Fatalf("random next 越界: %s", s.BVID)
		}
		seen[s.BVID] = true
		cur = s.BVID
	}
	// 50 次内应覆盖至少 2 个不同（随机性粗检）
	if len(seen) < 2 {
		t.Fatalf("random next 未分散: %v", seen)
	}
	// Prev 同样
	cur = p.Snapshot().BVID
	for i := 0; i < 20; i++ {
		p.Prev()
		s = p.Snapshot()
		if s.BVID == cur {
			t.Fatalf("random prev 停在当前: %s", s.BVID)
		}
		cur = s.BVID
	}
}

// TestRandomNextBVID 验证 NextBVID 在 random 下返回 ≠ 当前 且合法。
func TestRandomNextBVID(t *testing.T) {
	p := NewPlaybackState()
	p.Play(Song{BVID: "A"})
	p.Play(Song{BVID: "B"})
	p.Play(Song{BVID: "C"})
	p.SetMode("random")
	for i := 0; i < 30; i++ {
		cur := p.Snapshot().BVID
		nb := p.NextBVID()
		if nb == "" {
			t.Fatalf("NextBVID 空")
		}
		if nb == cur {
			t.Fatalf("NextBVID 返回当前: %s", nb)
		}
		if nb != "A" && nb != "B" && nb != "C" {
			t.Fatalf("NextBVID 越界: %s", nb)
		}
	}
}

// TestOrderModeUnchanged 验证 order 模式保持原有顺序循环（未破坏）。
func TestOrderModeUnchanged(t *testing.T) {
	p := NewPlaybackState()
	p.Play(Song{BVID: "A"})
	p.Play(Song{BVID: "B"})
	p.Play(Song{BVID: "C"})
	p.SetMode("order") // 默认也是 order
	p.Next() // C→A
	if p.Snapshot().BVID != "A" {
		t.Fatalf("order next: %s", p.Snapshot().BVID)
	}
	p.Prev() // A→C
	if p.Snapshot().BVID != "C" {
		t.Fatalf("order prev: %s", p.Snapshot().BVID)
	}
	// NextBVID 顺序：C 的下一首 = A
	if nb := p.NextBVID(); nb != "A" {
		t.Fatalf("order NextBVID: %s", nb)
	}
}

// TestSingleManualNext 验证 single 模式下手动 Next 仍切歌（用户明确要下一首），
// 与前端 ended（重播当前）语义一致。
func TestSingleManualNext(t *testing.T) {
	p := NewPlaybackState()
	p.Play(Song{BVID: "A"})
	p.Play(Song{BVID: "B"})
	p.SetMode("single")
	p.Next() // 手动 next：切到下一首（顺序）
	if p.Snapshot().BVID != "A" { // B 的下一首循环到 A
		t.Fatalf("single manual next: %s", p.Snapshot().BVID)
	}
}
