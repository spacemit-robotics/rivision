// Package sip implements SIP protocol for GB28181
package sip

import (
	"bufio"
	"crypto/md5"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

const (
	UserAgent = "gb28181-sim/1.0"

	// GB28181 Payload Types (RFC 3551 / GB28181-2015)
	PayloadTypePS   = 96 // PS (Program Stream)
	PayloadTypeH264 = 98  // H.264 ES
	PayloadTypeH265 = 99  // H.265 ES (HEVC)
)

// Config holds SIP client configuration
type Config struct {
	LocalIP      string
	LocalPort    int
	ServerIP     string
	ServerID     string
	Domain       string
	AgentID      string
	Password     string
	Channels     []string           // Multiple channel IDs
	ChannelNames map[string]string  // channelID -> display name
	UseTCP       bool
	Conn         net.Conn
}

// Client is a SIP client for GB28181
type Client struct {
cfg    Config
reader *bufio.Reader
cseq   int
}

// Message represents a SIP message
type Message struct {
Method     string
StatusCode int
Headers    map[string]string
Body       string
Raw        string
}

// SDP holds parsed SDP information (符合 GB28181-2015)
type SDP struct {
	IP          string
	Port        int
	PayloadType int    // RTP Payload Type
	UseTCP      bool
	SSRC        uint32
	Codec       string
	// GB28181 扩展字段
	ClockRate   int    // 时钟频率 (通常 90000)
	Format      string // 编码格式 (PS/H.264/H.265)
}

// GB28181 Payload Type 映射
var PTMap = map[int]string{
	96: "PS",    // Program Stream (GB28181 主要格式)
	97: "MPEG4", // MPEG-4
	98: "H264",  // H.264 Elementary Stream
	99: "H265",   // H.265
}

// GB28181 Codec 名称映射
var CodecMap = map[string]string{
	"PS":    "PS/90000",
	"H264":  "H264/90000",
	"H265":  "H265/90000",
	"MPEG4": "MPEG4/90000",
}

// ForceUDPMode 强制使用 UDP 模式
// FFmpeg 不直接支持 RFC 4571 TCP 传输，UDP 是最可靠的方案
const ForceUDPMode = true

// NewClient creates a new SIP client
func NewClient(cfg Config) *Client {
return &Client{
cfg:    cfg,
reader: bufio.NewReader(cfg.Conn),
cseq:   1,
}
}

// Register performs SIP REGISTER
func (c *Client) Register() error {
// First REGISTER (no auth)
c.send(c.buildRegister(c.cseq, ""))
c.cseq++

resp, err := c.Receive()
if err != nil {
return err
}

if resp.StatusCode == 401 {
// Parse WWW-Authenticate
authHeader := resp.Headers["WWW-Authenticate"]
nonce := extractQuoted(authHeader, "nonce")
realm := extractQuoted(authHeader, "realm")
qop := extractValue(authHeader, "qop")

// Build auth response
auth := c.buildAuthHeader(nonce, realm, qop, "REGISTER", fmt.Sprintf("sip:%s", c.cfg.Domain))

c.send(c.buildRegister(c.cseq, auth))
c.cseq++

resp, err = c.Receive()
if err != nil {
return err
}
}

if resp.StatusCode != 200 {
return fmt.Errorf("REGISTER failed: %d", resp.StatusCode)
}

logrus.Info("REGISTER successful")

// Send initial messages (catalog is omitted: platform queries Catalog after register)
c.sendDeviceInfo()
c.SendKeepalive(1)

return nil
}

// Receive reads and parses a SIP message
func (c *Client) Receive() (*Message, error) {
var lines []string
contentLength := 0

// Read headers
for {
line, err := c.reader.ReadString('\n')
if err != nil {
return nil, err
}
line = strings.TrimRight(line, "\r\n")
if line == "" {
break
}
lines = append(lines, line)

if strings.HasPrefix(strings.ToLower(line), "content-length:") {
fmt.Sscanf(line[15:], "%d", &contentLength)
}
}

// Read body
body := ""
if contentLength > 0 {
buf := make([]byte, contentLength)
_, err := c.reader.Read(buf)
if err != nil {
return nil, err
}
body = string(buf)
}

return parseMessage(lines, body), nil
}

// SendOK sends a 200 OK response
func (c *Client) SendOK(req *Message) {
hdrs := []string{
fmt.Sprintf("Via: %s", req.Headers["Via"]),
fmt.Sprintf("From: %s", req.Headers["From"]),
fmt.Sprintf("To: %s", req.Headers["To"]),
fmt.Sprintf("Call-ID: %s", req.Headers["Call-ID"]),
fmt.Sprintf("CSeq: %s", req.Headers["CSeq"]),
fmt.Sprintf("Contact: <sip:%s@%s:%d>", c.cfg.AgentID, c.cfg.LocalIP, c.cfg.LocalPort),
fmt.Sprintf("User-Agent: %s", UserAgent),
}
c.send(buildSIP("SIP/2.0 200 OK", hdrs, ""))
}

// SendTrying sends a 100 Trying response
func (c *Client) SendTrying(req *Message) {
hdrs := []string{
fmt.Sprintf("Via: %s", req.Headers["Via"]),
fmt.Sprintf("From: %s", req.Headers["From"]),
fmt.Sprintf("To: %s", req.Headers["To"]),
fmt.Sprintf("Call-ID: %s", req.Headers["Call-ID"]),
fmt.Sprintf("CSeq: %s", req.Headers["CSeq"]),
}
c.send(buildSIP("SIP/2.0 100 Trying", hdrs, ""))
}

// SendInviteOK sends a 200 OK with SDP for INVITE (符合 GB28181-2015)
func (c *Client) SendInviteOK(req *Message, sdp *SDP) {
	to := req.Headers["To"]
	if !strings.Contains(to, "tag=") {
		to += ";tag=ok"
	}

	protocol := "RTP/AVP"

	// 优先使用请求中指定的 PayloadType（来自 owl 协商）：
	// PT=99 → H.265 ES; PT=98 → H.264 ES; 其他默认 H.264
	pt := sdp.PayloadType
	if pt != PayloadTypeH264 && pt != PayloadTypeH265 {
		pt = PayloadTypeH264
	}

	var codecProfile, fmtpLine string
	if pt == PayloadTypeH265 {
		codecProfile = "H265/90000"
		// GB28181 H.265 fmtp（无 packetization-mode，HEVC 不需要）
		fmtpLine = fmt.Sprintf("a=fmtp:%d profile-space=0;tier-flag=0;level-id=93;profile-id=1\r\n", PayloadTypeH265)
	} else {
		codecProfile = "H264/90000"
		// H.264: Main/L4.2，无 B 帧重编码时 profile-level-id=4D002A
		fmtpLine = fmt.Sprintf("a=fmtp:%d packetization-mode=1;profile-level-id=4D002A\r\n", PayloadTypeH264)
	}

	// 构建 m= line 使用实际协商的 PT
	sdpBody := fmt.Sprintf(
		"v=0\r\n"+
			"o=%s 0 0 IN IP4 %s\r\n"+
			"s=Play\r\n"+
			"c=IN IP4 %s\r\n"+
			"t=0 0\r\n"+
			"m=video %d %s %d\r\n"+
			"a=sendonly\r\n"+
			"a=rtpmap:%d %s\r\n"+
			"%s",
		c.cfg.AgentID, c.cfg.LocalIP,
		sdp.IP,
		sdp.Port, protocol, pt,
		pt, codecProfile,
		fmtpLine,
	)

	// GB28181 SSRC (10位数字，y=字段)
	ssrc := sdp.SSRC
	if ssrc == 0 {
		ssrc = uint32(time.Now().UnixNano() % 10000000000)
	}
	sdpBody += fmt.Sprintf("y=%010d\r\n", ssrc)

	hdrs := []string{
		fmt.Sprintf("Via: %s", req.Headers["Via"]),
		fmt.Sprintf("From: %s", req.Headers["From"]),
		fmt.Sprintf("To: %s", to),
		fmt.Sprintf("Call-ID: %s", req.Headers["Call-ID"]),
		fmt.Sprintf("CSeq: %s", req.Headers["CSeq"]),
		fmt.Sprintf("Contact: <sip:%s@%s:%d>", c.cfg.AgentID, c.cfg.LocalIP, c.cfg.LocalPort),
		"Content-Type: application/sdp",
		fmt.Sprintf("User-Agent: %s", UserAgent),
	}
	c.send(buildSIP("SIP/2.0 200 OK", hdrs, sdpBody))
}

// SendSubscribeOK sends a 200 OK for SUBSCRIBE
func (c *Client) SendSubscribeOK(req *Message) {
to := req.Headers["To"]
if !strings.Contains(to, "tag=") {
to += fmt.Sprintf(";tag=%d", time.Now().UnixNano())
}

hdrs := []string{
fmt.Sprintf("Via: %s", req.Headers["Via"]),
fmt.Sprintf("From: %s", req.Headers["From"]),
fmt.Sprintf("To: %s", to),
fmt.Sprintf("Call-ID: %s", req.Headers["Call-ID"]),
fmt.Sprintf("CSeq: %s", req.Headers["CSeq"]),
fmt.Sprintf("Contact: <sip:%s@%s:%d>", c.cfg.AgentID, c.cfg.LocalIP, c.cfg.LocalPort),
"Expires: 600",
fmt.Sprintf("User-Agent: %s", UserAgent),
}
c.send(buildSIP("SIP/2.0 200 OK", hdrs, ""))
}

// SendKeepalive sends a keepalive MESSAGE
func (c *Client) SendKeepalive(seq int) error {
xml := fmt.Sprintf(
"<?xml version='1.0' encoding='GB2312'?><Notify><CmdType>Keepalive</CmdType>"+
"<SN>%d</SN><DeviceID>%s</DeviceID><Status>OK</Status></Notify>",
seq, c.cfg.AgentID,
)
c.sendMessage(xml, seq, "keep")
return nil
}

// HandleQuery handles MESSAGE queries
func (c *Client) HandleQuery(msg *Message) {
if strings.Contains(msg.Body, "<Query>") {
cmdRe := regexp.MustCompile(`<CmdType>(.+?)</CmdType>`)
snRe := regexp.MustCompile(`<SN>(\d+)</SN>`)

cmdMatch := cmdRe.FindStringSubmatch(msg.Body)
snMatch := snRe.FindStringSubmatch(msg.Body)

if len(cmdMatch) > 1 && len(snMatch) > 1 {
cmd := cmdMatch[1]
sn, _ := strconv.Atoi(snMatch[1])

switch cmd {
case "Catalog":
c.sendCatalog(sn)
case "DeviceInfo":
c.sendDeviceInfo()
}
}
}
}

func (c *Client) sendDeviceInfo() {
xml := fmt.Sprintf(
"<?xml version='1.0' encoding='GB2312'?><Response>"+
"<CmdType>DeviceInfo</CmdType><SN>1</SN><DeviceID>%s</DeviceID>"+
"<DeviceName>GB28181-Sim</DeviceName><Manufacturer>RiVision</Manufacturer>"+
"<Model>v1</Model><Firmware>1.0</Firmware><Result>OK</Result></Response>",
c.cfg.AgentID,
)
c.sendMessage(xml, 2, "info")
}

func (c *Client) sendCatalog(sn int) {
	// Build catalog with all channels
	var items strings.Builder
	for _, chID := range c.cfg.Channels {
		chName := chID
		if name, ok := c.cfg.ChannelNames[chID]; ok && name != "" {
			chName = name
		}
		items.WriteString(fmt.Sprintf(
			"<Item><DeviceID>%s</DeviceID><Name>%s</Name>"+
				"<Manufacturer>RiVision</Manufacturer><Model>v1</Model>"+
				"<Status>ON</Status></Item>",
			chID, chName,
		))
	}

xml := fmt.Sprintf(
"<?xml version='1.0' encoding='GB2312'?><Response><CmdType>Catalog</CmdType>"+
"<SN>%d</SN><DeviceID>%s</DeviceID><SumNum>%d</SumNum><DeviceList>%s</DeviceList></Response>",
sn, c.cfg.AgentID, len(c.cfg.Channels), items.String(),
)
c.sendMessage(xml, 3, "cat")
}

func (c *Client) sendMessage(xml string, cseq int, suffix string) {
protocol := "UDP"
if c.cfg.UseTCP {
protocol = "TCP"
}

hdrs := []string{
fmt.Sprintf("Via: SIP/2.0/%s %s:%d;branch=z9hG4bK%d", protocol, c.cfg.LocalIP, c.cfg.LocalPort, time.Now().UnixNano()),
fmt.Sprintf("From: <sip:%s@%s>;tag=msg", c.cfg.AgentID, c.cfg.Domain),
fmt.Sprintf("To: <sip:%s@%s>", c.cfg.ServerID, c.cfg.Domain),
fmt.Sprintf("Call-ID: %s%s", c.cfg.AgentID, suffix),
fmt.Sprintf("CSeq: %d MESSAGE", cseq),
"Content-Type: Application/MANSCDP+xml",
"Max-Forwards: 70",
fmt.Sprintf("User-Agent: %s", UserAgent),
}
c.send(buildSIP(fmt.Sprintf("MESSAGE sip:%s@%s SIP/2.0", c.cfg.ServerID, c.cfg.Domain), hdrs, xml))
}

func (c *Client) buildRegister(cseq int, auth string) []byte {
protocol := "UDP"
if c.cfg.UseTCP {
protocol = "TCP"
}

hdrs := []string{
fmt.Sprintf("Via: SIP/2.0/%s %s:%d;branch=z9hG4bK%d", protocol, c.cfg.LocalIP, c.cfg.LocalPort, time.Now().UnixNano()),
fmt.Sprintf("From: <sip:%s@%s>;tag=reg", c.cfg.AgentID, c.cfg.Domain),
fmt.Sprintf("To: <sip:%s@%s>", c.cfg.AgentID, c.cfg.Domain),
fmt.Sprintf("Call-ID: %s", c.cfg.AgentID),
fmt.Sprintf("CSeq: %d REGISTER", cseq),
fmt.Sprintf("Contact: <sip:%s@%s:%d>", c.cfg.AgentID, c.cfg.LocalIP, c.cfg.LocalPort),
"Max-Forwards: 70",
fmt.Sprintf("User-Agent: %s", UserAgent),
"Expires: 3600",
}
if auth != "" {
hdrs = append(hdrs, fmt.Sprintf("Authorization: %s", auth))
}
return buildSIP(fmt.Sprintf("REGISTER sip:%s SIP/2.0", c.cfg.Domain), hdrs, "")
}

func (c *Client) buildAuthHeader(nonce, realm, qop, method, uri string) string {
a1 := md5hex(fmt.Sprintf("%s:%s:%s", c.cfg.AgentID, realm, c.cfg.Password))
a2 := md5hex(fmt.Sprintf("%s:%s", method, uri))

if qop != "" {
nc := "00000001"
cnonce := fmt.Sprintf("%06x", time.Now().UnixNano()&0xFFFFFF)
response := md5hex(fmt.Sprintf("%s:%s:%s:%s:%s:%s", a1, nonce, nc, cnonce, qop, a2))
return fmt.Sprintf(
`Digest username="%s", realm="%s", nonce="%s", uri="%s", response="%s", algorithm=MD5, qop=%s, nc=%s, cnonce="%s"`,
c.cfg.AgentID, realm, nonce, uri, response, qop, nc, cnonce,
)
}

response := md5hex(fmt.Sprintf("%s:%s:%s", a1, nonce, a2))
return fmt.Sprintf(
`Digest username="%s", realm="%s", nonce="%s", uri="%s", response="%s", algorithm=MD5`,
c.cfg.AgentID, realm, nonce, uri, response,
)
}

func (c *Client) send(data []byte) {
logrus.Debugf("SIP TX:\n%s", string(data))
c.cfg.Conn.Write(data)
}

// ParseSDP parses SDP from message body (符合 GB28181-2015)
func ParseSDP(body string) (*SDP, error) {
	sdp := &SDP{
		PayloadType: 96, // 默认 PS
		Codec:       "PS",
		ClockRate:   90000,
	}

	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "c=IN IP4 "):
			sdp.IP = strings.TrimPrefix(line, "c=IN IP4 ")
		case strings.HasPrefix(line, "m=video "):
			parts := strings.Fields(line)
			if len(parts) >= 4 {
				sdp.Port, _ = strconv.Atoi(parts[1])
				sdp.UseTCP = strings.Contains(strings.ToUpper(parts[2]), "TCP")
				sdp.PayloadType, _ = strconv.Atoi(parts[3])
			}
		case strings.HasPrefix(line, "a=rtpmap:"):
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				codec := strings.Split(parts[1], "/")[0]
				sdp.Codec = strings.ToUpper(codec)
				// 解析时钟频率
				if strings.Contains(parts[1], "/") && len(parts) >= 3 {
					sdp.ClockRate, _ = strconv.Atoi(parts[2])
				}
			}
		case strings.HasPrefix(line, "y="):
			// GB28181 SSRC (10位数字)
			ssrc, _ := strconv.ParseUint(strings.TrimPrefix(line, "y="), 10, 32)
			sdp.SSRC = uint32(ssrc)
		case strings.HasPrefix(line, "a=fmtp:"):
			// 解析 fmtp 扩展参数 (SPS/PPS 等)
			if strings.Contains(line, "sprop-parameter-sets") {
				// H.264 SDP 包含 SPS/PPS
			}
		}
	}

	if sdp.IP == "" || sdp.Port == 0 {
		return nil, fmt.Errorf("invalid SDP: missing c= or m= line")
	}

	return sdp, nil
}

