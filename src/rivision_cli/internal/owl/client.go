// Copyright 2026 SpacemiT (Hangzhou) Technology Co. Ltd.
//
// SPDX-License-Identifier: Apache-2.0

// Package owl provides a client for OWL (GoWVP) GB28181 platform.
package owl

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Config OWL client configuration.
type Config struct {
	Enabled    bool   `yaml:"enabled" mapstructure:"enabled"`
	URL        string `yaml:"url" mapstructure:"url"` // e.g. http://127.0.0.1:15123
	Username   string `yaml:"username" mapstructure:"username"`
	Password   string `yaml:"password" mapstructure:"password"`
	UseRSAAuth bool   `yaml:"use_rsa_auth" mapstructure:"use_rsa_auth"` // default: false (plain HTTP login)
}

// Client OWL client.
type Client struct {
	config     Config
	httpClient *http.Client
	token      string
	rsaKey     *rsa.PublicKey
}

// NewClient creates a new OWL client.
func NewClient(cfg Config) *Client {
	return &Client{
		config: cfg,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Device OWL device info.
type Device struct {
	ID           string     `json:"id"`
	DeviceID     string     `json:"device_id"`
	Name         string     `json:"name"`
	Type         string     `json:"type"` // GB28181 / ONVIF / RTMP / RTSP
	Manufacturer string     `json:"manufacturer"`
	Model        string     `json:"model"`
	IP           string     `json:"ip"`
	Port         int        `json:"port"`
	IsOnline     bool       `json:"is_online"`
	Children     []*Channel `json:"children"`
	CreatedAt    string     `json:"created_at"`
	UpdatedAt    string     `json:"updated_at"`
}

// Channel OWL channel info.
type Channel struct {
	ID           string `json:"id"`
	DeviceID     string `json:"device_id"`
	ChannelID    string `json:"channel_id"` // GB28181 channel ID (20 digits)
	Name         string `json:"name"`
	Type         string `json:"type"` // GB28181 / RTMP / RTSP / ONVIF
	IsOnline     bool   `json:"is_online"`
	HasRecording bool   `json:"has_recording"`
	PTZType      int    `json:"ptz_type"`
	App          string `json:"app"`
	Stream       string `json:"stream"`
	// Extended fields from GB28181
	Manufacturer string `json:"manufacturer"`
	Model        string `json:"model"`
	Firmware     string `json:"firmware"`
}

// PlayOutput play response containing multi-protocol stream URLs.
type PlayOutput struct {
	App     string            `json:"app"`
	Stream  string            `json:"stream"`
	Session string            `json:"session"`
	URLs    map[string]string `json:"urls"` // http_flv, hls, rtsp, rtmp, webrtc
}

// SnapshotOutput snapshot capture response.
type SnapshotOutput struct {
	URL string `json:"url"`
}

// IsEnabled checks if the client is enabled.
func (c *Client) IsEnabled() bool {
	return c.config.Enabled && c.config.URL != ""
}

// GetConfig returns the current config.
func (c *Client) GetConfig() Config {
	return c.config
}

// ─── RSA login helpers ────────────────────────────────────────────────────────

// getRSAPublicKey fetches the RSA public key from OWL /login/key.
func (c *Client) getRSAPublicKey() (*rsa.PublicKey, error) {
	if c.rsaKey != nil {
		return c.rsaKey, nil
	}
	resp, err := c.httpClient.Get(c.config.URL + "/login/key")
	if err != nil {
		return nil, fmt.Errorf("fetch RSA key: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read RSA key response: %w", err)
	}

	var keyResp struct {
		Key string `json:"key"`
	}
	if err := json.Unmarshal(body, &keyResp); err != nil {
		return nil, fmt.Errorf("parse RSA key response: %w", err)
	}

	pemData, err := base64.StdEncoding.DecodeString(keyResp.Key)
	if err != nil {
		return nil, fmt.Errorf("decode base64 PEM: %w", err)
	}

	block, _ := pem.Decode(pemData)
	if block == nil {
		return nil, fmt.Errorf("PEM decode failed")
	}

	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse public key: %w", err)
	}

	rsaPub, ok := pub.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("not an RSA public key")
	}
	c.rsaKey = rsaPub
	return rsaPub, nil
}

