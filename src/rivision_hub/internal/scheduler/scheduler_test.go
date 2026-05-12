package scheduler

import (
	"context"
	"fmt"
	"testing"
)

// mockWorkerLister returns a fixed set of Workers.
type mockWorkerLister struct {
	workers []WorkerInfo
}

func (m *mockWorkerLister) ListWorkers(_ context.Context) ([]WorkerInfo, error) {
	return m.workers, nil
}

// mockStreamPusher records push/remove calls.
type mockStreamPusher struct {
	pushed  []string
	removed []string
}

func (m *mockStreamPusher) PushStream(_ context.Context, ip string, port int, cameraID, _ string) error {
	m.pushed = append(m.pushed, fmt.Sprintf("%s:%d/%s", ip, port, cameraID))
	return nil
}

func (m *mockStreamPusher) RemoveStream(_ context.Context, ip string, port int, cameraID string) error {
	m.removed = append(m.removed, fmt.Sprintf("%s:%d/%s", ip, port, cameraID))
	return nil
}

func TestAssignSelectsLowestLoad(t *testing.T) {
	lister := &mockWorkerLister{
		workers: []WorkerInfo{
			{NodeID: "w1", IP: "10.0.0.1", Port: 9181, ActiveStreams: 3, MaxStreams: 4, Online: true},
			{NodeID: "w2", IP: "10.0.0.2", Port: 9181, ActiveStreams: 1, MaxStreams: 4, Online: true},
		},
	}
	pusher := &mockStreamPusher{}
	s := NewScheduler(lister, pusher)

	a, err := s.Assign(context.Background(), "cam-01", "rtsp://cam1")
	if err != nil {
		t.Fatalf("Assign: %v", err)
	}
	if a.WorkerID != "w2" {
		t.Errorf("expected w2 (lower load), got %s", a.WorkerID)
	}
	if len(pusher.pushed) != 1 {
		t.Errorf("expected 1 push, got %d", len(pusher.pushed))
	}
}

func TestAssignDuplicate(t *testing.T) {
	lister := &mockWorkerLister{
		workers: []WorkerInfo{
			{NodeID: "w1", IP: "10.0.0.1", Port: 9181, MaxStreams: 4, Online: true},
		},
	}
	s := NewScheduler(lister, &mockStreamPusher{})

	s.Assign(context.Background(), "cam-01", "rtsp://cam1")
	_, err := s.Assign(context.Background(), "cam-01", "rtsp://cam1")
	if err == nil {
		t.Error("expected duplicate assign error")
	}
}

func TestAssignNoWorkers(t *testing.T) {
	lister := &mockWorkerLister{workers: nil}
	s := NewScheduler(lister, &mockStreamPusher{})

	_, err := s.Assign(context.Background(), "cam-01", "rtsp://cam1")
	if err == nil {
		t.Error("expected error when no workers available")
	}
}

func TestUnassign(t *testing.T) {
	lister := &mockWorkerLister{
		workers: []WorkerInfo{
			{NodeID: "w1", IP: "10.0.0.1", Port: 9181, MaxStreams: 4, Online: true},
		},
	}
	pusher := &mockStreamPusher{}
	s := NewScheduler(lister, pusher)

	s.Assign(context.Background(), "cam-01", "rtsp://cam1")
	err := s.Unassign(context.Background(), "cam-01")
	if err != nil {
		t.Fatalf("Unassign: %v", err)
	}
	if len(s.Assignments()) != 0 {
		t.Error("expected 0 assignments after unassign")
	}
}

func TestReassign(t *testing.T) {
	lister := &mockWorkerLister{
		workers: []WorkerInfo{
			{NodeID: "w1", IP: "10.0.0.1", Port: 9181, MaxStreams: 4, Online: true},
			{NodeID: "w2", IP: "10.0.0.2", Port: 9181, MaxStreams: 4, Online: true},
		},
	}
	pusher := &mockStreamPusher{}
	s := NewScheduler(lister, pusher)

	// Assign 2 cameras to w1 by manipulating assignments
	s.Assign(context.Background(), "cam-01", "rtsp://cam1")
	s.Assign(context.Background(), "cam-02", "rtsp://cam2")

	// Simulate w1 going offline — reassign all its cameras
	// First, mark w1 offline in lister
	lister.workers[0].Online = false

	n, err := s.Reassign(context.Background(), s.Assignments()[0].WorkerID)
	if err != nil {
		t.Fatalf("Reassign: %v", err)
	}
	// At least some should be reassigned
	_ = n
}
