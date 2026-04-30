// Copyright 2026 SpacemiT (Hangzhou) Technology Co. Ltd.
//
// SPDX-License-Identifier: Apache-2.0

package sip

import (
"strings"
"testing"
)

func TestParseSDP(t *testing.T) {
tests := []struct {
name    string
body    string
wantIP  string
wantPort int
wantPT  int
wantTCP bool
wantErr bool
}{
{
name: "basic UDP SDP",
body: `v=0
o=- 0 0 IN IP4 192.168.1.100
s=Play
c=IN IP4 192.168.1.100
t=0 0
m=video 20000 RTP/AVP 96
a=rtpmap:96 PS/90000
a=recvonly
y=0000000001`,
wantIP:   "192.168.1.100",
wantPort: 20000,
wantPT:   96,
wantTCP:  false,
wantErr:  false,
},
{
name: "TCP passive SDP",
body: `v=0
o=- 0 0 IN IP4 192.168.1.100
s=Play
c=IN IP4 192.168.1.100
t=0 0
m=video 20000 TCP/RTP/AVP 96
a=rtpmap:96 PS/90000
a=setup:passive
a=connection:new
y=0000000002`,
wantIP:   "192.168.1.100",
wantPort: 20000,
wantPT:   96,
wantTCP:  true,
wantErr:  false,
},
{
name: "H264 payload",
body: `v=0
o=- 0 0 IN IP4 10.0.0.1
s=Play
c=IN IP4 10.0.0.1
t=0 0
m=video 30000 RTP/AVP 98
a=rtpmap:98 H264/90000`,
wantIP:   "10.0.0.1",
wantPort: 30000,
wantPT:   98,
wantTCP:  false,
wantErr:  false,
},
{
name:    "missing c= line",
body:    `v=0\nm=video 20000 RTP/AVP 96`,
wantErr: true,
},
{
name:    "missing m= line",
body:    `v=0\nc=IN IP4 192.168.1.100`,
wantErr: true,
},
}

for _, tt := range tests {
t.Run(tt.name, func(t *testing.T) {
sdp, err := ParseSDP(tt.body)
if tt.wantErr {
if err == nil {
t.Errorf("ParseSDP() expected error, got nil")
}
return
}
if err != nil {
t.Errorf("ParseSDP() error = %v", err)
return
}
if sdp.IP != tt.wantIP {
t.Errorf("IP = %v, want %v", sdp.IP, tt.wantIP)
}
if sdp.Port != tt.wantPort {
t.Errorf("Port = %v, want %v", sdp.Port, tt.wantPort)
}
if sdp.PayloadType != tt.wantPT {
t.Errorf("PayloadType = %v, want %v", sdp.PayloadType, tt.wantPT)
}
if sdp.UseTCP != tt.wantTCP {
t.Errorf("UseTCP = %v, want %v", sdp.UseTCP, tt.wantTCP)
}
})
}
}

func TestBuildSIP(t *testing.T) {
headers := []string{
"Via: SIP/2.0/UDP 192.168.1.1:5060;branch=z9hG4bK123",
"From: <sip:test@domain>;tag=abc",
"To: <sip:server@domain>",
"Call-ID: call123",
"CSeq: 1 REGISTER",
}
body := ""

msg := buildSIP("REGISTER sip:domain SIP/2.0", headers, body)
msgStr := string(msg)

if !strings.HasPrefix(msgStr, "REGISTER sip:domain SIP/2.0\r\n") {
t.Error("Missing start line")
}
if !strings.Contains(msgStr, "Via: SIP/2.0/UDP") {
t.Error("Missing Via header")
}
if !strings.Contains(msgStr, "Content-Length: 0") {
t.Error("Missing Content-Length")
}
if !strings.HasSuffix(msgStr, "\r\n\r\n") {
t.Error("Missing final CRLF CRLF")
}
}

func TestParseMessage(t *testing.T) {
lines := []string{
"SIP/2.0 200 OK",
"Via: SIP/2.0/UDP 192.168.1.1:5060;branch=z9hG4bK123",
"From: <sip:test@domain>;tag=abc",
"To: <sip:server@domain>;tag=xyz",
"Call-ID: call123",
"CSeq: 1 REGISTER",
}
body := ""

msg := parseMessage(lines, body)

if msg.StatusCode != 200 {
t.Errorf("StatusCode = %v, want 200", msg.StatusCode)
}
if msg.Headers["Call-ID"] != "call123" {
t.Errorf("Call-ID = %v, want call123", msg.Headers["Call-ID"])
}
}

func TestExtractQuoted(t *testing.T) {
tests := []struct {
input string
key   string
want  string
}{
{
input: `Digest realm="test.com", nonce="abc123", qop=auth`,
key:   "realm",
want:  "test.com",
},
{
input: `Digest realm="test.com", nonce="abc123", qop=auth`,
key:   "nonce",
want:  "abc123",
},
{
input: `Digest realm="test.com"`,
key:   "missing",
want:  "",
},
}

for _, tt := range tests {
got := extractQuoted(tt.input, tt.key)
if got != tt.want {
t.Errorf("extractQuoted(%q, %q) = %q, want %q", tt.input, tt.key, got, tt.want)
}
}
}

func TestMd5hex(t *testing.T) {
// MD5("hello") = 5d41402abc4b2a76b9719d911017c592
got := md5hex("hello")
want := "5d41402abc4b2a76b9719d911017c592"
if got != want {
t.Errorf("md5hex(hello) = %v, want %v", got, want)
}
}
