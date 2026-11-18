//go:build native

package main

import (
	"encoding/base64"
	"fmt"
	"homecast/bilibili"
	"homecast/service/speaker"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
)

func (a *app) castPage(c *ui.Context) {
	// 整页滚动条（对齐主线页面滚动条）：整页内容包一个 Scroll，内容超出窗口
	// 时整页滚动；不做组件内部滚动条（DLNA 列表不再单独 Scroll）。
	ui.Scroll(c).Grow(1).Scrollbars(true).Children(func() {
		ui.Column(c).Fill().PaddingX(a.resp(24, 12)).PaddingY(a.resp(16, 10)).Gap(12 * uiScale).Children(func() {
			ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(10 * uiScale).Children(func() {
				ui.Text(c, "投屏 / 音箱").FontSize(20 * uiScale).Bold().TextColor(clrText)
				ui.Spacer(c).Grow(1)
				if chip(c, "发现设备").Clicked() {
					go a.discoverCast()
				}
			})
			// 当前投屏状态（对齐主线 d-stop：投屏中可停止）
			if a.castOn != "" {
				ui.Box(c).Fill().Radius(12 * uiScale).PaddingX(16 * uiScale).PaddingY(12 * uiScale).Background(clrCard).Children(func() {
					ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(12 * uiScale).Children(func() {
						icon(c, icCast, 18, clrPink)
						ui.Text(c, "正在投屏："+a.castOn).FontSize(14 * uiScale).TextColor(clrText).Bold()
						ui.Spacer(c).Grow(1)
						if chip(c, "停止投屏").Clicked() {
							a.stopCast()
						}
					})
				})
			}
			// 小米音箱（SSOT：xiaoai auth 登录态；未登录→扫码；已登录→账户头+刷新+导出 token）
			loggedIn := a.svc != nil && a.svc.speakerLoggedIn()
			if loggedIn {
				ui.Box(c).Fill().Radius(12 * uiScale).PaddingX(16 * uiScale).PaddingY(12 * uiScale).Background(clrCard).Children(func() {
					ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(10 * uiScale).Children(func() {
						icon(c, icMsg, 18, clrPurple)
						ui.Column(c).Grow(1).Gap(2 * uiScale).Children(func() {
							ui.Text(c, "小爱音箱").FontSize(14 * uiScale).Bold().TextColor(clrText)
							uid := a.svc.speakerUserID()
							ui.Text(c, "已登录 · "+uid).FontSize(11 * uiScale).TextColor(clrMuted)
						})
						if chip(c, "刷新设备").Clicked() {
							go a.refreshSpeakers()
						}
						if chip(c, "复制 token").Clicked() {
							if tok, err := a.svc.exportSpeakerToken(); err == nil {
								mygo.Clipboard.WriteText(tok)
								a.status = "已复制音箱 token（多端免扫码复用）"
							} else {
								a.status = err.Error()
							}
							a.win.Invalidate()
						}
					})
				})
			} else {
				if a.qrImage != "" {
					ui.Box(c).Fill().Radius(12 * uiScale).PaddingX(16 * uiScale).PaddingY(12 * uiScale).Background(clrCard).Children(func() {
						ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(16 * uiScale).Children(func() {
							// 二维码图（UI 解码 data URI）
							ui.Box(c).Size(120, 120).Draw(func(p *ui.Painter, r ui.Rect) {
								// 背景先画（否则后面 FillRadii 背景色会盖住二维码图）
								p.FillRadii(r, [4]float32{8, 8, 8, 8}, clrCard)
								if img, ok := a.covers["__qr__"]; ok {
									p.Clip(r, 8, func() { p.Image(img, r, ui.Cover) })
								}
							})
							ui.Column(c).Grow(1).Gap(6 * uiScale).Children(func() {
								ui.Text(c, "小米音箱扫码登录").FontSize(14 * uiScale).Bold().TextColor(clrText)
								ui.Text(c, "用米家 App 扫描二维码登录").FontSize(12 * uiScale).TextColor(clrMuted)
								ui.Text(c, a.qrStatus).FontSize(12 * uiScale).TextColor(clrPink)
							})
						})
					})
				}
				ui.Box(c).Fill().Radius(12 * uiScale).PaddingX(16 * uiScale).PaddingY(10 * uiScale).Background(clrCard).Children(func() {
					ui.Column(c).Gap(8 * uiScale).Children(func() {
						ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(10 * uiScale).Children(func() {
							icon(c, icMsg, 18, clrMuted)
							ui.Text(c, "小爱音箱未登录").FontSize(13 * uiScale).TextColor(clrText).Grow(1)
						})
						// token 输入框（对齐主线 speaker_login.html sp-token-input textarea）：
						// 用户手动粘贴/输入 token（多端免扫码复用）；也支持剪贴板一键填入。
						ui.TextInput(c, &a.tokenInput).Grow(1).Height(60 * uiScale).
							Placeholder("粘贴音箱 token（其他设备导出）…").
							FontSize(12 * uiScale).TextColor(clrText)
						ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(10 * uiScale).Children(func() {
							ui.Spacer(c).Grow(1)
							// 读剪贴板填入输入框（快捷）
							if chip(c, "粘贴剪贴板").Clicked() {
								go a.fillTokenFromClipboard()
							}
							// 用输入框 token 登录（对齐主线 textarea 值 → POST /speaker/token/import）
							if gradButton(c, "token 登录").Clicked() {
								go a.loginWithToken(a.tokenInput)
							}
							if gradButton(c, "扫码登录").Clicked() {
								go a.startSpeakerLogin()
							}
						})
					})
				})
			}
			// 音箱设备列表（登录后展示，SSOT speaker）——对齐主线 speaker_devices.html：
			// 网格卡片（图标+名称+硬件型号+在线/离线徽标+「投送当前歌」+「停止」），非单行列表。
			if loggedIn {
				if len(a.speakers) == 0 {
					ui.Text(c, "音箱：点「刷新设备」获取设备列表").FontSize(12 * uiScale).TextColor(clrMuted).PaddingX(4 * uiScale)
				}
				// 网格（2 列等分，对齐主线 grid sm:grid-cols-2）
				ui.Grid(c).ColumnTracks(ui.Fr(1), ui.Fr(1)).Gap(10 * uiScale).Fill().Children(func() {
					for _, sp := range a.speakers {
						sp := sp
						card := ui.Box(c).Fill().Radius(12 * uiScale).PaddingX(14 * uiScale).PaddingY(12 * uiScale).Background(clrCard)
						card.Children(func() {
							ui.Column(c).Fill().Gap(8 * uiScale).Children(func() {
								// 头部：图标 + 名称/硬件 + 在线徽标（对齐主线 sp 卡片头部）
								ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(10 * uiScale).Children(func() {
									icon(c, icVol, 18, clrMuted)
									ui.Column(c).Grow(1).Gap(2 * uiScale).Children(func() {
										ui.Text(c, sp.Name).FontSize(14 * uiScale).Bold().TextColor(clrText).MaxLines(1)
										// 硬件型号（对齐主线 {{.Hardware}}；旧版显示 DID 不对齐）
										ui.Text(c, sp.Hardware).FontSize(11 * uiScale).TextColor(clrMuted).MaxLines(1)
									})
									// 在线/离线徽标（对齐主线 sp 卡片右侧，复用 statusBadge）
									if sp.IsOnline {
										statusBadge(c, "在线", clrGreen)
									} else {
										statusBadge(c, "离线", clrMuted)
									}
								})
								// 按钮行：「投送」（平淡，对齐主线 sp-play-btn）+「停止」
								ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(8 * uiScale).Children(func() {
									if chip(c, "投送").Clicked() {
										go a.playToSpeaker(sp.DID, sp.Name)
									}
									if chip(c, "停止").Clicked() {
										go a.stopSpeaker(sp.DID)
									}
								})
								// 状态标签（对齐主线 speaker.js sp-state：播放中/已暂停/播完自动连播）
								if a.spkDid == sp.DID {
									var label string
									if a.playing {
										label = "▶ 播放中"
										if a.pb.NextBVID() != "" {
											label = "▶ 播放中 · 播完自动连播"
										}
									} else {
										label = "⏸ 已暂停"
									}
									ui.Text(c, label).FontSize(11 * uiScale).TextColor(clrPink).Bold()
								}
							})
						})
					}
				})
			}
			ui.Text(c, "DLNA 设备").FontSize(14 * uiScale).Bold().TextColor(clrText).PaddingX(4 * uiScale)
			if len(a.castDevices) == 0 {
				ui.Text(c, "点「发现设备」搜索同网段 DLNA 播放器/电视").FontSize(12 * uiScale).TextColor(clrMuted).PaddingX(4 * uiScale)
			}
			ui.Column(c).Gap(8 * uiScale).Children(func() {
				for _, d := range a.castDevices {
					d := d
					card := ui.Box(c).Fill().Radius(12 * uiScale).PaddingX(14 * uiScale).PaddingY(12 * uiScale).Background(clrCard)
					if card.Hovered() {
						card.Background(clrCardHov)
					}
					card.Children(func() {
						ui.Column(c).Fill().Gap(8 * uiScale).Children(func() {
							// 头部：图标 + 名称/IP + 状态徽标（对齐主线 cast-status span：在线/真实状态）
							ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(10 * uiScale).Children(func() {
								icon(c, icCast, 18, clrMuted)
								ui.Column(c).Grow(1).Gap(2 * uiScale).Children(func() {
									ui.Text(c, d.Name).FontSize(14 * uiScale).Bold().TextColor(clrText).MaxLines(1)
									ui.Text(c, d.IP+":"+fmt.Sprintf("%d", d.Port)).FontSize(11 * uiScale).TextColor(clrMuted).MaxLines(1)
								})
								stateText, ok := a.castStatus[d.UDN]
								if !ok || stateText == "" {
									stateText = "在线"
								}
								statusBadge(c, stateText, clrGreen) // 复用状态徽标（原纯文字）
							})
							// 按钮行：「查状态」+「投送」（对齐主线 hx/cast/status + cast-play-btn）
							ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(8 * uiScale).Children(func() {
								if chip(c, "查状态").Clicked() {
									go a.queryCastStatus(d.UDN)
								}
								if chip(c, "投送").Clicked() {
									go a.castTo(d.UDN, d.Name)
								}
							})
						})
					})
				}
			})
		})
	})
}

