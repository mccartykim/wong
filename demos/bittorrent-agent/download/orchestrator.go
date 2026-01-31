package download

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/example/bittorrent/metainfo"
	"github.com/example/bittorrent/peer"
	"github.com/example/bittorrent/piecemanager"
	"github.com/example/bittorrent/tracker"
)

// TrackerAnnouncer defines the interface for announcing to a tracker.
type TrackerAnnouncer interface {
	Announce(req *tracker.Announce) (*tracker.AnnounceResponse, error)
}

// Orchestrator manages the download of a torrent.
type Orchestrator struct {
	meta            *metainfo.TorrentMeta
	pieceManager    *piecemanager.PieceManager
	trackerClient   TrackerAnnouncer
	peerID          [20]byte
	listenPort      uint16
	maxPeers        int

	// State
	peers           map[string]*peer.Peer
	peersLock       sync.Mutex

	// Progress tracking
	startTime       time.Time
	bytesDownloaded int64
	bytesTotal      int64

	// Shutdown
	cancel          context.CancelFunc
	wg              sync.WaitGroup
}

// Config represents configuration for the orchestrator.
type Config struct {
	Meta         *metainfo.TorrentMeta
	OutputDir    string
	TrackerURL   string
	ListenPort   uint16
	MaxPeers     int
}

// NewOrchestrator creates a new download orchestrator.
func NewOrchestrator(cfg *Config) (*Orchestrator, error) {
	// Create piece manager
	pm, err := piecemanager.NewPieceManager(cfg.Meta, cfg.OutputDir)
	if err != nil {
		return nil, fmt.Errorf("failed to create piece manager: %w", err)
	}

	// Create tracker client
	trackerClient := tracker.NewClient(cfg.Meta.Announce)

	orch := &Orchestrator{
		meta:          cfg.Meta,
		pieceManager:  pm,
		trackerClient: trackerClient,
		peerID:        tracker.GeneratePeerID(),
		listenPort:    cfg.ListenPort,
		maxPeers:      cfg.MaxPeers,
		peers:         make(map[string]*peer.Peer),
		startTime:     time.Now(),
		bytesTotal:    cfg.Meta.TotalLength,
	}

	return orch, nil
}

// Start begins the download orchestration.
func (o *Orchestrator) Start(ctx context.Context) error {
	// Create a cancellable context
	ctx, cancel := context.WithCancel(ctx)
	o.cancel = cancel

	// Start tracker announce loop
	o.wg.Add(1)
	go o.trackerAnnounceLoop(ctx)

	// Start main download loop
	o.wg.Add(1)
	go o.downloadLoop(ctx)

	// Start progress reporter
	o.wg.Add(1)
	go o.progressReporter(ctx)

	return nil
}

// Wait blocks until the download is complete or cancelled.
func (o *Orchestrator) Wait() error {
	o.wg.Wait()
	return o.pieceManager.Close()
}

// Stop stops the download gracefully.
func (o *Orchestrator) Stop() {
	if o.cancel != nil {
		o.cancel()
	}
	o.Wait()
}

// Private helper methods

func (o *Orchestrator) trackerAnnounceLoop(ctx context.Context) {
	defer o.wg.Done()

	// Initial announce
	resp, err := o.announceToTracker("started")
	if err != nil {
		fmt.Printf("Tracker announce error: %v\n", err)
		return
	}

	// Connect to initial peers
	o.connectToPeers(ctx, resp.Peers)

	// Re-announce on interval
	ticker := time.NewTicker(time.Duration(resp.Interval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			resp, err := o.announceToTracker("")
			if err != nil {
				fmt.Printf("Tracker announce error: %v\n", err)
				continue
			}
			o.connectToPeers(ctx, resp.Peers)
			ticker.Reset(time.Duration(resp.Interval) * time.Second)
		}
	}
}

