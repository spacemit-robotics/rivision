package owl

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// newTestMux builds a mock OWL server supporting /login, /api/devices, /api/channels, etc.
func newTestMux() *http.ServeMux {
	mux := http.NewServeMux()

	// Login endpoint
	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"token": "test-token-abc123",
			},
		})
	})

	// Devices — /api/devices path (GoWVP-style wrapped)
	mux.HandleFunc("/api/devices", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			var req AddDeviceRequest
			json.NewDecoder(r.Body).Decode(&req)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"id":        "gb_" + req.DeviceID,
					"type":      req.Type,
					"device_id": req.DeviceID,
					"name":      req.Name,
					"is_online": false,
				},
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"items": []map[string]interface{}{
					{"id": "gb_001", "type": "GB28181", "device_id": "34020000001320000001", "name": "入口", "is_online": true},
					{"id": "onvif_01", "type": "ONVIF", "name": "走廊", "ip": "192.168.1.101", "is_online": true},
				},
				"total": 2,
			},
		})
	})

	// Devices/channels — /api/devices/channels
	mux.HandleFunc("/api/devices/channels", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"items": []map[string]interface{}{
					{
						"id": "gb_001", "type": "GB28181", "name": "入口",
						"children": []map[string]interface{}{
							{"id": "ch_001", "type": "GB28181", "name": "通道1", "is_online": true},
						},
					},
				},
			},
		})
	})

	// Channels — /api/channels (paginated)
	mux.HandleFunc("/api/channels", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"items": []map[string]interface{}{
					{"id": "ch_001", "type": "GB28181", "name": "通道1", "device_id": "gb_001", "is_online": true},
					{"id": "ch_002", "type": "ONVIF", "name": "通道2", "device_id": "onvif_01", "is_online": true},
				},
				"total": 2,
			},
		})
	})

	// Single channel
	mux.HandleFunc("/api/channels/ch_001", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"id": "ch_001", "type": "GB28181", "name": "通道1", "device_id": "gb_001", "is_online": true,
			},
		})
	})

	// Play channel — /api/channels/{id}/play
	mux.HandleFunc("/api/channels/ch_001/play", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"app":    "rtp",
				"stream": "34020000001320000001",
				"items": []map[string]interface{}{
					{"rtsp": "rtsp://192.168.1.10:554/rtp/34020000001320000001", "hls": "http://192.168.1.10:8220/hls.m3u8"},
				},
			},
		})
	})

	// Stop channel (best-effort)
	mux.HandleFunc("/api/channels/ch_001/stop", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{"code": 0})
	})

	// PTZ
	mux.HandleFunc("/api/ptz/control", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{"code": 0})
	})

	return mux
}

func TestLoginAndAuth(t *testing.T) {
	srv := httptest.NewServer(newTestMux())
	defer srv.Close()

	client := NewClient(Config{URL: srv.URL, Enabled: true, Username: "admin", Password: "pass123"})
	err := client.Login(context.Background())
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	client.mu.RLock()
	token := client.token
	client.mu.RUnlock()
	if token != "test-token-abc123" {
		t.Errorf("token = %q, want test-token-abc123", token)
	}
}

func TestEnsureLoginNoAuth(t *testing.T) {
	// No username → no login needed
	client := NewClient(Config{URL: "http://not-used", Enabled: true})
	err := client.EnsureLogin(context.Background())
	if err != nil {
		t.Fatalf("EnsureLogin with no auth: %v", err)
	}
}

func TestListDevicesWithFallback(t *testing.T) {
	srv := httptest.NewServer(newTestMux())
	defer srv.Close()

	client := NewClient(Config{URL: srv.URL, Enabled: true})
	devices, err := client.ListDevices(context.Background())
	if err != nil {
		t.Fatalf("ListDevices: %v", err)
	}
	if len(devices) != 2 {
		t.Fatalf("got %d devices, want 2", len(devices))
	}
	if devices[0].Type != "GB28181" {
		t.Errorf("device[0].Type = %q, want GB28181", devices[0].Type)
	}
	if devices[1].Type != "ONVIF" {
		t.Errorf("device[1].Type = %q, want ONVIF", devices[1].Type)
	}
}

func TestListDevicesWithChannels(t *testing.T) {
	srv := httptest.NewServer(newTestMux())
	defer srv.Close()

	client := NewClient(Config{URL: srv.URL, Enabled: true})
	devices, err := client.ListDevicesWithChannels(context.Background())
	if err != nil {
		t.Fatalf("ListDevicesWithChannels: %v", err)
	}
	if len(devices) == 0 {
		t.Fatal("no devices returned")
	}
	if len(devices[0].Children) == 0 {
		t.Error("device[0] should have children channels")
	}
}

