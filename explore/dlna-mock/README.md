# dlna-mock

最小 DLNA MediaRenderer 模拟器，用于验证 mygo 的 DLNA **发现（SSDP）** 和
**投送（SOAP）** 链路，不需要真实电视/音箱。

## 能力

- SSDP：`M-SEARCH` 应答三种 ST（MediaRenderer / AVTransport / rootdevice）
- HTTP：设备描述 XML + AVTransport/RenderingControl SCPD + SOAP 控制端点
- `GET /state`：查看收到的 URI / 标题 / 播放状态
- 收到 `SetAVTransportURI` / `Play` / `Stop` 会打日志

## 用法

```bash
go build -o dlna-mock .
./dlna-mock 38900
```

环境变量（跨机/NAT 场景）：

| 变量 | 作用 |
|---|---|
| `MOCK_ADVERTISE_IP` | 对外广播的 IP（NAT 内机器用 NAT 映射后的可达地址） |
| `MOCK_IFACE` | SSDP 加入多播组的网卡名（多网卡机器用） |

## 已知限制

- **SSDP 多播必须同二层网段**。跨 NAT / 不同网段时，`dlna.Search()` 收不到
  应答——这不是 mock 或 mygo 的问题，是网络拓扑限制。跨机验证投送链路时，
  可以绕过发现：把设备直接注入 `CastService.devices`，用 mock 广播的可达
  HTTP 地址调 `SetURL()`。
- 只实现了 `SetAVTransportURI` / `Play` / `Stop` / `GetTransportInfo`；
  `GetPositionInfo` / `GetVolume` 等返回空响应（不影响投送验证）。
