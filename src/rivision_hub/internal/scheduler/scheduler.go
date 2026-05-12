// Package scheduler implements the stream scheduler engine (v6_design §5.6).
// It assigns cameras to Workers via load-balanced selection, handles failover
// when Workers go offline, and re-distributes streams accordingly.
package scheduler

import (
	"context"
	"fmt"
	"log"
	"sort"
	"sync"
)

// WorkerInfo represents a Worker node's current state for scheduling.
type WorkerInfo struct {
	NodeID        string
	IP            string
	Port          int
	OwlURL        string  // OWL API URL on this Worker (e.g. http://192.168.1.10:15123)
	ZLMHost       string  // Worker ZLM IP for receiving RTP (SIP-media split)
	ZLMHTTPPort   int     // Worker ZLM HTTP API port (default 80)
	ZLMRTSPPort   int     // Worker ZLM RTSP port (default 554)
	ActiveStreams int
	MaxStreams    int
	CPUPercent    float64
	MemPercent    float64
	Online        bool
}

// CameraAssignment maps a camera to its assigned Worker.
type CameraAssignment struct {
	CameraID  string `json:"camera_id"`
	WorkerID  string `json:"worker_id"`
	RTSPURL   string `json:"rtsp_url"`
	Status    string `json:"status"` // "assigned" | "pending" | "failed"
}

// WorkerLister returns the current list of online Workers (from Gateway).
type WorkerLister interface {
	ListWorkers(ctx context.Context) ([]WorkerInfo, error)
}

// StreamPusher pushes a stream assignment to a Worker.
type StreamPusher interface {
	PushStream(ctx context.Context, workerIP string, workerPort int, cameraID, rtspURL string) error
	RemoveStream(ctx context.Context, workerIP string, workerPort int, cameraID string) error
}

// Scheduler assigns cameras to Workers using load-balanced selection (§5.6).
type Scheduler struct {
	workerLister WorkerLister
	streamPusher StreamPusher

	mu          sync.RWMutex
	assignments map[string]*CameraAssignment // cameraID → assignment
}

// NewScheduler creates a stream scheduler.
func NewScheduler(lister WorkerLister, pusher StreamPusher) *Scheduler {
	return &Scheduler{
		workerLister: lister,
		streamPusher: pusher,
		assignments:  make(map[string]*CameraAssignment),
	}
}

// Assign picks the best Worker for a camera and pushes the stream.
func (s *Scheduler) Assign(ctx context.Context, cameraID, rtspURL string) (*CameraAssignment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Already assigned?
	if a, ok := s.assignments[cameraID]; ok {
		return a, fmt.Errorf("camera %s already assigned to %s", cameraID, a.WorkerID)
	}

	workers, err := s.workerLister.ListWorkers(ctx)
	if err != nil {
		return nil, fmt.Errorf("list workers: %w", err)
	}

	worker, err := selectBestWorker(workers)
	if err != nil {
		return nil, err
	}

	// Push stream to selected Worker
	if err := s.streamPusher.PushStream(ctx, worker.IP, worker.Port, cameraID, rtspURL); err != nil {
		return nil, fmt.Errorf("push stream to %s: %w", worker.NodeID, err)
	}

	a := &CameraAssignment{
		CameraID: cameraID,
		WorkerID: worker.NodeID,
		RTSPURL:  rtspURL,
		Status:   "assigned",
	}
	s.assignments[cameraID] = a

	log.Printf("[scheduler] assigned %s → %s (%s:%d, %d/%d streams)",
		cameraID, worker.NodeID, worker.IP, worker.Port,
		worker.ActiveStreams+1, worker.MaxStreams)
	return a, nil
}

// Unassign removes a camera assignment and tells the Worker to stop the stream.
func (s *Scheduler) Unassign(ctx context.Context, cameraID string) error {
	s.mu.Lock()
	a, ok := s.assignments[cameraID]
	if !ok {
		s.mu.Unlock()
		return fmt.Errorf("camera %s not assigned", cameraID)
	}
	delete(s.assignments, cameraID)
	s.mu.Unlock()

	workers, _ := s.workerLister.ListWorkers(ctx)
	for _, w := range workers {
		if w.NodeID == a.WorkerID {
			s.streamPusher.RemoveStream(ctx, w.IP, w.Port, cameraID)
			break
		}
	}

	log.Printf("[scheduler] unassigned %s from %s", cameraID, a.WorkerID)
	return nil
}

// Reassign handles Worker failover: re-distributes all cameras from offlineWorkerID.
func (s *Scheduler) Reassign(ctx context.Context, offlineWorkerID string) (int, error) {
	s.mu.Lock()
	var toReassign []*CameraAssignment
	for _, a := range s.assignments {
		if a.WorkerID == offlineWorkerID {
			toReassign = append(toReassign, a)
		}
	}
	// Remove old assignments
	for _, a := range toReassign {
		delete(s.assignments, a.CameraID)
	}
	s.mu.Unlock()

	reassigned := 0
	for _, a := range toReassign {
		if _, err := s.Assign(ctx, a.CameraID, a.RTSPURL); err != nil {
			log.Printf("[scheduler] reassign %s failed: %v", a.CameraID, err)
			continue
		}
		reassigned++
	}

	log.Printf("[scheduler] reassigned %d/%d cameras from offline %s",
		reassigned, len(toReassign), offlineWorkerID)
	return reassigned, nil
}

// Assignments returns a snapshot of all current assignments.
func (s *Scheduler) Assignments() []CameraAssignment {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]CameraAssignment, 0, len(s.assignments))
	for _, a := range s.assignments {
		result = append(result, *a)
	}
	return result
}

// WorkerAssignments returns cameras assigned to a specific Worker.
func (s *Scheduler) WorkerAssignments(workerID string) []CameraAssignment {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []CameraAssignment
	for _, a := range s.assignments {
		if a.WorkerID == workerID {
			result = append(result, *a)
		}
	}
	return result
}

// selectBestWorker picks the online Worker with the lowest load and available capacity.
func selectBestWorker(workers []WorkerInfo) (*WorkerInfo, error) {
	var candidates []WorkerInfo
	for _, w := range workers {
		if w.Online && w.ActiveStreams < w.MaxStreams {
			candidates = append(candidates, w)
		}
	}
	if len(candidates) == 0 {
		return nil, fmt.Errorf("no available workers (all offline or at capacity)")
	}

	// Sort by active streams ascending, then CPU ascending
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].ActiveStreams != candidates[j].ActiveStreams {
			return candidates[i].ActiveStreams < candidates[j].ActiveStreams
		}
		return candidates[i].CPUPercent < candidates[j].CPUPercent
	})

	return &candidates[0], nil
}
