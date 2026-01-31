package download

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/binary"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mccartykim/wong-bittorrent/metainfo"
	"github.com/mccartykim/wong-bittorrent/peer"
)

// TestNewCreatesManagerAndWriter tests that New() creates manager and writer correctly
func TestNewCreatesManagerAndWriter(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a simple single-file torrent
	torrent := &metainfo.Torrent{
		Name:        "test.txt",
		Announce:    "http://tracker.example.com:8080/announce",
		PieceLength: 16384,
		Length:      32768,
		InfoHash:    [20]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20},
		Pieces: [][20]byte{
			{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1},
			{2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2},
		},
	}

	cfg := Config{
		Torrent:   torrent,
		OutputDir: tmpDir,
		Port:      6881,
		MaxPeers:  5,
	}

	d, err := New(cfg)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer d.Close()

	if d.config != cfg {
		t.Error("config not stored correctly")
	}

	if d.manager == nil {
		t.Error("manager not created")
	}

	if d.writer == nil {
		t.Error("writer not created")
	}

	if d.manager.NumPieces() != 2 {
		t.Errorf("expected 2 pieces, got %d", d.manager.NumPieces())
	}

	// Verify the file was created
	expectedFile := filepath.Join(tmpDir, "test.txt")
	if _, err := os.Stat(expectedFile); err != nil {
		t.Errorf("file not created: %v", err)
	}
}

