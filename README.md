# mymusic-mygo —— MyGo 框架试水支线

以 `../mymusic`（HomeMusic，B站音乐播放器，wails3 + htmx + SQLite 三端）为参考，
用 **[MyGo](https://mygo.run)**（Go 优先桌面框架，无 cgo）开的**最小试水切片**，验证
MyGo 能否优雅解决原项目最痛的部分。

> 试水结论：**能，且关键痛点被 MyGo 直接消解**（见下）。

## 验证了什么（全部实测通过）

| 能力 | 验证方式 | 结果 |
|---|---|---|
| 窗口 + 原生 UI 渲染 | xvfb 截图 | ✅ 搜索框/列表/控制条全部渲染（深色主题） |
| B站搜索网络链路 | `go test -tags live` | ✅ 0.36s 返回 20 条，解析正确 |
| 取流→下载→fMP4 解码 | `go test -tags live` | ✅ 8.7MB AAC 解码，`duration=4m59s` 正确 |
| 真实播放 + 进度 | xvfb + `MYGO_DEMO=1` 截图 | ✅ `0:05 / 4:59` 实时走 |
| 桌面歌词挂件 | 截图（无边框+透明+置顶） | ✅ 浮窗显示歌词文本 |

## 核心发现（相比原项目）

1. **歌词挂件不再需要 cgo**。原项目挂件是 GTK3/Win32 手写显示层（cgo，必须各自 C 工具链，
   交叉编译痛苦）；MyGo 原生 UI 用**一行选项**做无边框+透明+置顶+不进任务栏，`win.SetIgnoreMouseEvents(true)`
   一行做鼠标穿透。**无任何 cgo**，`CGO_ENABLED=0` 任意机器交叉编译。
2. **B站 fMP4/AAC 无需软转码秒开**。原项目有 `service/fmp4` + `service/conv` 做"软转码秒开"
   （音箱出声延迟痛点）；MyGo `audio` 包 + `audio/aac` 解码器**直接原生解码** B站 DASH 音频
   （`mp4a.40.2`），`audio.OpenStream` 一条链路搞定。
3. **B站客户端零成本复用**。`bilibili/`（原 `go/internal/bilibili`）纯标准库、零外部依赖，直接复制即可。
   关键参数对齐原项目：UA=Windows Chrome/120、referer=`search.bilibili.com`（www.bilibili.com 会触发 412 风控）。

## 异步模型（写 MyGo UI 的要点）

- view 是**纯函数**读 app 状态（主线程）；goroutine 只能通过 `win.Update(func)`（异步，postMain）改状态。
- **同步读状态用 `mygo.RunOnMain(func)`**（阻塞等待执行）。`win.Update` 不等待——**别用它读值**（踩过：读到零值）。
- `win.Invalidate()` 触发一帧重绘（状态已在主线程改好时用）；`watchProgress` 用 ticker + RunOnMain 比较 + Invalidate。
- `audio.Player` 自身 goroutine-safe；进度用闭包持有 player 指针，避免读字段竞争。

## 运行

```bash
go tool mygo dev        # 开发（live reload）
go run .                # 或直接跑
MYGO_DEMO=1 go run .    # 自检：自动搜索"周杰伦"→播放→开歌词挂件（xvfb/无音频设备下进度仍走）
```

Live 网络测试（需网络）：

```bash
go test -tags live -v -run 'TestLive' .
```

## 结构

```
mymusic-mygo/
├── main.go       # app 状态 + 主窗口 view（搜索/列表/控制条）
├── player.go     # 搜索/播放/进度 goroutine 逻辑 + MYGO_DEMO 自检
├── widget.go     # 桌面歌词挂件窗口（无边框/透明/置顶/穿透）
├── bilibili/     # 复制自 mymusic/go/internal/bilibili（纯标准库）
└── live_test.go  # live 网络验证（build tag: live）
```

## 后续可做（试水未覆盖）

- 真机出声验证（xvfb 无音频设备；播放时钟在走，真机会出声）
- 真实歌词：B站字幕/歌词 API 拉取 + 逐行高亮（当前用歌名占位）
- 流式秒开：整首下载 → 边下边播（B站 DASH 分段，MyGo fMP4 支持顺序解码）
- 收藏/歌单（原 `service/fav`、`playlist` 域包同样可复制复用）
- 托盘/通知/快捷键/最近文档（MyGo 桌面 API 全有）
- 音频输出延迟：MyGo `audio.SetLatency(LatencyInteractive)` 可调
