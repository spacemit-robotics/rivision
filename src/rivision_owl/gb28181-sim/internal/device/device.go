// Package device implements GB28181 device simulation
package device

import (
	"bufio"
	"fmt"
	"net"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/rivision/gb28181-sim/internal/sip"
	"github.com/sirupsen/logrus"
)

// ChannelConfig defines a single channel's configuration
type ChannelConfig struct {
	ID     string // Channel ID (20 digits)
	Name   string // Channel display name (optional, defaults to last 4 of ID)
	Source string // Video source: test, rtsp://..., file://...
}

// Config holds device configuration
type Config struct {
ServerIP      string
ServerPort    int
ServerID      string
Domain        string
AgentID       string
AgentPassword string
Channels      []ChannelConfig // Multiple channels
LocalIP       string
UseTCP        bool
}

// Device represents a simulated GB28181 device
type Device struct {
cfg        Config
conn       net.Conn
sipClient  *sip.Client
sessions   map[string]*MediaSession // channelID -> session
sessionsMu sync.RWMutex
stopCh     chan struct{}
wg         sync.WaitGroup
localIP    string
localPort  int
}

// MediaSession represents an active media streaming session
type MediaSession struct {
	ChannelID    string
	Source       string
	DestIP       string
	DestPort     int
	SSRC         uint32
	UseTCP       bool
	PayloadType  int  // RTP Payload Type (96=PS, 98=H264)
	stopCh       chan struct{}
	cmd          interface{} // *exec.Cmd for FFmpeg/GStreamer
}

// New creates a new device simulator
func New(cfg Config) (*Device, error) {
localIP := cfg.LocalIP
if localIP == "" {
ip, err := findLocalIP(cfg.ServerIP)
if err != nil {
return nil, fmt.Errorf("failed to detect local IP: %w", err)
}
localIP = ip
}

return &Device{
cfg:      cfg,
sessions: make(map[string]*MediaSession),
stopCh:   make(chan struct{}),
localIP:  localIP,
}, nil
}

// Run starts the device simulation
func (d *Device) Run() error {
// Connect to SIP server
var err error
if d.cfg.UseTCP {
d.conn, err = net.DialTimeout("tcp",
fmt.Sprintf("%s:%d", d.cfg.ServerIP, d.cfg.ServerPort),
10*time.Second)
} else {
d.conn, err = net.DialTimeout("udp",
fmt.Sprintf("%s:%d", d.cfg.ServerIP, d.cfg.ServerPort),
10*time.Second)
}
if err != nil {
return fmt.Errorf("failed to connect: %w", err)
}
defer d.conn.Close()

// Get local port
localAddr := d.conn.LocalAddr().String()
fmt.Sscanf(localAddr[strings.LastIndex(localAddr, ":")+1:], "%d", &d.localPort)

	// Create SIP client
	names := make(map[string]string)
	for _, ch := range d.cfg.Channels {
		names[ch.ID] = ch.Name
	}
	d.sipClient = sip.NewClient(sip.Config{
LocalIP:   d.localIP,
LocalPort: d.localPort,
ServerIP:  d.cfg.ServerIP,
ServerID:  d.cfg.ServerID,
Domain:    d.cfg.Domain,
AgentID:   d.cfg.AgentID,
Password:  d.cfg.AgentPassword,
		Channels:     d.getChannelIDs(),
		ChannelNames: names,
		UseTCP:       d.cfg.UseTCP,
Conn:      d.conn,
})

// Register
if err := d.sipClient.Register(); err != nil {
return fmt.Errorf("registration failed: %w", err)
}

// Start heartbeat
d.wg.Add(1)
go d.heartbeatLoop()

// Event loop
return d.eventLoop()
}

// Stop stops the device
func (d *Device) Stop() {
close(d.stopCh)

// Stop all media sessions
d.sessionsMu.Lock()
for _, session := range d.sessions {
close(session.stopCh)
}
d.sessionsMu.Unlock()

d.wg.Wait()
}

func (d *Device) getChannelIDs() []string {
ids := make([]string, len(d.cfg.Channels))
for i, ch := range d.cfg.Channels {
ids[i] = ch.ID
}
return ids
}

func (d *Device) getChannelSource(channelID string) string {
for _, ch := range d.cfg.Channels {
if ch.ID == channelID {
return ch.Source
}
}
return "test"
}

func (d *Device) heartbeatLoop() {
	defer d.wg.Done()
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	seq := 1
	for {
		select {
		case <-d.stopCh:
			return
		case <-ticker.C:
			// GB28181 keepalive: send without waiting for response.
			// Heartbeat on the shared socket would race with eventLoop reads.
			if err := d.sipClient.SendKeepalive(seq); err != nil {
				logrus.Warnf("Keepalive send failed: %v", err)
			}
			seq++
		}
	}
}

