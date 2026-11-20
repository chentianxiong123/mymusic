//go:build native

package main

import (
	"fmt"
	"testing"

	"time"

	"homecast/bilibili"
)

// 验证：启动恢复后（输出端从未 Play 过）SetPending 记住 bvid，
// Resume 能回退到 Play 加载音源（而不是空转）。
func TestResumeFallsBackToPlay(t *testing.T) {
	bili := bilibili.NewClient("", "", "", 0*time.Second)
	o := newMyGoOutput(bili)

	// 模拟启动恢复：pb restore 只写状态，输出端 o.p==nil
	o.SetPending("BV18gxq6HEgX", "测试歌")
	if o.bvid != "BV18gxq6HEgX" {
		t.Fatalf("SetPending 未记住 bvid: %q", o.bvid)
	}

	// Resume 应走回退分支 → 起后台 Play（gen 递增，o.p 稍后才非 nil）
	if err := o.Resume(); err != nil {
		t.Fatalf("Resume 返回错误: %v", err)
	}
	if o.gen == 0 {
		t.Fatalf("Resume 没有触发 Play（gen 仍为 0）——空转未修复")
	}
	fmt.Printf("✓ Resume 回退到 Play：gen=%d bvid=%s\n", o.gen, o.bvid)
}

// 验证：Stop 后 bvid 清空，SetPending 能重新注入
func TestSetPendingAfterStop(t *testing.T) {
	bili := bilibili.NewClient("", "", "", 0*time.Second)
	o := newMyGoOutput(bili)
	o.SetPending("BV1", "A")
	_ = o.Stop()
	if o.bvid != "" {
		t.Fatalf("Stop 应清空 bvid，实际 %q", o.bvid)
	}
	o.SetPending("BV2", "B")
	if o.bvid != "BV2" {
		t.Fatalf("Stop 后 SetPending 未生效: %q", o.bvid)
	}
	fmt.Printf("✓ Stop 后 SetPending 重新注入成功: %s\n", o.bvid)
}

// 验证：已有音源在播时 SetPending 不覆盖（防抢当前歌）
func TestSetPendingNoOverride(t *testing.T) {
	bili := bilibili.NewClient("", "", "", 0*time.Second)
	o := newMyGoOutput(bili)
	o.mu.Lock()
	o.bvid = "BVplaying"
	o.mu.Unlock()
	o.SetPending("BVnew", "新歌")
	if o.bvid != "BVplaying" {
		t.Fatalf("已加载时不应覆盖，实际 %q", o.bvid)
	}
	fmt.Printf("✓ 正在播时不覆盖: 仍是 %s\n", o.bvid)
}
