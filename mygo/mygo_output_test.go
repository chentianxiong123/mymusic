//go:build native

package main

import "testing"

// TestMyGoOutputFinishedHookIndependent 播完统计钩子与核心 ended 钩子互不覆盖。
func TestMyGoOutputFinishedHookIndependent(t *testing.T) {
	o := &MyGoOutput{}
	coreCalled, statCalled := false, false
	o.SetEndedHook(func() { coreCalled = true })
	o.SetFinishedHook(func() { statCalled = true })
	// 只验证两个注入点独立（不依赖 audio.Player 真播完）
	o.ended()
	o.onFinished()
	if !coreCalled || !statCalled {
		t.Fatalf("hook 被覆盖: core=%v stat=%v", coreCalled, statCalled)
	}
}
