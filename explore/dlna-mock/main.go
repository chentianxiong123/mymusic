// dlna-mock —— 本地假 DLNA MediaRenderer，用于验证 mygo 的发现 + 投送链路。
// 启动后监听 SSDP 239.255.255.250:1900 应答 M-SEARCH，HTTP 提供设备描述 +
// AVTransport/RenderingControl SOAP 服务，把收到的 SetAVTransportURI 打印出来。
package main

import (
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
)

var (
	mu       sync.Mutex
	lastURI  string
	lastTitle string
	playing  bool
)

func main() {
	port := 38900
	if len(os.Args) > 1 {
		fmt.Sscanf(os.Args[1], "%d", &port)
	}
	host, _ := os.Hostname()
	ip := localIP()
	if adv := os.Getenv("MOCK_ADVERTISE_IP"); adv != "" {
		ip = adv // NAT 场景：对外广播一个 NAT 可达地址
	}
	udn := "uuid:mymusic-dlna-mock-0001"
	base := fmt.Sprintf("http://%s:%d", ip, port)

	// --- 设备描述 XML ---
	mux := http.NewServeMux()
	mux.HandleFunc("/description.xml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/xml")
		fmt.Fprintf(w, `<?xml version="1.0"?>
<root xmlns="urn:schemas-upnp-org:device-1-0">
<specVersion><major>1</major><minor>0</minor></specVersion>
<URLBase>%s</URLBase>
<device>
<deviceType>urn:schemas-upnp-org:device:MediaRenderer:1</deviceType>
<friendlyName>Mock DLNA Renderer</friendlyName>
<manufacturer>mygo-mock</manufacturer>
<modelName>DLNA Mock</modelName>
<UDN>%s</UDN>
<serviceList>
<service>
<serviceType>urn:schemas-upnp-org:service:AVTransport:1</serviceType>
<serviceId>urn:upnp-org:serviceId:AVTransport</serviceId>
<SCPDURL>/avt.xml</SCPDURL>
<controlURL>/avt/control</controlURL>
<eventSubURL>/avt/event</eventSubURL>
</service>
<service>
<serviceType>urn:schemas-upnp-org:service:RenderingControl:1</serviceType>
<serviceId>urn:upnp-org:serviceId:RenderingControl</serviceId>
<SCPDURL>/rc.xml</SCPDURL>
<controlURL>/rc/control</controlURL>
<eventSubURL>/rc/event</eventSubURL>
</service>
</serviceList>
</device>
</root>`, base, udn)
	})
	// goupnp av1 客户端会拉 SCPDURL，给最小有效 SCPD
	scpd := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/xml")
		fmt.Fprint(w, `<?xml version="1.0"?>
<scpd xmlns="urn:schemas-upnp-org:service-1-0">
<specVersion><major>1</major><minor>0</minor></specVersion>
<actionList>
<action><name>SetAVTransportURI</name><argumentList>
<argument><name>InstanceID</name><direction>in</direction><relatedStateVariable>InstanceID</relatedStateVariable></argument>
<argument><name>CurrentURI</name><direction>in</direction><relatedStateVariable>CurrentURI</relatedStateVariable></argument>
<argument><name>CurrentURIMetaData</name><direction>in</direction><relatedStateVariable>CurrentURIMetaData</relatedStateVariable></argument>
</argumentList></action>
<action><name>Play</name><argumentList>
<argument><name>InstanceID</name><direction>in</direction><relatedStateVariable>InstanceID</relatedStateVariable></argument>
<argument><name>Speed</name><direction>in</direction><relatedStateVariable>TransportPlaySpeed</relatedStateVariable></argument>
</argumentList></action>
<action><name>Stop</name><argumentList>
<argument><name>InstanceID</name><direction>in</direction><relatedStateVariable>InstanceID</relatedStateVariable></argument>
</argumentList></action>
<action><name>GetTransportInfo</name><argumentList>
<argument><name>InstanceID</name><direction>in</direction><relatedStateVariable>InstanceID</relatedStateVariable></argument>
<argument><name>CurrentTransportState</name><direction>out</direction><relatedStateVariable>CurrentTransportState</relatedStateVariable></argument>
</argumentList></action>
</actionList>
<serviceStateTable>
<stateVariable sendEvents="no"><name>InstanceID</name><dataType>ui4</dataType></stateVariable>
<stateVariable sendEvents="no"><name>CurrentURI</name><dataType>string</dataType></stateVariable>
<stateVariable sendEvents="no"><name>CurrentURIMetaData</name><dataType>string</dataType></stateVariable>
<stateVariable sendEvents="no"><name>TransportPlaySpeed</name><dataType>string</dataType></stateVariable>
<stateVariable sendEvents="no"><name>CurrentTransportState</name><dataType>string</dataType></stateVariable>
</serviceStateTable>
</scpd>`)
	}
	mux.HandleFunc("/avt.xml", scpd)
	mux.HandleFunc("/rc.xml", scpd)

	// --- SOAP 控制 ---
	soap := func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		action := r.Header.Get("SOAPACTION")
		mu.Lock()
		switch {
		case strings.Contains(action, "SetAVTransportURI"):
			// 提取当前 URI（XML 转义后）
			var v struct {
				URI string `xml:"CurrentURI"`
			}
			_ = xml.Unmarshal([]byte(stripSOAP(string(body))), &v)
			lastURI = v.URI
			lastTitle = extractTitle(string(body))
			log.Printf("[SOAP] SetAVTransportURI uri=%s title=%s", lastURI, lastTitle)
		case strings.Contains(action, "Play"):
			playing = true
			log.Printf("[SOAP] Play -> 开始播放 uri=%s", lastURI)
		case strings.Contains(action, "Stop"):
			playing = false
			log.Printf("[SOAP] Stop")
		case strings.Contains(action, "GetTransportInfo"):
			st := "STOPPED"
			if playing { st = "PLAYING" }
			log.Printf("[SOAP] GetTransportInfo -> %s", st)
			w.Header().Set("Content-Type", `text/xml; charset="utf-8"`)
			fmt.Fprintf(w, `<?xml version="1.0"?><s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body><u:GetTransportInfoResponse xmlns:u="urn:schemas-upnp-org:service:AVTransport:1"><CurrentTransportState>%s</CurrentTransportState></u:GetTransportInfoResponse></s:Body></s:Envelope>`, st)
			mu.Unlock()
			return
		default:
			log.Printf("[SOAP] unknown action: %s", action)
		}
		mu.Unlock()
		w.Header().Set("Content-Type", `text/xml; charset="utf-8"`)
		fmt.Fprint(w, `<?xml version="1.0"?><s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body><u:Response xmlns:u="urn:schemas-upnp-org:service:AVTransport:1"/></s:Body></s:Envelope>`)
	}
	mux.HandleFunc("/avt/control", soap)
	mux.HandleFunc("/rc/control", soap)
	mux.HandleFunc("/state", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock(); defer mu.Unlock()
		fmt.Fprintf(w, "uri=%s\ntitle=%s\nplaying=%v\n", lastURI, lastTitle, playing)
	})

	// --- SSDP 应答 ---
	go func() {
		// SSDP：nil 接口 = 操作系统选默认多播接口（Windows 会加入所有 UP 网卡）
		addr, _ := net.ResolveUDPAddr("udp", "239.255.255.250:1900")
		log.Printf("SSDP interface: (all, nil)")
		conn, err := net.ListenMulticastUDP("udp", nil, addr)
		if err != nil { log.Printf("SSDP listen: %v", err); return }
		defer conn.Close()
		conn.SetReadBuffer(1 << 20)
		buf := make([]byte, 2048)
		log.Printf("SSDP listening on %v", addr)
		for {
			n, from, err := conn.ReadFromUDP(buf)
			if err != nil { continue }
			msg := string(buf[:n])
			if !strings.HasPrefix(msg, "M-SEARCH") { continue }
			st := header(msg, "ST")
			// 三种 ST 全应答（mygo 搜三类）
			types := []string{
				"urn:schemas-upnp-org:device:MediaRenderer:1",
				"urn:schemas-upnp-org:service:AVTransport:1",
				"upnp:rootdevice",
			}
			if st != "ssdp:all" {
				found := false
				for _, t := range types { if t == st { found = true } }
				if !found { continue }
			}
			resp := fmt.Sprintf("HTTP/1.1 200 OK\r\n"+
				"CACHE-CONTROL: max-age=1800\r\n"+
				"LOCATION: %s/description.xml\r\n"+
				"SERVER: MockDLNA/1.0\r\n"+
				"ST: %s\r\n"+
				"USN: %s::%s\r\n"+
				"\r\n", base, st, udn, st)
			_, _ = conn.WriteToUDP([]byte(resp), from)
			log.Printf("SSDP answered %s -> %v", st, from)
		}
	}()

	log.Printf("Mock DLNA Renderer: %s (ip=%s port=%d)", base, host, port)
	log.Printf("state endpoint: %s/state", base)
	log.Fatal(http.ListenAndServe(fmt.Sprintf("0.0.0.0:%d", port), mux))
}