// Helper functions
func buildSIP(startLine string, headers []string, body string) []byte {
headers = append(headers, fmt.Sprintf("Content-Length: %d", len(body)))
msg := startLine + "\r\n" + strings.Join(headers, "\r\n") + "\r\n\r\n" + body
return []byte(msg)
}

func parseMessage(lines []string, body string) *Message {
msg := &Message{
Headers: make(map[string]string),
Body:    body,
Raw:     strings.Join(lines, "\r\n") + "\r\n\r\n" + body,
}

if len(lines) == 0 {
return msg
}

// Parse start line
startLine := lines[0]
if strings.HasPrefix(startLine, "SIP/2.0 ") {
fmt.Sscanf(startLine, "SIP/2.0 %d", &msg.StatusCode)
} else {
parts := strings.Fields(startLine)
if len(parts) > 0 {
msg.Method = parts[0]
}
}

// Parse headers
for _, line := range lines[1:] {
idx := strings.Index(line, ":")
if idx > 0 {
key := strings.TrimSpace(line[:idx])
value := strings.TrimSpace(line[idx+1:])
msg.Headers[key] = value
}
}

return msg
}

func extractQuoted(s, key string) string {
re := regexp.MustCompile(fmt.Sprintf(`%s="([^"]+)"`, key))
m := re.FindStringSubmatch(s)
if len(m) > 1 {
return m[1]
}
return ""
}

func extractValue(s, key string) string {
re := regexp.MustCompile(fmt.Sprintf(`%s\s*=\s*"?([a-zA-Z0-9\-]+)`, key))
m := re.FindStringSubmatch(s)
if len(m) > 1 {
return m[1]
}
return ""
}

func md5hex(s string) string {
return fmt.Sprintf("%x", md5.Sum([]byte(s)))
}
