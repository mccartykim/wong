package download

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/mccartykim/wong-bittorrent/diskio"
	"github.com/mccartykim/wong-bittorrent/metainfo"
	"github.com/mccartykim/wong-bittorrent/peer"
	"github.com/mccartykim/wong-bittorrent/pieces"
	"github.com/mccartykim/wong-bittorrent/tracker"
)

// Config contains the configuration for a download
type Config struct {
	Torrent   *metainfo.Torrent
	OutputDir string
	Port      uint16
	MaxPeers  int
}

// Download coordinates the full download of a torrent
type Download struct {
	config   Config
	manager  *pieces.Manager
	writer   *diskio.Writer
	peers    map[string]*peerConn // ip:port -> connection
	mu       sync.Mutex
	cancel   context.CancelFunc
}

// peerConn represents a connection to a peer
type peerConn struct {
	conn *peer.Conn
	addr string
}

const (
	// blockSize is the size of each block requested from peers (16KB)
	blockSize = 16 * 1024
	// dialTimeout is the timeout for connecting to a peer
	dialTimeout = 5 * time.Second
	// keepAliveInterval is how often to send keep-alives
	keepAliveInterval = 1 * time.Minute
)

// New creates a new Download instance
func New(cfg Config) (*Download, error) {
	if cfg.Torrent == nil {
		return nil, fmt.Errorf("torrent is required")
	}
	if cfg.OutputDir == "" {
		return nil, fmt.Errorf("output directory is required")
	}
	if cfg.MaxPeers <= 0 {
		cfg.MaxPeers = 30
	}

	// Create pieces manager
	manager := pieces.NewManager(cfg.Torrent.PieceLength, cfg.Torrent.TotalLength(), cfg.Torrent.Pieces)

	// Build file entries for diskio
	var fileEntries []diskio.FileEntry
	var offset int64

	if cfg.Torrent.IsMultiFile() {
		// Multi-file torrent
		for _, file := range cfg.Torrent.Files {
			relPath := ""
			for i, component := range file.Path {
				if i == 0 {
					relPath = component
				} else {
					relPath = relPath + "/" + component
				}
			}
			fileEntries = append(fileEntries, diskio.FileEntry{
				Path:   relPath,
				Length: file.Length,
				Offset: offset,
			})
			offset += file.Length
		}
	} else {
		// Single-file torrent
		fileEntries = append(fileEntries, diskio.FileEntry{
			Path:   cfg.Torrent.Name,
			Length: cfg.Torrent.Length,
			Offset: 0,
		})
	}

	// Create disk writer
	writer, err := diskio.NewWriter(cfg.OutputDir, cfg.Torrent.Name, cfg.Torrent.PieceLength, cfg.Torrent.TotalLength(), fileEntries)
	if err != nil {
		return nil, fmt.Errorf("failed to create disk writer: %w", err)
	}

	return &Download{
		config:  cfg,
		manager: manager,
		writer:  writer,
		peers:   make(map[string]*peerConn),
	}, nil
}

// Run starts the download process and blocks until completion or context cancellation
func (d *Download) Run(ctx context.Context) error {
	// Create a cancellable context
	ctx, d.cancel = context.WithCancel(ctx)
	defer d.cancel()

	// Generate peer ID
	peerID := tracker.GeneratePeerID()

	// Channel to manage peer workers
	peerChan := make(chan tracker.Peer, d.config.MaxPeers)
	var wg sync.WaitGroup

	// Start goroutine to announce and get peers
	wg.Add(1)
	go func() {
		defer wg.Done()
		d.announceLoop(ctx, peerID, peerChan)
	}()

	// Start peer worker goroutines
	for i := 0; i < d.config.MaxPeers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			d.peerWorker(ctx, peerID, peerChan)
		}()
	}

	// Monitor for completion or context cancellation
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			close(peerChan)
			wg.Wait()
			d.closeAllPeers()
			return ctx.Err()
		case <-ticker.C:
			if d.manager.IsComplete() {
				close(peerChan)
				wg.Wait()
				d.closeAllPeers()
				return d.writer.Close()
			}
		}
	}
}

// announceLoop periodically announces to the tracker and sends peers to the peer channel
func (d *Download) announceLoop(ctx context.Context, peerID [20]byte, peerChan chan<- tracker.Peer) {
	announceTicker := time.NewTicker(5 * time.Second)
	defer announceTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-announceTicker.C:
			// Announce to tracker
			req := &tracker.AnnounceRequest{
				AnnounceURL: d.config.Torrent.Announce,
				InfoHash:    d.config.Torrent.InfoHash,
				PeerID:      peerID,
				Port:        d.config.Port,
				Uploaded:    0,
				Downloaded:  int64(d.manager.Downloaded() * d.config.Torrent.PieceLength),
				Left:        d.getLeft(),
				Event:       "",
			}

			resp, err := tracker.Announce(ctx, req)
			if err != nil {
				// Log error but continue
				_ = err
				continue
			}

			// Update announce interval
			if resp.Interval > 0 {
				announceInterval := time.Duration(resp.Interval) * time.Second
				announceTicker.Reset(announceInterval)
			}

			// Send new peers to channel (non-blocking)
			for _, p := range resp.Peers {
				select {
				case peerChan <- p:
				case <-ctx.Done():
					return
				default:
					// Channel full, skip this peer
				}
			}
		}
	}
}