// startSpeakerLogin 生成音箱登录二维码（SSOT：xiaoai QRLogin）+ 轮询状态。
func (a *app) startSpeakerLogin() {
	if a.svc == nil || a.svc.qr == nil {
		return
	}
	res, err := a.svc.qr.GenerateQRCode()
	if err != nil {
		a.win.Update(func() { a.qrStatus = "二维码生成失败：" + err.Error() })
		return
	}
	a.win.Update(func() {
		qri, _ := res["qr_image"].(string)
		a.qrImage = qri
		if qri != "" {
			// qr_image 是 "data:image/png;base64,..."（xiaoai qrcode.go 编码），
			// DecodeImage 只收原始 bytes，必须先去掉 data URI 前缀 + base64 解码，
			// 否则解码失败 → 二维码不显示。
			raw := []byte(qri)
			if i := strings.Index(qri, ","); i >= 0 {
				if b, derr := base64.StdEncoding.DecodeString(qri[i+1:]); derr == nil {
					raw = b
				}
			}
			if img, derr := ui.DecodeImage(raw); derr == nil {
				a.covers["__qr__"] = img
			} else {
				a.qrStatus = "二维码解码失败：" + derr.Error()
			}
		}
		a.qrStatus = "等待扫码…"
		a.win.Invalidate()
	})
	// 轮询（CheckStatus 长轮询 120s）；成功后刷新设备
	go func() {
		for {
			st, err := a.svc.qr.CheckStatus()
			if err != nil {
				a.win.Update(func() { a.qrStatus = "登录失败：" + err.Error() })
				return
			}
			status, _ := st["status"].(string)
			msg, _ := st["message"].(string)
			a.win.Update(func() { a.qrStatus = msg })
			switch status {
			case "success":
				// 把 QR token 灌进 speaker auth（对齐主线 hx/speaker.go applyQRToken）
				a.svc.applyQRToken()
				a.win.Update(func() { a.qrStatus = "登录成功，正在刷新设备…" })
				a.discoverCast()
				// 已登录 → 刷新音箱设备
				if a.svc.speakerLoggedIn() {
					go a.refreshSpeakers()
				}
				return
			case "failed":
				return
			}
		}
	}()
}