func header(msg, key string) string {
	for _, line := range strings.Split(msg, "\n") {
		if i := strings.Index(strings.ToLower(line), strings.ToLower(key)+":"); i >= 0 {
			return strings.TrimSpace(line[i+len(key)+1:])
		}
	}
	return ""
}

func stripSOAP(s string) string {
	i := strings.Index(s, "<u:"); if i < 0 { i = strings.Index(s, "<SetAVTransportURI") }
	if i < 0 { return s }
	return s[i:]
}

func extractTitle(s string) string {
	i := strings.Index(s, "&lt;title&gt;")
	if i < 0 { return "" }
	j := strings.Index(s[i:], "&lt;/title&gt;")
	if j < 0 { return "" }
	return s[i+len("&lt;title&gt;"):i+j]
}

func localIP() string {
	if n := os.Getenv("MOCK_IFACE"); n != "" {
		if iface, err := net.InterfaceByName(n); err == nil {
			if addrs, _ := iface.Addrs(); len(addrs) > 0 {
				for _, a := range addrs {
					if ipnet, ok := a.(*net.IPNet); ok && ipnet.IP.To4() != nil {
						return ipnet.IP.To4().String()
					}
				}
			}
		}
	}
	ifaces, _ := net.Interfaces()
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 { continue }
		addrs, _ := iface.Addrs()
		for _, a := range addrs {
			if ipnet, ok := a.(*net.IPNet); ok && !ipnet.IP.IsLoopback() && ipnet.IP.To4() != nil {
				return ipnet.IP.To4().String()
			}
		}
	}
	return "127.0.0.1"
}

func ifaceName() string {
	// 环境变量指定网卡名（跨主机测试时选与对端同网段的接口）
	if n := os.Getenv("MOCK_IFACE"); n != "" {
		return n
	}
	ifaces, _ := net.Interfaces()
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 { continue }
		addrs, _ := iface.Addrs()
		for _, a := range addrs {
			if ipnet, ok := a.(*net.IPNet); ok && ipnet.IP.To4() != nil && !ipnet.IP.IsLoopback() {
				return iface.Name
			}
		}
	}
	return ""
}