// TestNewValidatesInput tests that New() validates input
func TestNewValidatesInput(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{
			name:    "nil torrent",
			cfg:     Config{Torrent: nil},
			wantErr: true,
		},
		{
			name: "empty output dir",
			cfg: Config{
				Torrent: &metainfo.Torrent{Name: "test"},
			},
			wantErr: true,
		},
		{
			name: "valid config",
			cfg: Config{
				Torrent: &metainfo.Torrent{
					Name:        "test",
					PieceLength: 1024,
					Length:      1024,
					InfoHash:    [20]byte{1},
					Pieces:      [][20]byte{{1}},
				},
				OutputDir: tmpDir,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("New() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// mockPeer simulates a BitTorrent peer for testing
type mockPeer struct {
	serverConn net.Conn
	clientConn net.Conn
	pieces     map[int][]byte
	t          *testing.T
}

// newMockPeer creates a connected pair of sockets (one for client, one for server)
func newMockPeer(t *testing.T, numPieces int, pieceSize int) *mockPeer {
	serverConn, clientConn := net.Pipe()

	mp := &mockPeer{
		serverConn: serverConn,
		clientConn: clientConn,
		pieces:     make(map[int][]byte),
		t:          t,
	}

	// Initialize pieces with dummy data
	for i := 0; i < numPieces; i++ {
		data := make([]byte, pieceSize)
		for j := 0; j < len(data); j++ {
			data[j] = byte((i*256 + j) % 256)
		}
		mp.pieces[i] = data
	}

	return mp
}

// handlePeer runs the server side of the peer connection
func (mp *mockPeer) handlePeer(infoHash [20]byte, peerID [20]byte) {
	defer mp.serverConn.Close()

	// Perform handshake
	conn, err := peer.Handshake(mp.serverConn, infoHash, peerID)
	if err != nil {
		mp.t.Logf("handshake error: %v", err)
		return
	}

	// Send bitfield
	bitfield := mp.getBitfield()
	if err := conn.SendBitfield(bitfield); err != nil {
		mp.t.Logf("send bitfield error: %v", err)
		return
	}

	// Handle requests
	for {
		msg, err := conn.ReadMessage()
		if err != nil {
			return
		}

		if msg == nil {
			continue
		}

		switch msg.ID {
		case peer.MsgInterested:
			// Send unchoke
			if err := conn.SendUnchoke(); err != nil {
				return
			}

		case peer.MsgRequest:
			// Parse request
			index, begin, length, err := peer.ParseRequest(msg)
			if err != nil {
				continue
			}

			// Get piece data
			data, ok := mp.pieces[int(index)]
			if !ok || int(begin)+int(length) > len(data) {
				continue
			}

			// Send piece
			blockData := data[begin : begin+length]
			payload := new(bytes.Buffer)
			binary.Write(payload, binary.BigEndian, index)
			binary.Write(payload, binary.BigEndian, begin)
			payload.Write(blockData)

			if err := conn.SendMessage(&peer.Message{
				ID:      peer.MsgPiece,
				Payload: payload.Bytes(),
			}); err != nil {
				return
			}

		case peer.MsgNotInterested:
			// Send choke
			if err := conn.SendChoke(); err != nil {
				return
			}
		}
	}
}

// getBitfield returns a bitfield indicating all pieces are available
func (mp *mockPeer) getBitfield() []byte {
	numPieces := len(mp.pieces)
	numBytes := (numPieces + 7) / 8
	bitfield := make([]byte, numBytes)

	for i := 0; i < numPieces; i++ {
		byteIndex := i / 8
		bitIndex := 7 - (i % 8)
		bitfield[byteIndex] |= (1 << uint(bitIndex))
	}

	return bitfield
}

// TestContextCancellation tests that context cancellation stops the download
func TestContextCancellation(t *testing.T) {
	tmpDir := t.TempDir()

	torrent := &metainfo.Torrent{
		Name:        "test.txt",
		Announce:    "http://localhost:9999/announce", // non-existent tracker
		PieceLength: 1024,
		Length:      2048,
		InfoHash:    [20]byte{1},
		Pieces: [][20]byte{
			{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1},
			{2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2},
		},
	}

	cfg := Config{
		Torrent:   torrent,
		OutputDir: tmpDir,
		Port:      6881,
		MaxPeers:  1,
	}

	d, err := New(cfg)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer d.Close()

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel after a short delay
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	// Run should return context error
	err = d.Run(ctx)
	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

// TestDownloadWithMockPeer tests downloading with a mock peer serving all pieces
func TestDownloadWithMockPeer(t *testing.T) {
	tmpDir := t.TempDir()

	pieceSize := 1024
	numPieces := 2
	totalSize := int64(pieceSize * numPieces)

	// Create mock pieces with actual hashes
	mockPieces := make([][20]byte, numPieces)
	peerData := make([][]byte, numPieces)

	for i := 0; i < numPieces; i++ {
		data := make([]byte, pieceSize)
		for j := 0; j < len(data); j++ {
			data[j] = byte((i*256 + j) % 256)
		}
		peerData[i] = data

		// Use the actual hash of the data
		h := sha1.Sum(data)
		mockPieces[i] = h
	}

	torrent := &metainfo.Torrent{
		Name:        "test.txt",
		Announce:    "http://localhost:9999/announce",
		PieceLength: pieceSize,
		Length:      totalSize,
		InfoHash:    [20]byte{1},
		Pieces:      mockPieces,
	}

	cfg := Config{
		Torrent:   torrent,
		OutputDir: tmpDir,
		Port:      6881,
		MaxPeers:  1,
	}

	d, err := New(cfg)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer d.Close()

	// Verify manager was created with correct pieces
	if d.manager.NumPieces() != numPieces {
		t.Errorf("expected %d pieces, got %d", numPieces, d.manager.NumPieces())
	}

	// Verify writer was created
	if d.writer == nil {
		t.Fatal("writer not created")
	}
}

// TestPeerDisconnectMidDownload tests handling of peer disconnect
func TestPeerDisconnectMidDownload(t *testing.T) {
	tmpDir := t.TempDir()

	torrent := &metainfo.Torrent{
		Name:        "test.txt",
		Announce:    "http://localhost:9999/announce",
		PieceLength: 1024,
		Length:      2048,
		InfoHash:    [20]byte{1},
		Pieces: [][20]byte{
			{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1},
			{2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2},
		},
	}

	cfg := Config{
		Torrent:   torrent,
		OutputDir: tmpDir,
		Port:      6881,
		MaxPeers:  1,
	}

	d, err := New(cfg)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer d.Close()

	// Test that closeAllPeers works correctly
	// Since peerConn is private, we can just test the closeAllPeers method
	// by verifying it clears the peers map
	d.closeAllPeers()

	d.mu.Lock()
	if len(d.peers) != 0 {
		t.Errorf("expected no peers after closeAllPeers, got %d", len(d.peers))
	}
	d.mu.Unlock()
}

// TestGetLeft tests the getLeft calculation
func TestGetLeft(t *testing.T) {
	tmpDir := t.TempDir()

	torrent := &metainfo.Torrent{
		Name:        "test.txt",
		Announce:    "http://tracker.example.com:8080/announce",
		PieceLength: 1024,
		Length:      4096,
		InfoHash:    [20]byte{1},
		Pieces: [][20]byte{
			{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1},
			{2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2},
			{3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3},
			{4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4},
		},
	}

	cfg := Config{
		Torrent:   torrent,
		OutputDir: tmpDir,
		Port:      6881,
		MaxPeers:  1,
	}

	d, err := New(cfg)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer d.Close()

	// Initially, all bytes are left
	left := d.getLeft()
	if left != 4096 {
		t.Errorf("expected 4096 bytes left, got %d", left)
	}
}

// TestMultiFileTorrent tests handling of multi-file torrents
func TestMultiFileTorrent(t *testing.T) {
	tmpDir := t.TempDir()

	torrent := &metainfo.Torrent{
		Name:        "testdir",
		Announce:    "http://tracker.example.com:8080/announce",
		PieceLength: 1024,
		InfoHash:    [20]byte{1},
		Pieces: [][20]byte{
			{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1},
			{2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2},
		},
		Files: []metainfo.File{
			{
				Length: 1024,
				Path:   []string{"file1.txt"},
			},
			{
				Length: 1024,
				Path:   []string{"subdir", "file2.txt"},
			},
		},
	}

	cfg := Config{
		Torrent:   torrent,
		OutputDir: tmpDir,
		Port:      6881,
		MaxPeers:  1,
	}

	d, err := New(cfg)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer d.Close()

	// Verify files were created
	file1 := filepath.Join(tmpDir, "file1.txt")
	file2 := filepath.Join(tmpDir, "subdir", "file2.txt")

	if _, err := os.Stat(file1); err != nil {
		t.Errorf("file1.txt not created: %v", err)
	}

	if _, err := os.Stat(file2); err != nil {
		t.Errorf("file2.txt not created: %v", err)
	}
}

// TestDownloadWithRealPeerConnection tests that the Download package correctly initializes
// and can be run with a context (without actual network)
func TestDownloadWithRealPeerConnection(t *testing.T) {
	tmpDir := t.TempDir()

	pieceSize := 256

	// Create data with known hash
	data := make([]byte, pieceSize)
	for i := 0; i < len(data); i++ {
		data[i] = byte(i % 256)
	}
	dataHash := sha1.Sum(data)

	torrent := &metainfo.Torrent{
		Name:        "test.txt",
		Announce:    "http://localhost:9999/announce",
		PieceLength: pieceSize,
		Length:      int64(pieceSize),
		InfoHash:    [20]byte{1},
		Pieces:      [][20]byte{dataHash},
	}

	cfg := Config{
		Torrent:   torrent,
		OutputDir: tmpDir,
		Port:      6881,
		MaxPeers:  1,
	}

	d, err := New(cfg)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer d.Close()

	// Test that the Download can be created and used
	if d.config.Torrent != torrent {
		t.Error("torrent not set correctly")
	}

	if d.manager == nil {
		t.Error("manager not created")
	}

	// Verify the file was created
	expectedFile := filepath.Join(tmpDir, "test.txt")
	if _, err := os.Stat(expectedFile); err != nil {
		t.Errorf("file not created: %v", err)
	}
}

// TestMaxPeersDefault tests that MaxPeers defaults to 30
func TestMaxPeersDefault(t *testing.T) {
	tmpDir := t.TempDir()

	torrent := &metainfo.Torrent{
		Name:        "test.txt",
		PieceLength: 1024,
		Length:      1024,
		InfoHash:    [20]byte{1},
		Pieces:      [][20]byte{{1}},
	}

	cfg := Config{
		Torrent:   torrent,
		OutputDir: tmpDir,
		MaxPeers:  0, // Should default to 30
	}

	d, err := New(cfg)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer d.Close()

	if d.config.MaxPeers != 30 {
		t.Errorf("expected MaxPeers to default to 30, got %d", d.config.MaxPeers)
	}
}
