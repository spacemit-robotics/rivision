// Package search — hierarchical.go implements 2-level Hub hierarchy for 500+ store scale (G6).
// Central Hub treats Regional Hubs as "big Workers" — the same federated search
// protocol works transparently since Regional Hubs expose the same /api/v1/search API.
package search

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"
)

// HubMode defines the Hub's role in a hierarchy.
type HubMode string

const (
	HubModeStandalone HubMode = "standalone"   // default: single Hub
	HubModeRegional   HubMode = "regional"     // manages Workers, reports to Central
	HubModeCentral    HubMode = "central"      // manages Regional Hubs
)

// HierarchicalConfig configures multi-level Hub hierarchy.
type HierarchicalConfig struct {
	Mode       HubMode `yaml:"mode" json:"mode"`                  // standalone|regional|central
	ParentURL  string  `yaml:"parent_url" json:"parent_url"`      // regional → central URL
	RegionName string  `yaml:"region_name" json:"region_name"`    // e.g. "华东", "华南"
}

// RegionalHubInfo describes a registered Regional Hub (from Central Hub's perspective).
type RegionalHubInfo struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	URL        string    `json:"url"`        // base URL e.g. http://regional-1:9280
	RegionName string    `json:"region_name"`
	WorkerCount int      `json:"worker_count"`
	CameraCount int      `json:"camera_count"`
	Healthy    bool      `json:"healthy"`
	LastSeen   time.Time `json:"last_seen"`
}

// HierarchicalSearch extends FederatedSearch to support 2-level hierarchy.
type HierarchicalSearch struct {
	*FederatedSearch
	mode         HubMode
	parentURL    string
	regionName   string
	client       *http.Client

	mu           sync.RWMutex
	regionalHubs map[string]*RegionalHubInfo // Central Hub: registered regional hubs
}

// NewHierarchicalSearch creates a hierarchical search engine.
func NewHierarchicalSearch(federated *FederatedSearch, cfg HierarchicalConfig) *HierarchicalSearch {
	mode := cfg.Mode
	if mode == "" {
		mode = HubModeStandalone
	}
	return &HierarchicalSearch{
		FederatedSearch: federated,
		mode:            mode,
		parentURL:       cfg.ParentURL,
		regionName:      cfg.RegionName,
		client:          &http.Client{Timeout: 60 * time.Second},
		regionalHubs:    make(map[string]*RegionalHubInfo),
	}
}

// RegisterRegionalHub adds a Regional Hub to the Central Hub's registry.
func (hs *HierarchicalSearch) RegisterRegionalHub(hub RegionalHubInfo) {
	hs.mu.Lock()
	defer hs.mu.Unlock()
	hub.Healthy = true
	hub.LastSeen = time.Now()
	hs.regionalHubs[hub.ID] = &hub
	log.Printf("[hierarchical] registered Regional Hub: %s (%s) at %s", hub.ID, hub.RegionName, hub.URL)
}

// UnregisterRegionalHub removes a Regional Hub.
func (hs *HierarchicalSearch) UnregisterRegionalHub(hubID string) {
	hs.mu.Lock()
	defer hs.mu.Unlock()
	delete(hs.regionalHubs, hubID)
}

// ListRegionalHubs returns all registered Regional Hubs.
func (hs *HierarchicalSearch) ListRegionalHubs() []RegionalHubInfo {
	hs.mu.RLock()
	defer hs.mu.RUnlock()
	hubs := make([]RegionalHubInfo, 0, len(hs.regionalHubs))
	for _, h := range hs.regionalHubs {
		hubs = append(hubs, *h)
	}
	return hubs
}