// importTokenFromClipboard 从剪贴板读 token 粘贴登录（对齐主线 speaker_login.html
// 的 sp-token-input 粘贴入口 + POST /api/v1/speaker/token/import）。
// fillTokenFromClipboard 读剪贴板填入 token 输入框（不登录，用户可编辑后再登录）。
func (a *app) fillTokenFromClipboard() {
	if a.svc == nil || a.svc.qrAuth == nil {
		return
	}
	data, err := mygo.Clipboard.Read(transfer.Text)
	if err != nil {
		a.win.Update(func() { a.status = "读取剪贴板失败：" + err.Error() })
		return
	}
	text, _ := data.Read(transfer.Text)
	tok := strings.TrimSpace(string(text))
	a.win.Update(func() {
		a.tokenInput = tok
		if tok == "" {
			a.status = "剪贴板为空，请先复制音箱 token"
		}
		a.win.Invalidate()
	})
}

// loginWithToken 用输入的 token 登录（对齐主线 sp-token-input textarea 值 → import）。
func (a *app) loginWithToken(tok string) {
	if a.svc == nil || a.svc.qrAuth == nil {
		return
	}
	tok = strings.TrimSpace(tok)
	if tok == "" {
		a.win.Update(func() { a.status = "请输入音箱 token" })
		a.win.Invalidate()
		return
	}
	if err := a.svc.importSpeakerToken(tok); err != nil {
		a.win.Update(func() { a.status = err.Error() })
		a.win.Invalidate()
		return
	}
	a.win.Update(func() { a.status = "token 登录成功，正在刷新设备…" })
	go a.refreshSpeakers()
	a.win.Invalidate()
}

