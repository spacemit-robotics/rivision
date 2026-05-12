// Package nodes provides the Gateway HTTP client for Hub (v6_design §5.4).
// Hub does NOT embed Gateway code — it calls Gateway via HTTP API.
package nodes

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/rivision/rivision-hub/internal/scheduler"
)

// GatewayClient communicates with rivision_gateway (:9090) via HTTP.
type GatewayClient struct {
	gatewayURL string
	httpClient *http.Client
}

// NewGatewayClient creates a Gateway API client.
func NewGatewayClient(gatewayURL string) *GatewayClient {
	return &GatewayClient{
		gatewayURL: gatewayURL,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// GatewayNode is the Gateway API response for a single node.
type GatewayNode struct {
	NodeID        string                 `json:"node_id"`
	Name          string                 `json:"name"`
	Type          string                 `json:"type"`
	IP            string                 `json:"ip"`
	Port          int                    `json:"port"`
	Status        string                 `json:"status"`
	ActiveStreams int                    `json:"active_streams"`
	MaxStreams    int                    `json:"max_streams"`
	CPUPercent    float64                `json:"cpu_percent"`
	MemPercent    float64                `json:"mem_percent"`
	DiskPercent   float64                `json:"disk_percent"`
	Load1         float64                `json:"load_1"`
	Uptime        int64                  `json:"uptime_seconds"`
	Models        []string               `json:"models"`
	Capabilities  map[string]interface{} `json:"capabilities"`
	LastHeartbeat string                 `json:"last_heartbeat"`
}

// ListNodes fetches all registered nodes from Gateway.
func (c *GatewayClient) ListNodes(ctx context.Context) ([]GatewayNode, error) {
	url := c.gatewayURL + "/api/v1/nodes"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gateway list nodes: %w", err)
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("gateway list nodes: status %d: %s", resp.StatusCode, data)
	}

	var result struct {
		Nodes []GatewayNode `json:"nodes"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("decode nodes: %w", err)
	}
	return result.Nodes, nil
}

// GetNode fetches a single node's status from Gateway.
func (c *GatewayClient) GetNode(ctx context.Context, nodeID string) (*GatewayNode, error) {
	url := fmt.Sprintf("%s/api/v1/nodes/%s", c.gatewayURL, nodeID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gateway get node: %w", err)
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("gateway get node %s: status %d: %s", nodeID, resp.StatusCode, data)
	}

	var node GatewayNode
	if err := json.Unmarshal(data, &node); err != nil {
		return nil, fmt.Errorf("decode node: %w", err)
	}
	return &node, nil
}

// ListWorkers implements scheduler.WorkerLister for Hub's scheduler.
func (c *GatewayClient) ListWorkers(ctx context.Context) ([]scheduler.WorkerInfo, error) {
	nodes, err := c.ListNodes(ctx)
	if err != nil {
		return nil, err
	}

	var workers []scheduler.WorkerInfo
	for _, n := range nodes {
		if n.Type != "worker" {
			continue
		}
		maxStreams := n.MaxStreams
		if maxStreams == 0 {
			maxStreams = 4
		}
		workers = append(workers, scheduler.WorkerInfo{
			NodeID:        n.NodeID,
			IP:            n.IP,
			Port:          n.Port,
			ActiveStreams: n.ActiveStreams,
			MaxStreams:    maxStreams,
			CPUPercent:    n.CPUPercent,
			MemPercent:    n.MemPercent,
			Online:        n.Status == "online" || n.Status == "ok",
		})
	}
	return workers, nil
}

// PushStream implements scheduler.StreamPusher — tells a Worker to start pulling.
func (c *GatewayClient) PushStream(ctx context.Context, workerIP string, workerPort int, cameraID, rtspURL string) error {
	url := fmt.Sprintf("http://%s:%d/api/v1/streams", workerIP, workerPort)
	body, _ := json.Marshal(map[string]string{
		"camera_id": cameraID,
		"url":       rtspURL,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("push stream to %s:%d: %w", workerIP, workerPort, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("push stream: status %d: %s", resp.StatusCode, data)
	}
	return nil
}

// RemoveStream tells a Worker to stop a camera stream.
func (c *GatewayClient) RemoveStream(ctx context.Context, workerIP string, workerPort int, cameraID string) error {
	url := fmt.Sprintf("http://%s:%d/api/v1/streams/%s", workerIP, workerPort, cameraID)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("remove stream from %s:%d: %w", workerIP, workerPort, err)
	}
	resp.Body.Close()
	return nil
}
