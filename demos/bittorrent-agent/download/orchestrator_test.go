package download

import (
	"context"
	"testing"
	"time"

	"github.com/example/bittorrent/metainfo"
	"github.com/example/bittorrent/tracker"
)

// MockTrackerAnnouncer is a mock implementation of TrackerAnnouncer.
type MockTrackerAnnouncer struct {
	announceCount int
	lastRequest   *tracker.Announce
}

func (m *MockTrackerAnnouncer) Announce(req *tracker.Announce) (*tracker.AnnounceResponse, error) {
	m.announceCount++
	m.lastRequest = req
	return &tracker.AnnounceResponse{
		Interval: 30,
		Peers:    []tracker.PeerInfo{},
	}, nil
}

// TestOrchestratorCreation tests that an orchestrator can be created.
func TestOrchestratorCreation(t *testing.T) {
	meta := &metainfo.TorrentMeta{
		Announce:    "http://example.com/announce",
		PieceLength: 16384,
		TotalLength: 32768,
		Files: []metainfo.FileInfo{
			{Length: 32768, Path: "test.txt"},
		},
		Pieces: make([][20]byte, 2),
	}

	tempDir := t.TempDir()

	cfg := &Config{
		Meta:       meta,
		OutputDir:  tempDir,
		TrackerURL: meta.Announce,
		ListenPort: 6881,
		MaxPeers:   30,
	}

	orch, err := NewOrchestrator(cfg)
	if err != nil {
		t.Fatalf("failed to create orchestrator: %v", err)
	}

	if orch == nil {
		t.Fatal("orchestrator is nil")
	}

	defer orch.pieceManager.Close()
}

// TestOrchestratorStart tests that the orchestrator can start.
func TestOrchestratorStart(t *testing.T) {
	meta := &metainfo.TorrentMeta{
		Announce:    "http://example.com/announce",
		PieceLength: 16384,
		TotalLength: 32768,
		Files: []metainfo.FileInfo{
			{Length: 32768, Path: "test.txt"},
		},
		Pieces: make([][20]byte, 2),
	}

	tempDir := t.TempDir()

	cfg := &Config{
		Meta:       meta,
		OutputDir:  tempDir,
		TrackerURL: meta.Announce,
		ListenPort: 6881,
		MaxPeers:   30,
	}

	orch, err := NewOrchestrator(cfg)
	if err != nil {
		t.Fatalf("failed to create orchestrator: %v", err)
	}
	defer orch.pieceManager.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	err = orch.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start orchestrator: %v", err)
	}

	// Wait a bit for goroutines to start
	time.Sleep(100 * time.Millisecond)

	// Stop the orchestrator
	orch.Stop()
}

// TestOrchestratorTracker tests that the orchestrator announces to tracker.
func TestOrchestratorTracker(t *testing.T) {
	meta := &metainfo.TorrentMeta{
		Announce:    "http://example.com/announce",
		PieceLength: 16384,
		TotalLength: 32768,
		Files: []metainfo.FileInfo{
			{Length: 32768, Path: "test.txt"},
		},
		Pieces: make([][20]byte, 2),
	}

	tempDir := t.TempDir()

	cfg := &Config{
		Meta:       meta,
		OutputDir:  tempDir,
		TrackerURL: meta.Announce,
		ListenPort: 6881,
		MaxPeers:   30,
	}

	orch, err := NewOrchestrator(cfg)
	if err != nil {
		t.Fatalf("failed to create orchestrator: %v", err)
	}
	defer orch.pieceManager.Close()

	// Replace tracker with mock
	mockTracker := &MockTrackerAnnouncer{}
	orch.trackerClient = mockTracker

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	err = orch.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start orchestrator: %v", err)
	}

	// Wait for initial announce
	time.Sleep(100 * time.Millisecond)

	// Stop the orchestrator
	orch.Stop()

	// Verify tracker was called
	if mockTracker.announceCount == 0 {
		t.Error("tracker.Announce was not called")
	}

	if mockTracker.lastRequest == nil {
		t.Fatal("lastRequest is nil")
	}

	// Verify announce request
	if mockTracker.lastRequest.InfoHash != meta.InfoHash {
		t.Error("info_hash mismatch")
	}

	if mockTracker.lastRequest.Port != cfg.ListenPort {
		t.Error("port mismatch")
	}
}

// TestOrchestratorProgress tests that the orchestrator tracks progress.
func TestOrchestratorProgress(t *testing.T) {
	meta := &metainfo.TorrentMeta{
		Announce:    "http://example.com/announce",
		PieceLength: 16384,
		TotalLength: 32768,
		Files: []metainfo.FileInfo{
			{Length: 32768, Path: "test.txt"},
		},
		Pieces: make([][20]byte, 2),
	}

	tempDir := t.TempDir()

	cfg := &Config{
		Meta:       meta,
		OutputDir:  tempDir,
		TrackerURL: meta.Announce,
		ListenPort: 6881,
		MaxPeers:   30,
	}

	orch, err := NewOrchestrator(cfg)
	if err != nil {
		t.Fatalf("failed to create orchestrator: %v", err)
	}
	defer orch.pieceManager.Close()

	// Check initial progress
	downloaded, total := orch.pieceManager.GetProgress()
	if downloaded != 0 {
		t.Errorf("initial downloaded should be 0, got %d", downloaded)
	}
	if total != meta.TotalLength {
		t.Errorf("total should be %d, got %d", meta.TotalLength, total)
	}
}