func (d *Device) eventLoop() error {
	reader := bufio.NewReader(d.conn)

	for {
		d.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		msg, err := d.sipClient.Receive()

		select {
		case <-d.stopCh:
			return nil
		default:
		}

		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				// Socket timeout is normal; check for shutdown signal.
				continue
			}
			// Non-timeout errors (e.g. socket closed) — exit cleanly on stop.
			select {
			case <-d.stopCh:
				return nil
			default:
			}
			return fmt.Errorf("receive error: %w", err)
		}

		logrus.Debugf("RX: %s", msg.Method)

		switch msg.Method {
		case "INVITE":
			d.handleInvite(msg)
		case "BYE":
			d.handleBye(msg)
		case "MESSAGE":
			d.handleMessage(msg)
		case "SUBSCRIBE":
			d.sipClient.SendSubscribeOK(msg)
		case "":
			// Response, ignore
		default:
			d.sipClient.SendOK(msg)
		}

		_ = reader // Keep reference
	}
}

func (d *Device) handleInvite(msg *sip.Message) {
	// Send 100 Trying
	d.sipClient.SendTrying(msg)

	// Parse SDP
	sdp, err := sip.ParseSDP(msg.Body)
	if err != nil {
		logrus.Errorf("Failed to parse SDP: %v", err)
		return
	}

	// Extract channel ID from request URI or To header
	channelID := extractChannelID(msg)
	if channelID == "" {
		logrus.Warn("Cannot determine channel ID from INVITE")
		return
	}

	// 探测文件实际编码：若owl请求了非HEVC PT但文件是HEVC，
	// 覆盖 sdp.PayloadType 使 SendInviteOK 发出正确的 PT=99 SDP。
	source := d.getChannelSource(channelID)
	if !strings.HasPrefix(source, "rtsp://") && source != "test" {
		var filePath string
		if strings.HasPrefix(source, "file://") {
			filePath = strings.TrimPrefix(source, "file://")
		} else {
			filePath = source
		}
		ext := strings.ToLower(filepath.Ext(filePath))
		if ext == ".h265" || ext == ".265" || ext == ".hevc" {
			sdp.PayloadType = PayloadTypeH265
		} else if ext == ".mp4" || ext == ".mkv" || ext == ".avi" || ext == ".mov" {
			codec := probeVideoCodec(filePath)
			if codec == "hevc" {
				sdp.PayloadType = PayloadTypeH265
				logrus.Infof("INVITE channel %s: MP4 内含 HEVC → 改用 PT=99", channelID)
			}
		}
	}

	logrus.Infof("INVITE for channel %s -> %s:%d (PT=%d %s, UDP, SSRC=%d)",
		channelID, sdp.IP, sdp.Port, sdp.PayloadType, sdp.Codec, sdp.SSRC)

	// Send 200 OK with SDP (sdp.PayloadType 现在已正确反映实际编码)
	d.sipClient.SendInviteOK(msg, sdp)

	// Start media session
	session := &MediaSession{
		ChannelID:   channelID,
		Source:      source,
		DestIP:      sdp.IP,
		DestPort:    sdp.Port,
		SSRC:        sdp.SSRC,
		UseTCP:      false, // 强制 UDP
		PayloadType: sdp.PayloadType,
		stopCh:      make(chan struct{}),
	}

	d.sessionsMu.Lock()
	d.sessions[channelID] = session
	d.sessionsMu.Unlock()

	d.wg.Add(1)
	go d.runMediaSession(session)
}

func (d *Device) handleBye(msg *sip.Message) {
d.sipClient.SendOK(msg)

channelID := extractChannelID(msg)
d.sessionsMu.Lock()
if session, ok := d.sessions[channelID]; ok {
close(session.stopCh)
delete(d.sessions, channelID)
}
d.sessionsMu.Unlock()

logrus.Infof("BYE received for channel %s", channelID)
}

func (d *Device) handleMessage(msg *sip.Message) {
d.sipClient.SendOK(msg)
d.sipClient.HandleQuery(msg)
}

func (d *Device) runMediaSession(session *MediaSession) {
defer d.wg.Done()

logrus.Infof("Starting media session: %s -> %s:%d (source: %s)",
session.ChannelID, session.DestIP, session.DestPort, session.Source)

// Determine source type and start streaming
if err := StartMediaStream(session); err != nil {
logrus.Errorf("Media stream error: %v", err)
}
}

func extractChannelID(msg *sip.Message) string {
// Try to extract from To header or Request-URI
to := msg.Headers["To"]
if to != "" {
// Format: <sip:channelID@domain>
start := strings.Index(to, "sip:")
if start >= 0 {
end := strings.Index(to[start:], "@")
if end > 0 {
return to[start+4 : start+end]
}
}
}
return ""
}

func findLocalIP(dst string) (string, error) {
conn, err := net.Dial("udp", fmt.Sprintf("%s:80", dst))
if err != nil {
return "", err
}
defer conn.Close()
localAddr := conn.LocalAddr().(*net.UDPAddr)
return localAddr.IP.String(), nil
}
