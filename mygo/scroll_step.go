//go:build native

package main

import (
	"time"

	"github.com/egoist/mygo/ui"
)

// scrollStep 横滑箭头：基于"意图目标"而非动画插值中的 ScrollState.X 计算下一步，
// 支持快速连续点击/往返滑动（之前基于 X 会因为 X 还在动画中途而算出负值或回起点，
// 表现为第二下要等第一下动画结束）。
//
// 注意：不能用 "target 与 X 的差值" 判断用户是否手动拖动——动画进行中 X 必然落后于
// target，那样会把正常连点误判成手动拖动。目标越界（内容变了/MaxX 变小）时才以 X 兜底。
func scrollStep(st *ui.ScrollState, target *float32, step float32, dir float32) {
	base := *target
	if base < 0 || base > st.MaxX {
		base = st.X // 首次或内容变化后越界：从当前位置起步
	}
	v := base + dir*step
	if v < 0 {
		v = 0
	}
	if v > st.MaxX {
		v = st.MaxX
	}
	*target = v
	st.ScrollTo(v, 0, ui.Animation{Duration: 140 * time.Millisecond})
}
