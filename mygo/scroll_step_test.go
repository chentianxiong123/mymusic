//go:build native

package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestScrollStepRoundTrip 快速往返点击箭头：第二次基于意图目标而非动画中间位置。
// 回归：之前用 ScrollState.X（动画插值中）算，右点后立刻左点会算出负值 clamp 到 0，
// 表现为"第二下要等第一下滑完"。
func TestScrollStepRoundTrip(t *testing.T) {
	st := &ui.ScrollState{MaxX: 1000}
	var target float32

	// 连点右两次（动画中 X 还没到目标，但目标应累加到 480）
	scrollStep(st, &target, 240, 1)
	if target != 240 {
		t.Fatalf("第一次右点目标=%.0f，期望 240", target)
	}
	st.X = 50 // 模拟动画插值中（才滑了 50）
	scrollStep(st, &target, 240, 1)
	if target != 480 {
		t.Fatalf("动画中第二次右点目标=%.0f，期望 480（不能基于 X=50 算出 290）", target)
	}

	// 立刻反向左点：从意图目标 480 回到 240，而不是从动画中 X 回退
	st.X = 120
	scrollStep(st, &target, 240, -1)
	if target != 240 {
		t.Fatalf("反向左点目标=%.0f，期望 240", target)
	}

	// clamp：左越界不 < 0，右越界不 > MaxX
	scrollStep(st, &target, 240, -1)
	if target != 0 {
		t.Fatalf("左越界目标=%.0f，期望 0", target)
	}
	target = 960
	scrollStep(st, &target, 240, 1)
	if target != 1000 {
		t.Fatalf("右越界目标=%.0f，期望 MaxX=1000", target)
	}
}
