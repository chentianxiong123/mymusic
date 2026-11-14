# 提案：MyGo 完整功能接入主线 SSOT（公开域包，按 DAG 传播）

**日期**：2026-05-12
**提案人**：MyGo 支线（mymusic-mygo）
**对象**：主线 `mymusic/go`（homecast module）
**性质**：内核评审 + 多入口共享方案。**本版替代** `PROPOSAL_multi_entry_visibility.md`（旧版只公开 playback，不够）与 `PROPOSAL_playback_export.md`（副本方案，已否决）。

---

## 〇、一句话

客户端全栈（MyGo 自己持状态）**丢功能**（持久化/音箱/投屏/登录态/跨端，实测 MyGo 当前 0 个 SQLite 文件、音箱/投屏占位）——不可取。**MyGo 要完整功能，必须复用主线后端能力（SSOT）**，即 MyGo 同进程调主线**域包**。因 Go `internal` 传播规则，MyGo import 的公开域包**必须连带公开其依赖树**（DAG 传播），故公开范围 > 单 `playback`。本文给出**最小可行集**与**完整集**两档，供主线权衡"开口大小 vs 功能完整性"。

---

## 一、为什么不是客户端全栈（驳回）

| 功能 | MyGo 现状（实测） | 主线 |
|---|---|---|
| 持久化（SQLite） | **0 文件**——重启丢队列/收藏/设置/歌词偏移 | `fav/playlist/lyriccache/cache` 域包 |
| 音箱投送 | 投屏页**占位**，无真 API | `speaker` 域包 |
| DLNA 投屏 | 占位，无真投屏 | `cast` 域包 |
| B站登录态/高音质 | 无真登录/大会员音质 | `token` 域包 |
| 跨端同步 / 多端 | 只 Linux | wails3 三端 |

**客户端全栈 = 只保"本地搜索+播放+歌词"，其余全丢。** 故 MyGo 走客户端全栈不可取，必须复用主线 SSOT。

---

## 二、Go `internal` 传播规则（决定公开范围）

MyGo 是独立 module。**MyGo import 的包，其依赖链不能有 `homecast/internal/**`**（否则编译器拦 `internal not allowed`，实测报错）。所以：

> **公开域包 X ⇒ 必须公开 X 的整棵依赖树**（DAG 传播）。

主线域包 DAG（实测）：
- `music` → `bilibili` + `service/lyric`
- `speaker` → `bilibili` + `speaker`(协议) + `service/token`
- `cast` → `bilibili` + `dlna` + `service/token`
- `fav` / `lyric` / `playlist` → `store`
- `lyriccache`/`cache`/`transcode` → `service`(域)
- `playback` → **零内部依赖**（stdlib + sql 接口）

---

## 三、两档公开范围（按 DAG 传播）

### 档 A：核心播放（最小可行集）
| 公开包 | 连带（DAG） |
|---|---|
| `playback` | 无（零依赖） |
| `music` | `bilibili` + `lyric` → `store` |
| `fav`、`playlist` | `store` |
| `lyric` | `store` |
| `store` | 无（纯 SQLite 装配） |