// peerWorker handles a single peer connection and downloads pieces
func (d *Download) peerWorker(ctx context.Context, peerID [20]byte, peerChan <-chan tracker.Peer) {
	for {
		select {
		case <-ctx.Done():
			return
		case p, ok := <-peerChan:
			if !ok {
				return
			}

			// Connect to peer
			addr := fmt.Sprintf("%s:%d", p.IP.String(), p.Port)
			conn, err := d.connectToPeer(ctx, addr, peerID)
			if err != nil {
				continue
			}

			// Register peer
			d.mu.Lock()
			d.peers[addr] = conn
			d.mu.Unlock()

			// Download from peer
			d.downloadFromPeer(ctx, conn)

			// Unregister peer
			d.mu.Lock()
			delete(d.peers, addr)
			d.mu.Unlock()

			conn.conn.Close()
		}
	}
}

// connectToPeer establishes a connection to a peer and performs handshake
func (d *Download) connectToPeer(ctx context.Context, addr string, peerID [20]byte) (*peerConn, error) {
	// Create a dialer with timeout
	dialer := net.Dialer{Timeout: dialTimeout}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to %s: %w", addr, err)
	}

	// Perform handshake
	peerConnection, err := peer.Handshake(conn, d.config.Torrent.InfoHash, peerID)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("handshake with %s failed: %w", addr, err)
	}

	return &peerConn{
		conn: peerConnection,
		addr: addr,
	}, nil
}

// downloadFromPeer downloads pieces from a connected peer
func (d *Download) downloadFromPeer(ctx context.Context, pc *peerConn) {
	// Send interested
	if err := pc.conn.SendInterested(); err != nil {
		return
	}

	// Wait for unchoke with timeout
	unChokeTimeout := time.NewTimer(30 * time.Second)
	defer unChokeTimeout.Stop()

	keepAliveTicker := time.NewTicker(keepAliveInterval)
	defer keepAliveTicker.Stop()

	// Read messages to handle unchoke and bitfield
	for {
		select {
		case <-ctx.Done():
			return
		case <-unChokeTimeout.C:
			return
		case <-keepAliveTicker.C:
			if err := pc.conn.SendKeepAlive(); err != nil {
				return
			}
		default:
		}

		msg, err := pc.conn.ReadMessage()
		if err != nil {
			return
		}

		// Skip keep-alives
		if msg == nil {
			continue
		}

		switch msg.ID {
		case peer.MsgUnchoke:
			pc.conn.Choked = false
			unChokeTimeout.Stop()

		case peer.MsgBitfield:
			pc.conn.Bitfield = msg.Payload

		case peer.MsgHave:
			idx, err := peer.ParseHave(msg)
			if err == nil {
				pc.conn.SetPiece(int(idx))
			}

		case peer.MsgChoke:
			pc.conn.Choked = true
			return

		case peer.MsgPiece:
			idx, begin, data, err := peer.ParsePiece(msg)
			if err == nil {
				d.manager.ReceiveBlock(int(idx), int(begin), data)
			}
		}

		// If unchoked, start requesting pieces
		if !pc.conn.Choked && len(pc.conn.Bitfield) > 0 {
			break
		}
	}

	// Download loop
	for {
		select {
		case <-ctx.Done():
			return
		case <-keepAliveTicker.C:
			if err := pc.conn.SendKeepAlive(); err != nil {
				return
			}
		default:
		}

		// Pick a piece to download
		pieceIdx, haspiece := d.manager.PickPiece(pc.conn.Bitfield)
		if !haspiece {
			// No piece available from this peer, wait a bit and retry
			time.Sleep(100 * time.Millisecond)
			continue
		}

		// Mark as requested
		d.manager.MarkRequested(pieceIdx)

		// Request blocks for this piece
		pieceLen := d.manager.PieceLength(pieceIdx)
		for begin := 0; begin < pieceLen; begin += blockSize {
			len := blockSize
			if begin+len > pieceLen {
				len = pieceLen - begin
			}

			// Request block
			if err := pc.conn.SendRequest(uint32(pieceIdx), uint32(begin), uint32(len)); err != nil {
				return
			}

			msg, err := pc.conn.ReadMessage()
			if err != nil {
				return
			}

			if msg == nil {
				begin -= blockSize // Retry this block
				continue
			}

			switch msg.ID {
			case peer.MsgPiece:
				idx, off, data, err := peer.ParsePiece(msg)
				if err == nil && int(idx) == pieceIdx && int(off) == begin {
					d.manager.ReceiveBlock(int(idx), int(off), data)
				}

			case peer.MsgChoke:
				pc.conn.Choked = true
				return

			case peer.MsgHave:
				idx, _ := peer.ParseHave(msg)
				pc.conn.SetPiece(int(idx))
			}
		}
	}
}

// getLeft calculates how many bytes are left to download
func (d *Download) getLeft() int64 {
	downloaded := int64(d.manager.Downloaded() * d.config.Torrent.PieceLength)
	return d.config.Torrent.TotalLength() - downloaded
}

// closeAllPeers closes all peer connections
func (d *Download) closeAllPeers() {
	d.mu.Lock()
	defer d.mu.Unlock()

	for _, pc := range d.peers {
		pc.conn.Close()
	}
	d.peers = make(map[string]*peerConn)
}

// Close closes the download and releases resources
func (d *Download) Close() error {
	if d.cancel != nil {
		d.cancel()
	}
	d.closeAllPeers()
	return d.writer.Close()
}
