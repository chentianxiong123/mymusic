# 请愿书：为 MyGo 原生入口导出主线播放核心包

**日期**：2026-05-12（按项目时间）
**申请人**：MyGo 支线（mymusic-mygo）
**请愿对象**：主线 `mymusic/go`（homecast module）维护者
**状态**：待审批

---

## 一、背景与动机

主线 `mymusic`（HomeMusic）目前是 wails3 + htmx 架构。我们正在开发一个**并行的 MyGo 原生入口**（`mymusic-mygo`，Go + MyGo UI + `audio` 原生解码），作为"另一套入口"，**不取代主线、不合并 UI 代码**，只共享**业务核心逻辑**。

MyGo 入口已经验证：
- B站搜索 / 取流 / 下载 / **`audio` 直接解码**（fMP4/AAC，免 ffmpeg 软转码——主线"秒开"痛点的直接解法）
- 原生自绘 UI（深色 + 紫粉渐变）、桌面歌词挂件（透明/置顶/鼠标穿透）、播放控制面板、队列/收藏/设置页

现在需要让 MyGo 入口**复用主线的播放域核心**，而不是各写一套状态机——否则队列/进度/连播/持久化逻辑会在两个入口**分叉漂移**。

## 二、技术障碍（为什么必须请愿）

Go 的 **`internal` 包可见性规则**：`homecast/internal/**` 只能被 **`homecast` 模块树内**的包 import。

`mymusic-mygo` 是**独立 Go module**（有自己的 `go.mod`，`replace homecast => ../mymusic/go`），其 import path 前缀不是 `homecast`，因此：

```go
import "homecast/internal/service/playback" // ❌ 被 Go 拒绝：internal 不可见
```

**结论**：在不改主线结构的前提下，MyGo 无法 import 主线 `internal/service/playback`。要么复制（代码分叉），要么主线提供一个**导出路径**（本请愿）。

## 三、请愿内容（最小改动）

在主线 `mymusic/go/` 增加一个**导出包**，把播放核心暴露给模块外的入口：

```
mymusic/go/
  core/playback/          ← 新增（导出路径，package playback）
    playback.go           （与 internal/service/playback 同源）
    output.go
```

- **只新增** `core/playback/` 两个文件（playback.go、output.go），**不改动** `internal/service/playback/` 现有内容
- **主线现有代码（hx / api / server / web）零改动**——它们继续用 `homecast/internal/service/playback`，不受影响
- 包内容：`internal/service/playback` 的**同源副本**（`PlaybackState` / `Song` / `State` / `SSEMsg` / `Output` 接口）

之后 MyGo 入口改为：

```go
import "homecast/core/playback" // ✅ 模块外可见
```

## 四、为什么选"新增导出副本"而不是"移动"

| 方案 | 主线影响 | 风险 |
|---|---|---|
| **新增 `core/playback` 副本**（本请愿） | 只加 2 文件，现有 import 零改动 | 极低，主线行为不变 |
| 移动 `internal/service/playback` → `core/playback` | 需改 hx/api/server 全部 import | 中，动主线调用链 |
| MyGo 自己复制 playback | 主线零改动 | MyGo 与主线**分叉**，违背共享初衷 |

选新增副本，是因为它把"主线行为不变"放在第一位；`internal` 与 `core` 的同步由后续主线收编时统一（届时再删 internal 引用，本请愿不涉及）。

## 五、请愿的收益

1. **单点维护**：播放域状态机（队列/进度/连播/持久化/广播）只有一份真身，MyGo 与主线共享
2. **零分叉**：两个入口的播放语义（连播、循环、持久化、设备切换）永远一致
3. **接入成本低**：`playback` 包**零内部依赖**（只依赖 `database/sql` 接口 + 自身 `Output`/`Song`），导出即可用，无需改动包内任何逻辑
4. **天然接缝**：`Output` 接口就是给播放后端插的（主线：web/小爱/DLNA；MyGo：`audio` 原生解码）。MyGo 只实现一个 `Output`（`audio.Player` 直解），核心全复用

## 六、风险与对策

- **双份代码（internal + core）**：仅新增副本，主线不删 internal；后续主线收编时决定单源，本请愿不强制
- **包名冲突**：`core/playback` 与 `internal/service/playback` 是不同 import path，无冲突
- **主线构建**：新增纯 Go 包，`go build ./...` 增量，无行为影响

## 七、请求

请主线审批在 `mymusic/go/core/playback/` 新增导出包（同源副本），以便 MyGo 入口直接 import 主线播放核心，实现共享、零分叉。**主线现有逻辑不动。**

---

*附：已调研事实——`internal/service/playback` 包 import 仅含 `database/sql`/`encoding/json`/`log`/`sync`/`time`（标准库 + sql 接口），**不 import 任何 `homecast/...` 内部包**，导出即编译通过（已在支线验证 `go build`）。*
