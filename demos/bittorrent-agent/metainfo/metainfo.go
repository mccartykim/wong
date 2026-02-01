package metainfo

import (
	"crypto/sha1"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/example/bittorrent/bencode"
)

// TorrentMeta represents parsed metadata from a .torrent file.
type TorrentMeta struct {
	Announce    string      // Tracker URL
	InfoHash    [20]byte    // SHA1 hash of raw bencoded info dict
	PieceLength int64       // Length of each piece in bytes
	Pieces      [][20]byte  // List of piece hashes (each 20 bytes)
	Files       []FileInfo  // For multi-file torrents
	Name        string      // Single filename or directory name
	TotalLength int64       // Total bytes in all files
}

// FileInfo represents a single file in a multi-file torrent.
type FileInfo struct {
	Length int64  // File size in bytes
	Path   string // Relative path (using "/" as separator)
}

// ParseTorrent parses a .torrent file from raw bytes.
func ParseTorrent(data []byte) (*TorrentMeta, error) {
	// Decode the entire torrent
	decoded, err := bencode.Decode(data)
	if err != nil {
		return nil, fmt.Errorf("failed to decode torrent: %w", err)
	}

	torrentDict, ok := decoded.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("torrent root is not a dict")
	}

	// Extract announce
	announceVal, ok := torrentDict["announce"]
	if !ok {
		return nil, fmt.Errorf("announce field missing")
	}
	announce, ok := announceVal.(string)
	if !ok {
		return nil, fmt.Errorf("announce is not a string")
	}

	// Extract info dict and get raw bytes
	infoVal, ok := torrentDict["info"]
	if !ok {
		return nil, fmt.Errorf("info field missing")
	}
	infoDict, ok := infoVal.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("info is not a dict")
	}

	// Get raw bencoded info dict bytes
	rawInfoBytes, err := extractRawInfoDict(data, torrentDict, infoDict)
	if err != nil {
		return nil, fmt.Errorf("failed to extract raw info dict: %w", err)
	}

	// Compute info_hash
	h := sha1.Sum(rawInfoBytes)

	// Parse info dict fields
	pieceLength, err := getInt64(infoDict, "piece length")
	if err != nil {
		return nil, fmt.Errorf("piece length: %w", err)
	}

	piecesStr, err := getString(infoDict, "pieces")
	if err != nil {
		return nil, fmt.Errorf("pieces: %w", err)
	}

	// Parse pieces (each 20 bytes is one SHA1 hash)
	pieces, err := parsePieces(piecesStr)
	if err != nil {
		return nil, fmt.Errorf("invalid pieces: %w", err)
	}

	// Extract name
	name, err := getString(infoDict, "name")
	if err != nil {
		return nil, fmt.Errorf("name: %w", err)
	}

	// Determine single-file or multi-file
	var files []FileInfo
	var totalLength int64

	if filesVal, ok := infoDict["files"]; ok {
		// Multi-file torrent
		filesList, ok := filesVal.([]interface{})
		if !ok {
			return nil, fmt.Errorf("files is not a list")
		}
		files, totalLength, err = parseFilesList(filesList)
		if err != nil {
			return nil, fmt.Errorf("files list: %w", err)
		}
	} else {
		// Single-file torrent
		length, err := getInt64(infoDict, "length")
		if err != nil {
			return nil, fmt.Errorf("length: %w", err)
		}
		totalLength = length
		files = []FileInfo{{Length: length, Path: name}}
	}

	return &TorrentMeta{
		Announce:    announce,
		InfoHash:    h,
		PieceLength: pieceLength,
		Pieces:      pieces,
		Files:       files,
		Name:        name,
		TotalLength: totalLength,
	}, nil
}

// extractRawInfoDict extracts the raw bencoded bytes of the info dict from the torrent file.
// This is critical for computing info_hash correctly.
func extractRawInfoDict(data []byte, torrentDict map[string]interface{}, infoDict map[string]interface{}) ([]byte, error) {
	// Strategy: re-encode the info dict to get the exact bencoded bytes.
	// This works because bencode encoding is deterministic (dict keys are sorted).
	// We convert the infoDict back to interface{} and re-encode it.
	encodedInfo, err := bencode.Encode(infoDict)
	if err != nil {
		return nil, fmt.Errorf("failed to re-encode info dict: %w", err)
	}
	return encodedInfo, nil
}

// parsePieces parses the pieces field (concatenated 20-byte SHA1 hashes).
func parsePieces(piecesStr string) ([][20]byte, error) {
	if len(piecesStr)%20 != 0 {
		return nil, fmt.Errorf("pieces length %d is not a multiple of 20", len(piecesStr))
	}

	numPieces := len(piecesStr) / 20
	pieces := make([][20]byte, numPieces)

	for i := 0; i < numPieces; i++ {
		copy(pieces[i][:], piecesStr[i*20:(i+1)*20])
	}

	return pieces, nil
}

// parseFilesList parses the files list for multi-file torrents.
func parseFilesList(filesList []interface{}) ([]FileInfo, int64, error) {
	var files []FileInfo
	var totalLength int64

	for i, item := range filesList {
		fileDict, ok := item.(map[string]interface{})
		if !ok {
			return nil, 0, fmt.Errorf("file %d is not a dict", i)
		}

		length, err := getInt64(fileDict, "length")
		if err != nil {
			return nil, 0, fmt.Errorf("file %d length: %w", i, err)
		}

		pathVal, ok := fileDict["path"]
		if !ok {
			return nil, 0, fmt.Errorf("file %d missing path", i)
		}

		pathList, ok := pathVal.([]interface{})
		if !ok {
			return nil, 0, fmt.Errorf("file %d path is not a list", i)
		}

		// Convert path list to string using "/" separator
		pathParts := make([]string, len(pathList))
		for j, p := range pathList {
			s, ok := p.(string)
			if !ok {
				return nil, 0, fmt.Errorf("file %d path component %d is not a string", i, j)
			}
			pathParts[j] = s
		}

		path := filepath.Join(pathParts...)
		path = strings.ReplaceAll(path, "\\", "/") // Normalize to forward slashes

		files = append(files, FileInfo{Length: length, Path: path})
		totalLength += length
	}

	return files, totalLength, nil
}

// Helper functions
func getString(dict map[string]interface{}, key string) (string, error) {
	val, ok := dict[key]
	if !ok {
		return "", fmt.Errorf("missing key: %s", key)
	}
	s, ok := val.(string)
	if !ok {
		return "", fmt.Errorf("%s is not a string", key)
	}
	return s, nil
}

func getInt64(dict map[string]interface{}, key string) (int64, error) {
	val, ok := dict[key]
	if !ok {
		return 0, fmt.Errorf("missing key: %s", key)
	}
	i, ok := val.(int64)
	if !ok {
		return 0, fmt.Errorf("%s is not an int64", key)
	}
	return i, nil
}