// encryptRSA encrypts plaintext using RSA OAEP/SHA256.
func encryptRSA(pub *rsa.PublicKey, plaintext []byte) ([]byte, error) {
	// OAEP with SHA256, label = empty
	// 注意：rand.Reader 不能为 nil，否则在某些 Go 版本会触发 panic。
	return rsa.EncryptOAEP(
		sha256.New(),
		rand.Reader,
		pub,
		plaintext,
		nil,
	)
}

// ─── Login ───────────────────────────────────────────────────────────────────

// Login authenticates with OWL and stores the token.
// 默认先走 plain；若服务端要求 data 字段，会自动回退 RSA（避免手工切换参数）。
func (c *Client) Login() error {
	if c.config.UseRSAAuth {
		errRSA := c.loginRSA()
		if errRSA == nil {
			return nil
		}
		// 显式 RSA 模式下也尝试 plain，提升兼容性
		errPlain := c.loginPlain()
		if errPlain == nil {
			return nil
		}
		return fmt.Errorf("rsa login failed: %v; plain fallback failed: %v", errRSA, errPlain)
	}

	errPlain := c.loginPlain()
	if errPlain == nil {
		return nil
	}

	// plain 失败后自动回退 RSA（适配要求 data 字段的 OWL 实现）
	errRSA := c.loginRSA()
	if errRSA == nil {
		return nil
	}
	return fmt.Errorf("plain login failed: %v; rsa fallback failed: %v", errPlain, errRSA)
}

// EnsureLogin 确保已登录（token 为空时触发登录）
func (c *Client) EnsureLogin() error {
	if strings.TrimSpace(c.token) != "" {
		return nil
	}
	return c.Login()
}

func (c *Client) loginPlain() error {
	data := map[string]interface{}{
		"username": c.config.Username,
		"password": c.config.Password,
	}
	var result struct {
		Token string `json:"token"`
	}
	if err := c.post("/login", data, &result); err != nil {
		return fmt.Errorf("plain login: %w", err)
	}
	if strings.TrimSpace(result.Token) == "" {
		return fmt.Errorf("plain login: empty token in response")
	}
	c.token = result.Token
	return nil
}

func (c *Client) loginRSA() error {
	pubKey, err := c.getRSAPublicKey()
	if err != nil {
		return fmt.Errorf("get RSA key: %w", err)
	}

	credJSON, _ := json.Marshal(map[string]string{
		"username": c.config.Username,
		"password": c.config.Password,
	})

	ciphertext, err := encryptRSA(pubKey, credJSON)
	if err != nil {
		return fmt.Errorf("RSA encrypt: %w", err)
	}

	data := map[string]string{
		"data": base64.StdEncoding.EncodeToString(ciphertext),
	}
	var result struct {
		Token string `json:"token"`
	}
	if err := c.post("/login", data, &result); err != nil {
		return fmt.Errorf("RSA login: %w", err)
	}
	if strings.TrimSpace(result.Token) == "" {
		return fmt.Errorf("RSA login: empty token in response")
	}
	c.token = result.Token
	return nil
}

// ─── Server Info ─────────────────────────────────────────────────────────────

// ServerInfo server status info.
type ServerInfo struct {
	Version  string `json:"version"`
	UpTime  int64  `json:"up_time"`
	Channels int    `json:"channels"`
	Devices  int    `json:"devices"`
	Streams  int    `json:"streams"`
}

// GetServerInfo returns OWL server info.
func (c *Client) GetServerInfo() (*ServerInfo, error) {
	if err := c.EnsureLogin(); err != nil {
		return nil, err
	}
	var info ServerInfo
	if err := c.get("/api/server/info", nil, &info); err == nil {
		return &info, nil
	}

	// 兼容 rivision_owl：无 /api/server/info，仅有 /health
	var health struct {
		Version string `json:"version"`
	}
	if err := c.get("/health", nil, &health); err == nil {
		return &ServerInfo{Version: health.Version}, nil
	}
	return nil, fmt.Errorf("server info endpoint unavailable")
}

// GetStatus returns OWL server status map.
func (c *Client) GetStatus() (map[string]interface{}, error) {
	var result map[string]interface{}
	if err := c.get("/api/server/info", nil, &result); err == nil {
		return result, nil
	}
	if err := c.get("/health", nil, &result); err == nil {
		return result, nil
	}
	return nil, fmt.Errorf("status endpoint unavailable")
}

