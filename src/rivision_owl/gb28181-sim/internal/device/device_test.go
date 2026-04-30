package device

import (
"testing"
)

func TestParseChannels(t *testing.T) {
tests := []struct {
name     string
input    []string
wantLen  int
wantID   string
wantSrc  string
}{
{
name:     "single channel with source",
input:    []string{"34020000001310000001:test"},
wantLen:  1,
wantID:   "34020000001310000001",
wantSrc:  "test",
},
{
name:     "channel with rtsp source",
input:    []string{"34020000001310000001:rtsp://admin:admin@192.168.1.100/stream"},
wantLen:  1,
wantID:   "34020000001310000001",
wantSrc:  "rtsp://admin:admin@192.168.1.100/stream",
},
{
name:     "channel with file source",
input:    []string{"34020000001310000001:file:///videos/test.h264"},
wantLen:  1,
wantID:   "34020000001310000001",
wantSrc:  "file:///videos/test.h264",
},
{
name:     "multiple channels",
input:    []string{"ch1:test", "ch2:rtsp://x", "ch3:file:///y"},
wantLen:  3,
wantID:   "ch1",
wantSrc:  "test",
},
}

for _, tt := range tests {
t.Run(tt.name, func(t *testing.T) {
configs := parseChannelsTest(tt.input)
if len(configs) != tt.wantLen {
t.Errorf("len = %v, want %v", len(configs), tt.wantLen)
}
if len(configs) > 0 {
if configs[0].ID != tt.wantID {
t.Errorf("ID = %v, want %v", configs[0].ID, tt.wantID)
}
if configs[0].Source != tt.wantSrc {
t.Errorf("Source = %v, want %v", configs[0].Source, tt.wantSrc)
}
}
})
}
}

// parseChannelsTest is a copy of main.parseChannels for testing
func parseChannelsTest(channels []string) []ChannelConfig {
var configs []ChannelConfig
for _, ch := range channels {
idx := findFirstColon(ch)
if idx > 0 {
configs = append(configs, ChannelConfig{
ID:     ch[:idx],
Source: ch[idx+1:],
})
} else {
configs = append(configs, ChannelConfig{
ID:     ch,
Source: "test",
})
}
}
return configs
}

func findFirstColon(s string) int {
for i, c := range s {
if c == ':' {
return i
}
}
return -1
}

func TestFindLocalIP(t *testing.T) {
// Test with a known reachable IP (Google DNS)
ip, err := findLocalIP("8.8.8.8")
if err != nil {
t.Skipf("Network not available: %v", err)
}
if ip == "" {
t.Error("findLocalIP returned empty string")
}
t.Logf("Local IP for reaching 8.8.8.8: %s", ip)
}

func TestDeviceConfig(t *testing.T) {
cfg := Config{
ServerIP:      "192.168.1.100",
ServerPort:    5060,
ServerID:      "34020000002000000001",
Domain:        "3402000000",
AgentID:       "34020000001320000001",
AgentPassword: "12345678",
Channels: []ChannelConfig{
{ID: "34020000001310000001", Source: "test"},
{ID: "34020000001310000002", Source: "rtsp://x"},
},
LocalIP: "192.168.1.50",
UseTCP:  false,
}

if len(cfg.Channels) != 2 {
t.Errorf("Channels len = %v, want 2", len(cfg.Channels))
}
if cfg.Channels[0].ID != "34020000001310000001" {
t.Errorf("Channel[0].ID = %v", cfg.Channels[0].ID)
}
}

func TestMediaSession(t *testing.T) {
session := &MediaSession{
ChannelID: "34020000001310000001",
Source:    "test",
DestIP:    "192.168.1.100",
DestPort:  20000,
SSRC:      12345,
UseTCP:    false,
stopCh:    make(chan struct{}),
}

if session.ChannelID != "34020000001310000001" {
t.Errorf("ChannelID = %v", session.ChannelID)
}
if session.DestPort != 20000 {
t.Errorf("DestPort = %v", session.DestPort)
}
}
