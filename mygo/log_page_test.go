//go:build native

// log_page_test.go —— 日志筛选映射回归测试（级别 filter 1..4 ↔ e.Level 0..3）。
package main

import (
	"testing"
	"time"
)

func TestLogLevelFilterMapping(t *testing.T) {
	entries := []LogEntry{
		{Time: time.Now(), Level: logDebug, Category: "a", Message: "debug msg"},
		{Time: time.Now(), Level: logInfo, Category: "b", Message: "info msg"},
		{Time: time.Now(), Level: logWarn, Category: "a", Message: "warn msg"},
		{Time: time.Now(), Level: logError, Category: "b", Message: "error msg"},
	}
	// filter 1=DEBUG 2=INFO 3=WARN 4=ERROR
	for i, want := range []string{"debug msg", "info msg", "warn msg", "error msg"} {
		got := filterLogEntries(entries, i+1, "", "")
		if len(got) != 1 || got[0].Message != want {
			t.Errorf("filter %d: want [%s], got %+v", i+1, want, got)
		}
	}
	// 组合筛选
	got := filterLogEntries(entries, 0, "a", "msg")
	if len(got) != 2 {
		t.Errorf("cat+keyword: want 2, got %d", len(got))
	}
}