// ─── Devices ─────────────────────────────────────────────────────────────────

// GetDevices returns paginated device list.
func (c *Client) GetDevices(page, pageSize int) ([]*Device, int, error) {
	if err := c.EnsureLogin(); err != nil {
		return nil, 0, err
	}
	params := url.Values{}
	params.Set("page", fmt.Sprintf("%d", page))
	params.Set("size", fmt.Sprintf("%d", pageSize)) // OWL 使用 size

	var result struct {
		Items []*Device `json:"items"`
		Total int       `json:"total"`
	}
	if err := c.get("/api/devices", params, &result); err == nil {
		return result.Items, result.Total, nil
	}

	// 兼容 rivision_owl：设备接口为 /devices
	if err := c.get("/devices", params, &result); err == nil {
		return result.Items, result.Total, nil
	}

	// 兜底：若 /devices 不可用，尝试 /devices/channels 并提取设备列表
	devItems, _, err := c.GetDevicesWithChannels(page, pageSize)
	if err != nil {
		return nil, 0, fmt.Errorf("devices endpoint unavailable")
	}
	for _, d := range devItems {
		if d == nil {
			continue
		}
		d.Children = nil
	}
	return devItems, len(devItems), nil
}

// GetDevicesWithChannels returns devices including their channels.
func (c *Client) GetDevicesWithChannels(page, pageSize int) ([]*Device, int, error) {
	if err := c.EnsureLogin(); err != nil {
		return nil, 0, err
	}
	params := url.Values{}
	params.Set("page", fmt.Sprintf("%d", page))
	params.Set("size", fmt.Sprintf("%d", pageSize)) // OWL 使用 size

	var result struct {
		Items []*Device `json:"items"`
		Total int       `json:"total"`
	}
	if err := c.get("/api/devices/channels", params, &result); err == nil {
		return result.Items, result.Total, nil
	}

	// 兼容 rivision_owl：设备+通道接口为 /devices/channels
	if err := c.get("/devices/channels", params, &result); err == nil {
		return result.Items, result.Total, nil
	}

	// 兼容返回格式：{success:true,data:{devices:[...],total:n}}
	var wrapped struct {
		Success bool `json:"success"`
		Data struct {
			Devices []*Device `json:"devices"`
			Items   []*Device `json:"items"`
			Total   int       `json:"total"`
		} `json:"data"`
	}
	err2 := c.get("/devices/channels", params, &wrapped)
	if err2 != nil {
		err2 = c.get("/api/devices/channels", params, &wrapped)
	}
	if err2 == nil {
		items := wrapped.Data.Devices
		if len(items) == 0 {
			items = wrapped.Data.Items
		}
		total := wrapped.Data.Total
		if total == 0 {
			total = len(items)
		}
		return items, total, nil
	}
	return nil, 0, fmt.Errorf("devices/channels endpoint unavailable")
}

// GetDevice returns a single device.
func (c *Client) GetDevice(deviceID string) (*Device, error) {
	var device Device
	err := c.get(fmt.Sprintf("/api/devices/%s", deviceID), nil, &device)
	return &device, err
}

// AddDevice adds a new device.
func (c *Client) AddDevice(deviceType, name string, params map[string]interface{}) (*Device, error) {
	data := map[string]interface{}{
		"type": deviceType,
		"name": name,
	}
	for k, v := range params {
		data[k] = v
	}
	var device Device
	err := c.post("/api/devices", data, &device)
	return &device, err
}

// DeleteDevice removes a device.
func (c *Client) DeleteDevice(deviceID string) error {
	return c.delete(fmt.Sprintf("/api/devices/%s", deviceID))
}

// RefreshCatalog triggers a device catalog refresh.
func (c *Client) RefreshCatalog(deviceID string) error {
	return c.post(fmt.Sprintf("/api/devices/%s/catalog", deviceID), nil, nil)
}

// DiscoverONVIF discovers ONVIF devices.
func (c *Client) DiscoverONVIF() ([]*Device, error) {
	var result struct {
		Items []*Device `json:"items"`
	}
	err := c.get("/api/onvif/discover", nil, &result)
	return result.Items, err
}