func (o *Orchestrator) announceToTracker(event string) (*tracker.AnnounceResponse, error) {
	downloaded, total := o.pieceManager.GetProgress()
	left := total - downloaded

	req := &tracker.Announce{
		InfoHash:   o.meta.InfoHash,
		PeerID:     o.peerID,
		Port:       o.listenPort,
		Uploaded:   0, // Simplified
		Downloaded: downloaded,
		Left:       left,
		Event:      event,
	}

	return o.trackerClient.Announce(req)
}

func (o *Orchestrator) connectToPeers(ctx context.Context, peerList []tracker.PeerInfo) {
	o.peersLock.Lock()
	defer o.peersLock.Unlock()

	for _, peerInfo := range peerList {
		if len(o.peers) >= o.maxPeers {
			break
		}

		peerAddr := peerInfo.IP.String() + ":" + fmt.Sprintf("%d", peerInfo.Port)

		// Skip if already connected
		if _, ok := o.peers[peerAddr]; ok {
			continue
		}

		// Connect to peer
		p := peer.NewPeer(peerInfo.IP, peerInfo.Port)
		p.SetNumPieces(o.pieceManager.GetNumPieces())

		if err := p.Connect(10 * time.Second); err != nil {
			continue
		}

		if err := p.Handshake(o.meta.InfoHash, o.peerID); err != nil {
			p.Close()
			continue
		}

		o.peers[peerAddr] = p

		// Start peer handler
		o.wg.Add(1)
		go o.handlePeer(ctx, p)
	}
}

func (o *Orchestrator) handlePeer(ctx context.Context, p *peer.Peer) {
	defer o.wg.Done()
	defer p.Close()

	// Send interested
	if err := p.SendInterested(); err != nil {
		return
	}

	msgChan := p.ReceiveMessages()
	errChan := p.ReceiveErrors()

	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-msgChan:
			if !ok {
				return
			}
			o.handlePeerMessage(p, msg)
		case err, ok := <-errChan:
			if !ok {
				return
			}
			if err != nil {
				return
			}
		}
	}
}

func (o *Orchestrator) handlePeerMessage(p *peer.Peer, msg *peer.Message) {
	// Handle messages as needed
	// For now, mainly track state changes
}

func (o *Orchestrator) downloadLoop(ctx context.Context) {
	defer o.wg.Done()

	// Sequential piece selection strategy
	_ = 0

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Find a piece to download
		piece := o.pieceManager.GetPendingBlock()
		if piece == nil {
			// All pieces downloaded, verify and write
			downloaded, total := o.pieceManager.GetProgress()
			if downloaded >= total {
				fmt.Println("Download complete!")
				return
			}

			// Wait a bit before checking again
			time.Sleep(100 * time.Millisecond)
			continue
		}

		// Find a peer that has this piece and is not choking
		o.peersLock.Lock()
		var selectedPeer *peer.Peer
		for _, p := range o.peers {
			if p.HasPiece(piece.Index) && !p.PeerChoking {
				selectedPeer = p
				break
			}
		}
		o.peersLock.Unlock()

		if selectedPeer == nil {
			// No suitable peer, wait a bit
			time.Sleep(100 * time.Millisecond)
			continue
		}

		// Request the block
		if err := selectedPeer.SendRequest(piece.Index, piece.Begin, uint32(len(piece.Data))); err != nil {
			continue
		}

		// Wait for piece data (simplified - in real implementation, handle async)
		// For now, just continue
	}
}

func (o *Orchestrator) progressReporter(ctx context.Context) {
	defer o.wg.Done()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			downloaded, total := o.pieceManager.GetProgress()
			percentComplete := float64(downloaded) / float64(total) * 100
			elapsed := time.Since(o.startTime)
			speed := float64(downloaded) / elapsed.Seconds() / 1024 / 1024 // MB/s

			fmt.Printf("\rProgress: %.1f%% (%d/%d bytes) | Speed: %.2f MB/s",
				percentComplete, downloaded, total, speed)
		}
	}
}
