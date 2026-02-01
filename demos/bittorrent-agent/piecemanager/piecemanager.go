package piecemanager

import (
	"crypto/sha1"
	"fmt"
	"os"
	"sync"

	"github.com/example/bittorrent/metainfo"
)

const BlockSize = 16384

// PieceState represents the state of a piece.
type PieceState int

const (
	StatePending PieceState = iota
	StateInProgress
	StateComplete
	StateVerified
)

// Block represents a block within a piece.
type Block struct {
	Index    uint32
	Begin    uint32
	Data     []byte
	Received bool
}

// Piece represents a piece in the torrent.
type Piece struct {
	Index   uint32
	Size    int64
	Hash    [20]byte
	State   PieceState
	Blocks  []*Block
	Data    []byte
	lock    sync.Mutex
}

// PieceManager manages the download progress and disk I/O.
type PieceManager struct {
	pieces      []*Piece
	files       []*os.File
	fileSizes   []int64
	pieceLength int64
	totalSize   int64

	// Statistics
	bytesDownloaded int64
	lock            sync.Mutex
}

// NewPieceManager creates a new piece manager.
func NewPieceManager(meta *metainfo.TorrentMeta, outputDir string) (*PieceManager, error) {
	pm := &PieceManager{
		pieceLength: meta.PieceLength,
		totalSize:   meta.TotalLength,
	}

	// Create output directory if it doesn't exist
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	// Open or create output files
	var files []*os.File
	var fileSizes []int64

	for _, file := range meta.Files {
		filePath := outputDir + "/" + file.Path
		// Create parent directories
		dir := ""
		for _, part := range file.Path {
			if part == '/' {
				dir += string(part)
				os.MkdirAll(outputDir+"/"+dir, 0755)
			} else {
				dir += string(part)
			}
		}

		// Create or open file
		f, err := os.OpenFile(filePath, os.O_CREATE|os.O_RDWR, 0644)
		if err != nil {
			return nil, fmt.Errorf("failed to create file %s: %w", filePath, err)
		}

		// Pre-allocate space
		if err := f.Truncate(file.Length); err != nil {
			f.Close()
			return nil, fmt.Errorf("failed to allocate space for %s: %w", filePath, err)
		}

		files = append(files, f)
		fileSizes = append(fileSizes, file.Length)
	}

	pm.files = files
	pm.fileSizes = fileSizes

	// Create pieces
	numPieces := len(meta.Pieces)
	pm.pieces = make([]*Piece, numPieces)

	for i := 0; i < numPieces; i++ {
		piece := &Piece{
			Index: uint32(i),
			Hash:  meta.Pieces[i],
			State: StatePending,
		}

		// Calculate piece size
		startOffset := int64(i) * meta.PieceLength
		endOffset := startOffset + meta.PieceLength
		if endOffset > meta.TotalLength {
			endOffset = meta.TotalLength
		}
		piece.Size = endOffset - startOffset

		// Create blocks for this piece
		numBlocks := (piece.Size + BlockSize - 1) / BlockSize
		piece.Blocks = make([]*Block, numBlocks)

		for j := int64(0); j < numBlocks; j++ {
			blockBegin := j * BlockSize
			blockSize := BlockSize
			if blockBegin+int64(blockSize) > piece.Size {
				blockSize = int(piece.Size - blockBegin)
			}

			piece.Blocks[j] = &Block{
				Index:    uint32(i),
				Begin:    uint32(blockBegin),
				Data:     make([]byte, blockSize),
				Received: false,
			}
		}

		pm.pieces[i] = piece
	}

	return pm, nil
}

// GetPendingBlock returns a pending block to download.
func (pm *PieceManager) GetPendingBlock() *Block {
	pm.lock.Lock()
	defer pm.lock.Unlock()

	for _, piece := range pm.pieces {
		if piece.State == StatePending || piece.State == StateInProgress {
			piece.lock.Lock()
			for _, block := range piece.Blocks {
				if !block.Received {
					piece.State = StateInProgress
					piece.lock.Unlock()
					return block
				}
			}
			piece.lock.Unlock()
		}
	}
	return nil
}