// refreshSpeakers 刷新小米音箱设备列表（speaker 域包 ListDevices）。
func (a *app) refreshSpeakers() {
	if a.svc == nil || a.svc.speaker == nil || !a.svc.speakerLoggedIn() {
		return
	}
	list := a.svc.speaker.ListDevices() // SpeakerService.ListDevices 无错误返回（auth 层处理）
	a.win.Update(func() {
		a.speakers = list
		a.win.Invalidate()
	})
}

// discoverCast 发现 DLNA 设备（cast 域包 Discover，后台执行）。
func (a *app) discoverCast() {
	if a.svc == nil || a.svc.cast == nil {
		return
	}
	_ = a.svc.cast.Discover()
	list := a.svc.cast.DeviceList()
	a.win.Update(func() {
		a.castDevices = list
		// 小米音箱设备也拉一次（登录后才有）
		if a.svc.speaker != nil {
			a.speakers = a.svc.speaker.ListDevices()
		}
		a.win.Invalidate()
	})
}

// castTo 投当前播放到指定 DLNA 设备（cast 域包 Cast）。
// queryCastStatus 查 DLNA 设备播放状态（对齐主线 hx/cast/status：cast.Status(udn) → state）。
func (a *app) queryCastStatus(udn string) {
	if a.svc == nil || a.svc.cast == nil {
		return
	}
	status, err := a.svc.cast.Status(udn)
	if err != nil || status == nil {
		a.win.Update(func() { a.castStatus[udn] = "状态不可用" })
		return
	}
	state, _ := status["state"].(string)
	if state == "" {
		state = "就绪"
	}
	a.win.Update(func() {
		if a.castStatus == nil {
			a.castStatus = map[string]string{}
		}
		a.castStatus[udn] = state
	})
}