→ 公开 **6 个包 + `store`**：`playback/music/fav/playlist/lyric/store` + 协议层 `bilibili`。
**MyGo 得到**：搜索/取流/播放/收藏/歌单/歌词/**持久化（SQLite 恢复队列收藏）**。

### 档 B：完整功能（+音箱/投屏/登录）
| 追加公开 | 连带 |
|---|---|
| `speaker` | `speaker`(协议) + `token` |
| `cast` | `dlna` + `token` |
| `token`、`lyriccache`、`cache`、`transcode` | 各自依赖 |

→ 再加 `speaker/cast/token/lyriccache/cache/transcode` + 协议层 `speaker/dlna`。
**MyGo 得到**：音箱投送/自动连播、DLNA 投屏、B站登录态/高音质、歌词缓存/转码。

> 档 A 满足"播放器"定位；档 B 才完整对齐主线。**建议先档 A 跑通，音箱/投屏按需走档 B**（MyGo 有 UI 占位，无真 API，暂不需要）。

---

## 四、执行（每档：git mv 移动，非复制，单源）

```
# 每包：git mv go/internal/<路径> go/<公开路径>   # 保留历史
# 主线引用处改 import 前缀：homecast/internal/... → homecast/...
# 验证：go build ./... && go test ./...  全绿（行为零变化）
```

- **移动而非复制** = 永远单源（SSOT 无副本，11-10 理念不破）。
- 主线 `hx/`、`api/`、`server/` 只改 import 前缀字符串，**逻辑零改动**。
- 公开后 `internal/` 里只剩薄胶水层（`hx/`、`api/`、`server`）+ 未被 MyGo 使用的域包（`sites`/`widget` 等）。

---

## 五、MyGo 侧（同进程，单二进制，绕开 htmx/js，功能完整）

```
MyGo main + 主线域包(档A或B) + SQLite  →  单二进制
  UI 命令    →  域包直调（playback.Play/Next/Seek；fav.Add；speaker.Push...）
  状态渲染   →  playback.Subscribe()（chan，直订阅，无 SSE 网络层）
  出声       →  实现 playback.Output（audio.Player 直解）——主线已有插后端机制
  持久化     →  store.Migrate(域包 schemas)（队列/收藏/设置启动恢复）
```

MyGo **不再有独立状态机/独立持久化**，全部走主线域包（SSOT）。`Output` 接口 = 主线"插播放后端"（web/音箱/DLNA 之外再插 MyGo `audio`）。

---

## 六、为什么不偏离内核（回应"会不会偏离文档内核"）

| 维度 | 结论 |
|---|---|
| SSOT / 单源 | **不偏离**：移动非复制，域包仍单份真身（11-10 无副本理念保持） |
| 域包垂直切片 | **不偏离**：仍是域包（逻辑+SQL+表+测试同包），仅目录位置 |
| DAG / 薄胶水 / 表归域 | **不偏离**：包内依赖不变 |
| `internal` 边界 | **有意的多入口豁免**：给"MyGo 共享的域包"开 public 出口，**有文档规则**（§二传播规则）而非临时凿洞；未共享域包（sites/widget）留 internal |
| 内聚纪律 | 核心（域包+同包测试+薄胶水）不变；目录由**新规则**（多入口共享）覆盖 |

**一句话**：公开是**给内核补"多入口共享"规则**，让豁免有据；不是给保护边界凿临时洞。开口大小由**档 A/B** 控制（能少公开就少公开）。

---

## 七、风险与边界

| 项 | 说明 |
|---|---|
| 主线 import 前缀改动 | 只改字符串，`go build`/`go test ./...` 全绿验证 |
| 档 A/B 范围 | 档 A 最小（播放+收藏+歌单+歌词+持久化）；档 B 才含音箱/投屏/登录 |
| 传播规则 | 公开域包必须连带公开依赖树（`internal` 规则），已按 DAG 列全 |
| 进度双写 | MyGo 作唯一前端时无此问题；web+MyGo 并存才需 `source` 标识（另轮） |
| 副本残留 | 无（移动非复制；`core/playback` 旧副本已删） |

## 八、请求

请主线审批：**①在文档补"多入口包可见性规则"（§二传播规则）②按档 A（核心播放）公开域包（git mv + import 前缀，单源）**。主线现有逻辑零行为变化，`go build`/`go test` 全绿后即可；档 B（音箱/投屏/登录）按需后续追加。

---

*附（实测依据）：①`playback` 零内部依赖；②`import homecast/internal/service/playback` 编译器拦 `internal not allowed`；③MyGo 当前 0 个 SQLite 文件、音箱/投屏页为占位（客户端全栈丢功能实证）；④主线域包 DAG 已实测（见 §二）。*