func TestAddDevice(t *testing.T) {
	srv := httptest.NewServer(newTestMux())
	defer srv.Close()

	client := NewClient(Config{URL: srv.URL, Enabled: true})
	dev, err := client.AddDevice(context.Background(), AddDeviceRequest{
		Type:     "GB28181",
		DeviceID: "34020000001320000001",
		Name:     "测试摄像头",
	})
	if err != nil {
		t.Fatalf("AddDevice: %v", err)
	}
	if dev.Type != "GB28181" {
		t.Errorf("Type = %q, want GB28181", dev.Type)
	}
}

func TestGetAllChannels(t *testing.T) {
	srv := httptest.NewServer(newTestMux())
	defer srv.Close()

	client := NewClient(Config{URL: srv.URL, Enabled: true})
	channels, err := client.GetAllChannels(context.Background())
	if err != nil {
		t.Fatalf("GetAllChannels: %v", err)
	}
	if len(channels) != 2 {
		t.Fatalf("got %d channels, want 2", len(channels))
	}
}

func TestGetChannel(t *testing.T) {
	srv := httptest.NewServer(newTestMux())
	defer srv.Close()

	client := NewClient(Config{URL: srv.URL, Enabled: true})
	ch, err := client.GetChannel(context.Background(), "ch_001")
	if err != nil {
		t.Fatalf("GetChannel: %v", err)
	}
	if ch.Type != "GB28181" {
		t.Errorf("channel.Type = %q, want GB28181", ch.Type)
	}
}

func TestPlayChannel(t *testing.T) {
	srv := httptest.NewServer(newTestMux())
	defer srv.Close()

	client := NewClient(Config{URL: srv.URL, Enabled: true})
	result, err := client.PlayChannel(context.Background(), "ch_001")
	if err != nil {
		t.Fatalf("PlayChannel: %v", err)
	}
	if len(result.Items) == 0 {
		t.Fatal("no items in play result")
	}
	if result.Items[0].RTSP == "" {
		t.Error("RTSP URL is empty")
	}
}

func TestPlayChannelGoWVPCompat(t *testing.T) {
	// GoWVP returns flat URLs map instead of items array
	mux := http.NewServeMux()
	mux.HandleFunc("/api/channels/ch_gowvp/play", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"app":    "live",
				"stream": "cam01",
				"urls": map[string]string{
					"rtsp":     "rtsp://192.168.1.10:554/live/cam01",
					"http_flv": "http://192.168.1.10:8220/live/cam01.flv",
				},
			},
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := NewClient(Config{URL: srv.URL, Enabled: true})
	result, err := client.PlayChannel(context.Background(), "ch_gowvp")
	if err != nil {
		t.Fatalf("PlayChannel GoWVP: %v", err)
	}
	if result.URLs["rtsp"] != "rtsp://192.168.1.10:554/live/cam01" {
		t.Errorf("URLs[rtsp] = %q", result.URLs["rtsp"])
	}
}