func (a *app) castTo(udn, name string) {
	if a.svc == nil || a.svc.cast == nil || a.current == nil {
		return
	}
	url := "https://www.bilibili.com/video/" + a.current.bvid
	res, err := a.svc.cast.Cast(url, udn, a.current.title)
	if err != nil {
		a.status = "投屏失败：" + err.Error()
		a.win.Invalidate()
		return
	}
	if code, _ := res["code"].(int); code == 0 {
		a.castOn = name
		a.status = "已投屏到 " + name
	} else {
		if msg, _ := res["message"].(string); msg != "" {
			a.status = "投屏失败：" + msg
		}
	}
	a.win.Invalidate()
}

// stopCast 停止投屏（DLNA Stop 动作）。
func (a *app) stopCast() {
	if a.svc == nil || a.svc.cast == nil {
		return
	}
	for _, d := range a.castDevices {
		a.svc.cast.Control(d.UDN, "stop", "", 0)
	}
	a.castOn = ""
	a.status = "已停止投屏"
	a.win.Invalidate()
}

// playToSpeaker 播放当前歌到小米音箱（SSOT speaker.Play：B站 bvid → DASH → 代理 URL）。
func (a *app) playToSpeaker(did, name string) {
	if a.svc == nil || a.svc.speaker == nil || a.current == nil {
		return
	}
	// 自动连播：NextBVID 非空 = 音箱播完由 speaker 域包定时器触发 hook 切下一首
	// （对齐主线 hx/speaker.go：next 为空则 NextBVID() 取核心队列下一首；
	// 核心队列 SSOT 算 next，不依赖前端传参）。
	opts := speaker.PlayOpts{Title: a.current.title, Duration: 0, NextBVID: a.pb.NextBVID()}
	// 音质 mock：主线 hx/speaker.go 固定 30280（= bilibili.Audio192K，B站 qn 码，
	// 不是"192k"这个字面量）；旧版传 192 会走 GetBestAudioURL 回退链才拿到 30280。
	qn := bilibili.Audio192K
	if a.quality == 0 {
		qn = bilibili.Audio64K
	} else if a.quality == 1 {
		qn = bilibili.Audio128K
	}
	if res, err := a.svc.speaker.Play(a.current.bvid, did, qn, opts); err != nil {
		a.status = "音箱播放失败：" + err.Error()
	} else if code, _ := res["code"].(int); code == 0 {
		a.castOn = name
		a.status = "已在音箱播放：" + name
		// 投送成功 → 同步切核心 source=speaker（对齐主线 hx/speaker.go 注释：
		// “消除双响窗口——分两步时前端在‘音箱已响但核心还是 web/playing’的间隙
		// 让手机也开播”）。SwitchSpeaker 内 prevOut(本地 MyGoOutput).Stop() 停本地
		// 出声 + 状态切音箱 + SpeakerOutput 接管进度投影。缺它：音箱在响核心却说
		// web/playing，本地也在播 = 真双响。
		a.pb.SwitchSpeaker(did)
	}
	a.win.Invalidate()
}

// stopSpeaker 停止音箱投送（对齐主线 stop_speaker 命令 → playback.StopSpeaker 唯一实现：
// 音箱停 + source 切回 web；castOn 清空回“未投屏”态）。
func (a *app) stopSpeaker(did string) {
	if a.svc == nil || a.svc.speaker == nil {
		return
	}
	a.pb.StopSpeaker(did)
	a.castOn = ""
	a.status = "已停止投屏"
	a.win.Invalidate()
}

// openCtxMenu 打开右键菜单（对齐主线 contextmenu：点某行）