// ─── Channels ─────────────────────────────────────────────────────────────────

// GetChannels returns paginated channel list.
// Pass empty deviceID to get all channels.
func (c *Client) GetChannels(page, pageSize int, deviceID string) ([]*Channel, int, error) {
	if err := c.EnsureLogin(); err != nil {
		return nil, 0, err
	}
	params := url.Values{}
	params.Set("page", fmt.Sprintf("%d", page))
	params.Set("size", fmt.Sprintf("%d", pageSize)) // OWL 使用 size 而非 page_size
	if deviceID != "" {
		params.Set("device_id", deviceID)
	}

	var result struct {
		Items []*Channel `json:"items"`
		Total int        `json:"total"`
	}
	err := c.get("/api/channels", params, &result)
	if err == nil {
		return result.Items, result.Total, nil
	}

	// 兼容 rivision_owl：通道接口为 /channels
	if err2 := c.get("/channels", params, &result); err2 == nil {
		return result.Items, result.Total, nil
	}

	// 兼容返回格式：{success:true,data:{channels:[...],total:n}}
	var wrapped struct {
		Success bool `json:"success"`
		Data struct {
			Channels []*Channel `json:"channels"`
			Items    []*Channel `json:"items"`
			Total    int        `json:"total"`
		} `json:"data"`
	}
	err2 := c.get("/api/channels", params, &wrapped)
	if err2 != nil {
		err2 = c.get("/channels", params, &wrapped)
	}
	if err2 == nil {
		items := wrapped.Data.Channels
		if len(items) == 0 {
			items = wrapped.Data.Items
		}
		total := wrapped.Data.Total
		if total == 0 {
			total = len(items)
		}
		return items, total, nil
	}

	// 兼容某些 OWL / GB28181-SIM 版本：/channels 分页异常或无该接口时，回退 /devices/channels 聚合
	if len(result.Items) == 1 && result.Total > 1 {
		if devItems, _, devErr := c.GetDevicesWithChannels(1, 100); devErr == nil {
			channels := make([]*Channel, 0)
			for _, d := range devItems {
				if d == nil {
					continue
				}
				if deviceID != "" && d.ID != deviceID && d.DeviceID != deviceID {
					continue
				}
				for _, ch := range d.Children {
					if ch == nil {
						continue
					}
					if ch.DeviceID == "" {
						ch.DeviceID = d.ID
					}
					channels = append(channels, ch)
				}
			}
			if len(channels) > 0 {
				return channels, len(channels), nil
			}
		}
	}

	// 兼容某些 OWL / GB28181-SIM 版本：无 /api/channels，仅支持 /api/devices/channels
	devItems, _, devErr := c.GetDevicesWithChannels(page, pageSize)
	if devErr != nil {
		return nil, 0, err
	}

	channels := make([]*Channel, 0)
	for _, d := range devItems {
		if d == nil {
			continue
		}
		if deviceID != "" && d.ID != deviceID && d.DeviceID != deviceID {
			continue
		}
		for _, ch := range d.Children {
			if ch == nil {
				continue
			}
			if ch.DeviceID == "" {
				ch.DeviceID = d.ID
			}
			channels = append(channels, ch)
		}
	}
	return channels, len(channels), nil
}

// GetAllChannels returns ALL channels across all pages.
func (c *Client) GetAllChannels() ([]*Channel, error) {
	var all []*Channel
	seen := map[string]struct{}{}
	page := 1
	pageSize := 100
	maxPages := 20

	for i := 0; i < maxPages; i++ {
		items, total, err := c.GetChannels(page, pageSize, "")
		if err != nil {
			return nil, err
		}

		addedThisPage := 0
		for _, ch := range items {
			if ch == nil {
				continue
			}
			id := ch.ID
			if id == "" {
				id = ch.ChannelID
			}
			if id == "" {
				continue
			}
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			all = append(all, ch)
			addedThisPage++
		}

		if total > 0 && len(all) >= total {
			break
		}
		// 关键保护：若分页接口每页重复同一条，避免死循环并允许降级返回当前去重结果
		if len(items) == 0 || addedThisPage == 0 {
			break
		}
		page++
	}
	return all, nil
}