func TestResolveRTSPWithCache(t *testing.T) {
	callCount := 0
	mux := http.NewServeMux()
	mux.HandleFunc("/api/channels/ch_cache/play", func(w http.ResponseWriter, r *http.Request) {
		callCount++
		json.NewEncoder(w).Encode(map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"app": "rtp", "stream": "ch_cache",
				"items": []map[string]interface{}{{"rtsp": "rtsp://192.168.1.10:554/rtp/ch_cache"}},
			},
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := NewClient(Config{URL: srv.URL, Enabled: true, CacheTTL: 1 * time.Minute})

	ctx := context.Background()

	// First call → hits server
	rtsp1, err := client.ResolveRTSP(ctx, "ch_cache")
	if err != nil {
		t.Fatalf("ResolveRTSP 1st: %v", err)
	}
	if rtsp1 == "" {
		t.Error("empty RTSP URL")
	}
	if callCount != 1 {
		t.Errorf("expected 1 server call, got %d", callCount)
	}

	// Second call → served from cache
	rtsp2, err := client.ResolveRTSP(ctx, "ch_cache")
	if err != nil {
		t.Fatalf("ResolveRTSP 2nd: %v", err)
	}
	if rtsp2 != rtsp1 {
		t.Errorf("cached URL mismatch: %q vs %q", rtsp2, rtsp1)
	}
	if callCount != 1 {
		t.Errorf("cache miss: expected 1 server call, got %d", callCount)
	}

	// Invalidate cache → next call hits server
	client.InvalidateRTSPCache("ch_cache")
	_, _ = client.ResolveRTSP(ctx, "ch_cache")
	if callCount != 2 {
		t.Errorf("after invalidation: expected 2 server calls, got %d", callCount)
	}
}

func TestPTZControl(t *testing.T) {
	srv := httptest.NewServer(newTestMux())
	defer srv.Close()

	client := NewClient(Config{URL: srv.URL, Enabled: true})
	err := client.PTZControl(context.Background(), "ch_001", "left", 4)
	if err != nil {
		t.Fatalf("PTZControl: %v", err)
	}
}

func TestStopChannel(t *testing.T) {
	srv := httptest.NewServer(newTestMux())
	defer srv.Close()

	client := NewClient(Config{URL: srv.URL, Enabled: true})
	err := client.StopChannel(context.Background(), "ch_001")
	if err != nil {
		t.Fatalf("StopChannel: %v", err)
	}
}

func TestPing(t *testing.T) {
	srv := httptest.NewServer(newTestMux())
	defer srv.Close()

	client := NewClient(Config{URL: srv.URL, Enabled: true})
	if err := client.Ping(context.Background()); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestAuthRetryOn401(t *testing.T) {
	callCount := 0
	mux := http.NewServeMux()
	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"token": "new-token"},
		})
	})
	mux.HandleFunc("/api/devices", func(w http.ResponseWriter, r *http.Request) {
		callCount++
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") || auth == "Bearer expired" {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"code":401,"msg":"unauthorized"}`))
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"items": []map[string]interface{}{{"id": "dev1", "type": "RTSP", "name": "test"}},
			},
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := NewClient(Config{URL: srv.URL, Enabled: true, Username: "admin", Password: "pass"})

	// Set an expired token
	client.mu.Lock()
	client.token = "expired"
	client.mu.Unlock()

	// ListDevices should auto-retry after 401
	devices, err := client.ListDevices(context.Background())
	if err != nil {
		t.Fatalf("ListDevices with retry: %v", err)
	}
	if len(devices) != 1 {
		t.Errorf("got %d devices, want 1", len(devices))
	}
	if callCount < 2 {
		t.Errorf("expected at least 2 API calls (first=401, second=ok), got %d", callCount)
	}
}

func TestExtractBestURL(t *testing.T) {
	client := NewClient(Config{Enabled: true})

	// items format
	result1 := &PlayResult{
		Items: []StreamLiveAddr{
			{RTSP: "rtsp://host/stream", RTMP: "rtmp://host/stream"},
		},
	}
	if got := client.extractBestURL(result1); got != "rtsp://host/stream" {
		t.Errorf("extractBestURL items: %q, want rtsp", got)
	}

	// URLs map format (GoWVP)
	result2 := &PlayResult{
		URLs: map[string]string{
			"rtmp":     "rtmp://host/stream",
			"http_flv": "http://host/stream.flv",
		},
	}
	if got := client.extractBestURL(result2); got != "rtmp://host/stream" {
		t.Errorf("extractBestURL urls: %q, want rtmp", got)
	}

	// Empty
	result3 := &PlayResult{}
	if got := client.extractBestURL(result3); got != "" {
		t.Errorf("extractBestURL empty: %q, want empty", got)
	}
}

func TestNormalizeStreamURL(t *testing.T) {
	// OWL configured with real IP → should replace localhost in stream URLs
	client := NewClient(Config{URL: "http://192.168.1.100:15123", Enabled: true})

	// localhost → real IP
	got := client.normalizeStreamURL("rtsp://127.0.0.1:554/rtp/stream1")
	if got != "rtsp://192.168.1.100:554/rtp/stream1" {
		t.Errorf("normalize 127.0.0.1: got %q", got)
	}

	// localhost hostname
	got = client.normalizeStreamURL("rtsp://localhost:554/rtp/stream2")
	if got != "rtsp://192.168.1.100:554/rtp/stream2" {
		t.Errorf("normalize localhost: got %q", got)
	}

	// Already external → no change
	got = client.normalizeStreamURL("rtsp://10.0.0.5:554/rtp/stream3")
	if got != "rtsp://10.0.0.5:554/rtp/stream3" {
		t.Errorf("normalize external: got %q", got)
	}

	// OWL itself is localhost → cannot improve, return as-is
	localClient := NewClient(Config{URL: "http://localhost:15123", Enabled: true})
	got = localClient.normalizeStreamURL("rtsp://127.0.0.1:554/rtp/stream4")
	if got != "rtsp://127.0.0.1:554/rtp/stream4" {
		t.Errorf("normalize owl-localhost: got %q", got)
	}
}

func TestIsEnabled(t *testing.T) {
	c1 := NewClient(Config{Enabled: true, URL: "http://owl:15123"})
	if !c1.IsEnabled() {
		t.Error("should be enabled")
	}

	c2 := NewClient(Config{Enabled: false, URL: "http://owl:15123"})
	if c2.IsEnabled() {
		t.Error("should be disabled")
	}
}