// ReceiveBlock receives a block of data for a piece.
func (pm *PieceManager) ReceiveBlock(pieceIndex uint32, begin uint32, data []byte) error {
	if pieceIndex >= uint32(len(pm.pieces)) {
		return fmt.Errorf("invalid piece index: %d", pieceIndex)
	}

	piece := pm.pieces[pieceIndex]
	piece.lock.Lock()
	defer piece.lock.Unlock()

	// Find the corresponding block
	blockIdx := begin / BlockSize
	if blockIdx >= uint32(len(piece.Blocks)) {
		return fmt.Errorf("invalid block index for piece %d", pieceIndex)
	}

	block := piece.Blocks[blockIdx]
	if block.Received {
		return nil // Already received
	}

	// Copy data to block
	if len(data) != len(block.Data) {
		return fmt.Errorf("block size mismatch for piece %d block %d", pieceIndex, blockIdx)
	}
	copy(block.Data, data)
	block.Received = true

	// Check if piece is complete
	allReceived := true
	for _, b := range piece.Blocks {
		if !b.Received {
			allReceived = false
			break
		}
	}

	if allReceived {
		piece.State = StateComplete
	}

	return nil
}

// VerifyPiece verifies a piece by computing its SHA1 hash.
func (pm *PieceManager) VerifyPiece(pieceIndex uint32) (bool, error) {
	if pieceIndex >= uint32(len(pm.pieces)) {
		return false, fmt.Errorf("invalid piece index: %d", pieceIndex)
	}

	piece := pm.pieces[pieceIndex]
	piece.lock.Lock()

	if piece.State != StateComplete {
		piece.lock.Unlock()
		return false, fmt.Errorf("piece %d is not complete", pieceIndex)
	}

	// Concatenate all block data
	piece.Data = make([]byte, piece.Size)
	offset := 0
	for _, block := range piece.Blocks {
		copy(piece.Data[offset:], block.Data)
		offset += len(block.Data)
	}

	// Compute SHA1
	hash := sha1.Sum(piece.Data)
	piece.lock.Unlock()

	if hash != piece.Hash {
		return false, nil
	}

	piece.lock.Lock()
	piece.State = StateVerified
	piece.lock.Unlock()

	return true, nil
}

// WritePiece writes a verified piece to disk.
func (pm *PieceManager) WritePiece(pieceIndex uint32) error {
	if pieceIndex >= uint32(len(pm.pieces)) {
		return fmt.Errorf("invalid piece index: %d", pieceIndex)
	}

	piece := pm.pieces[pieceIndex]
	piece.lock.Lock()
	defer piece.lock.Unlock()

	if piece.State != StateVerified {
		return fmt.Errorf("piece %d is not verified", pieceIndex)
	}

	// Calculate offset in the overall file
	_ = int64(pieceIndex) * pm.pieceLength
	dataOffset := int64(0)

	// Write to appropriate files
	fileIdx := 0
	currentFileOffset := int64(0)

	for dataOffset < int64(len(piece.Data)) && fileIdx < len(pm.files) {
		// Calculate how much to write to this file
		remainingInFile := pm.fileSizes[fileIdx] - currentFileOffset
		remainingData := int64(len(piece.Data)) - dataOffset

		toWrite := remainingInFile
		if toWrite > remainingData {
			toWrite = remainingData
		}

		// Write to file
		_, err := pm.files[fileIdx].WriteAt(piece.Data[dataOffset:dataOffset+toWrite], currentFileOffset)
		if err != nil {
			return fmt.Errorf("failed to write piece %d to file %d: %w", pieceIndex, fileIdx, err)
		}

		dataOffset += toWrite
		currentFileOffset += toWrite

		// Move to next file if current is full
		if currentFileOffset >= pm.fileSizes[fileIdx] {
			fileIdx++
			currentFileOffset = 0
		}
	}

	pm.lock.Lock()
	pm.bytesDownloaded += piece.Size
	pm.lock.Unlock()

	return nil
}

// GetProgress returns download progress.
func (pm *PieceManager) GetProgress() (int64, int64) {
	pm.lock.Lock()
	defer pm.lock.Unlock()
	return pm.bytesDownloaded, pm.totalSize
}

// GetPieceState returns the state of a piece.
func (pm *PieceManager) GetPieceState(pieceIndex uint32) PieceState {
	if pieceIndex >= uint32(len(pm.pieces)) {
		return StatePending
	}
	return pm.pieces[pieceIndex].State
}

// Close closes all files.
func (pm *PieceManager) Close() error {
	for _, f := range pm.files {
		if err := f.Close(); err != nil {
			return err
		}
	}
	return nil
}

// GetNumPieces returns the total number of pieces.
func (pm *PieceManager) GetNumPieces() int {
	return len(pm.pieces)
}