// GetChannel returns a single channel by its internal ID.
func (c *Client) GetChannel(channelID string) (*Channel, error) {
	var ch Channel
	err := c.get(fmt.Sprintf("/api/channels/%s", channelID), nil, &ch)
	return &ch, err
}

// AddChannel adds a RTMP/RTSP channel.
func (c *Client) AddChannel(channelType, name, streamURL string) (*Channel, error) {
	data := map[string]interface{}{
		"type": channelType,
		"name": name,
		"url":  streamURL,
	}
	var ch Channel
	err := c.post("/api/channels", data, &ch)
	return &ch, err
}

// DeleteChannel removes a channel.
func (c *Client) DeleteChannel(channelID string) error {
	return c.delete(fmt.Sprintf("/api/channels/%s", channelID))
}

// ─── Playback ─────────────────────────────────────────────────────────────────

// Play initiates streaming for a channel.
// Returns multi-protocol URLs (HTTP-FLV, HLS, RTSP, RTMP, WebRTC).
func (c *Client) Play(channelID string) (*PlayOutput, error) {
	if err := c.EnsureLogin(); err != nil {
		return nil, err
	}

	// 先按标准输出解析
	var output PlayOutput
	err := c.post(fmt.Sprintf("/api/channels/%s/play", channelID), nil, &output)
	if err == nil {
		if output.URLs == nil {
			output.URLs = map[string]string{}
		}
		return &output, nil
	}
	// 兼容 rivision_owl：播放接口为 /channels/:id/play，且返回 items 数组
	var compat struct {
		App    string `json:"app"`
		Stream string `json:"stream"`
		Items []struct {
			HTTPFLV string `json:"http_flv"`
			RTMP    string `json:"rtmp"`
			RTSP    string `json:"rtsp"`
			WebRTC  string `json:"webrtc"`
			HLS     string `json:"hls"`
		} `json:"items"`
	}
	err = c.post(fmt.Sprintf("/channels/%s/play", channelID), nil, &compat)
	if err != nil {
		return &output, err
	}

	urls := map[string]string{}
	if len(compat.Items) > 0 {
		it := compat.Items[0]
		if it.RTSP != "" {
			urls["rtsp"] = it.RTSP
		}
		if it.RTMP != "" {
			urls["rtmp"] = it.RTMP
		}
		if it.HTTPFLV != "" {
			urls["http_flv"] = it.HTTPFLV
		}
		if it.WebRTC != "" {
			urls["webrtc"] = it.WebRTC
		}
		if it.HLS != "" {
			urls["hls"] = it.HLS
		}
	}

	return &PlayOutput{App: compat.App, Stream: compat.Stream, URLs: urls}, nil
}

// Stop stops the active stream for a channel.
func (c *Client) Stop(channelID string) error {
	if err := c.EnsureLogin(); err != nil {
		return err
	}
	if err := c.post(fmt.Sprintf("/api/channels/%s/stop", channelID), nil, nil); err == nil {
		return nil
	}
	// 兼容 rivision_owl：当前可能无 stop 接口，返回 nil 以保持幂等
	_ = c.post(fmt.Sprintf("/channels/%s/stop", channelID), nil, nil)
	return nil
}

// GetRTSPURL is a convenience method: Play + extract RTSP URL.
func (c *Client) GetRTSPURL(channelID string) (string, error) {
	output, err := c.Play(channelID)
	if err != nil {
		return "", err
	}
	if rtsp, ok := output.URLs["rtsp"]; ok && rtsp != "" {
		return rtsp, nil
	}
	// try common keys
	for _, k := range []string{"rtsp", "http_flv", "rtmp", "hls", "webrtc"} {
		if v, ok := output.URLs[k]; ok && v != "" {
			return v, nil
		}
	}
	return "", fmt.Errorf("no available URL in response")
}

// ─── Snapshot ─────────────────────────────────────────────────────────────────

// Snapshot captures a JPEG snapshot from a channel.
func (c *Client) Snapshot(channelID string) error {
	if err := c.post(fmt.Sprintf("/api/channels/%s/snapshot", channelID), nil, nil); err == nil {
		return nil
	}
	return c.post(fmt.Sprintf("/channels/%s/snapshot", channelID), nil, nil)
}