// SearchAcrossRegions performs federated search across all Regional Hubs (Central Hub mode).
// Each Regional Hub is treated as a "big Worker" — it already aggregates its own Workers' results.
func (hs *HierarchicalSearch) SearchAcrossRegions(ctx context.Context, localNodes []NodeInfo, req SearchRequest) (*FederatedSearchResponse, error) {
	if hs.mode != HubModeCentral {
		// Non-central mode: just do normal federated search
		return hs.FederatedSearch.Search(ctx, localNodes, req)
	}

	startTime := time.Now()

	// Search local Workers + all Regional Hubs in parallel
	type searchResult struct {
		source  string
		results []SearchResult
		err     error
	}

	hs.mu.RLock()
	hubs := make([]*RegionalHubInfo, 0, len(hs.regionalHubs))
	for _, h := range hs.regionalHubs {
		if h.Healthy {
			hubs = append(hubs, h)
		}
	}
	hs.mu.RUnlock()

	resultsCh := make(chan searchResult, len(hubs)+1)
	var wg sync.WaitGroup

	// Search local Workers (if any)
	if len(localNodes) > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := hs.FederatedSearch.Search(ctx, localNodes, req)
			if err != nil {
				resultsCh <- searchResult{source: "local", err: err}
				return
			}
			resultsCh <- searchResult{source: "local", results: resp.Results}
		}()
	}

	// Search Regional Hubs
	for _, hub := range hubs {
		wg.Add(1)
		go func(h *RegionalHubInfo) {
			defer wg.Done()
			results, err := hs.searchRegionalHub(ctx, h, req)
			resultsCh <- searchResult{source: h.ID, results: results, err: err}
		}(hub)
	}

	wg.Wait()
	close(resultsCh)

	// Aggregate all results
	var allResults []SearchResult
	successCount := 0
	for sr := range resultsCh {
		if sr.err != nil {
			log.Printf("[hierarchical] search %s failed: %v", sr.source, sr.err)
			continue
		}
		successCount++
		allResults = append(allResults, sr.results...)
	}

	// Rank and dedupe
	finalResults := hs.FederatedSearch.rankAndDedupe(allResults, req.Limit)

	return &FederatedSearchResponse{
		Results:      finalResults,
		TotalNodes:   len(localNodes) + len(hubs),
		SuccessNodes: successCount,
		TotalResults: len(allResults),
		SearchTimeMs: time.Since(startTime).Milliseconds(),
	}, nil
}

func (hs *HierarchicalSearch) searchRegionalHub(ctx context.Context, hub *RegionalHubInfo, req SearchRequest) ([]SearchResult, error) {
	url := fmt.Sprintf("%s/api/v1/search/federated", hub.URL)

	body, _ := json.Marshal(req)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := hs.client.Do(httpReq)
	if err != nil {
		// Mark hub as unhealthy
		hs.mu.Lock()
		if h, ok := hs.regionalHubs[hub.ID]; ok {
			h.Healthy = false
		}
		hs.mu.Unlock()
		return nil, fmt.Errorf("search regional hub %s: %w", hub.ID, err)
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("regional hub %s: HTTP %d", hub.ID, resp.StatusCode)
	}

	var fedResp FederatedSearchResponse
	if err := json.Unmarshal(data, &fedResp); err != nil {
		return nil, err
	}

	// Tag all results with the hub's region
	for i := range fedResp.Results {
		if fedResp.Results[i].NodeID == "" {
			fedResp.Results[i].NodeID = hub.ID
		}
	}

	// Update last seen
	hs.mu.Lock()
	if h, ok := hs.regionalHubs[hub.ID]; ok {
		h.LastSeen = time.Now()
		h.Healthy = true
	}
	hs.mu.Unlock()

	return fedResp.Results, nil
}

// HealthCheckRegionalHubs pings all Regional Hubs and updates their health status.
func (hs *HierarchicalSearch) HealthCheckRegionalHubs(ctx context.Context) {
	hs.mu.RLock()
	hubs := make([]*RegionalHubInfo, 0, len(hs.regionalHubs))
	for _, h := range hs.regionalHubs {
		hubs = append(hubs, h)
	}
	hs.mu.RUnlock()

	for _, hub := range hubs {
		url := fmt.Sprintf("%s/api/v1/system/health", hub.URL)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			continue
		}
		resp, err := hs.client.Do(req)
		hs.mu.Lock()
		if h, ok := hs.regionalHubs[hub.ID]; ok {
			if err != nil || resp.StatusCode != http.StatusOK {
				h.Healthy = false
			} else {
				h.Healthy = true
				h.LastSeen = time.Now()
			}
		}
		hs.mu.Unlock()
		if resp != nil {
			resp.Body.Close()
		}
	}
}

// Mode returns the current Hub mode.
func (hs *HierarchicalSearch) Mode() HubMode {
	return hs.mode
}