// SnapshotWithAuth captures a snapshot with Authorization header.
func (c *Client) SnapshotWithAuth(channelID string) ([]byte, error) {
	if err := c.EnsureLogin(); err != nil {
		return nil, err
	}
	req, err := http.NewRequest("POST", c.config.URL+fmt.Sprintf("/api/channels/%s/snapshot", channelID), nil)
	if err != nil {
		return nil, err
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("snapshot request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		// 兼容 rivision_owl：/channels/:id/snapshot
		req2, err2 := http.NewRequest("POST", c.config.URL+fmt.Sprintf("/channels/%s/snapshot", channelID), nil)
		if err2 == nil {
			c.setHeaders(req2)
			resp2, err3 := c.httpClient.Do(req2)
			if err3 == nil {
				defer resp2.Body.Close()
				if resp2.StatusCode < 400 {
					return io.ReadAll(resp2.Body)
				}
			}
		}
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("snapshot error %d: %s", resp.StatusCode, string(body))
	}

	return io.ReadAll(resp.Body)
}

// ─── AI / Analysis ───────────────────────────────────────────────────────────

// EnableAI enables AI detection for a channel.
func (c *Client) EnableAI(channelID string) error {
	return c.post(fmt.Sprintf("/api/channels/%s/ai/enable", channelID), nil, nil)
}

// DisableAI disables AI detection for a channel.
func (c *Client) DisableAI(channelID string) error {
	return c.post(fmt.Sprintf("/api/channels/%s/ai/disable", channelID), nil, nil)
}

// ─── PTZ ─────────────────────────────────────────────────────────────────────

// PTZControl sends a PTZ command.
func (c *Client) PTZControl(channelID, command string, speed int) error {
	if err := c.EnsureLogin(); err != nil {
		return err
	}
	data := map[string]interface{}{
		"channel_id": channelID,
		"command":    command, // left / right / up / down / zoom_in / zoom_out / stop
		"speed":      speed,   // 1-8
	}
	if err := c.post("/api/ptz/control", data, nil); err == nil {
		return nil
	}
	return c.post("/ptz/control", data, nil)
}

// ─── HTTP helpers ─────────────────────────────────────────────────────────────

func (c *Client) get(path string, params url.Values, result interface{}) error {
	reqURL := c.config.URL + path
	if params != nil && len(params) > 0 {
		reqURL += "?" + params.Encode()
	}

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return fmt.Errorf("create GET request: %w", err)
	}
	c.setHeaders(req)

	return c.doRequest(req, result)
}

func (c *Client) post(path string, data interface{}, result interface{}) error {
	var body io.Reader
	if data != nil {
		jsonData, err := json.Marshal(data)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		body = bytes.NewReader(jsonData)
	}

	req, err := http.NewRequest("POST", c.config.URL+path, body)
	if err != nil {
		return fmt.Errorf("create POST request: %w", err)
	}
	c.setHeaders(req)
	if data != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	return c.doRequest(req, result)
}

func (c *Client) delete(path string) error {
	req, err := http.NewRequest("DELETE", c.config.URL+path, nil)
	if err != nil {
		return fmt.Errorf("create DELETE request: %w", err)
	}
	c.setHeaders(req)

	return c.doRequest(req, nil)
}

func (c *Client) setHeaders(req *http.Request) {
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	req.Header.Set("User-Agent", "rivision-cli/owl-client")
}

func (c *Client) doRequest(req *http.Request, result interface{}) error {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return fmt.Errorf("API error %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	// Try wrapped response {code,msg,data} only when structure is actually present.
	var wrapped struct {
		Code int             `json:"code"`
		Msg  string          `json:"msg"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &wrapped); err == nil {
		hasWrapped := len(wrapped.Data) > 0 || wrapped.Msg != "" || strings.Contains(string(body), `"code"`)
		if hasWrapped {
			if wrapped.Code != 0 && wrapped.Code != 200 {
				return fmt.Errorf("OWL error %d: %s", wrapped.Code, wrapped.Msg)
			}
			if result != nil {
				if len(wrapped.Data) > 0 {
					return json.Unmarshal(wrapped.Data, result)
				}
				// wrapped 结构但无 data（如仅 msg），按 direct 兜底解析
				return json.Unmarshal(body, result)
			}
			return nil
		}
	}

	// Fallback: direct parse.
	if result != nil {
		return json.Unmarshal(body, result)
	}
	return nil
}
