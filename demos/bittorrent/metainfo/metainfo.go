package metainfo

import (
	"crypto/sha1"
	"fmt"
	"io"
	"os"

	"github.com/mccartykim/wong-bittorrent/bencode"
)

// Torrent represents a parsed .torrent file per BEP 3
type Torrent struct {
	Announce     string     // tracker URL
	AnnounceList [][]string // BEP 12 - list of tiers of tracker URLs
	InfoHash     [20]byte   // SHA1 of bencoded info dict
	PieceLength  int        // number of bytes in each piece
	Pieces       [][20]byte // SHA1 hashes for each piece
	Name         string     // name of the torrent (file or directory)
	Length       int64      // total length in single-file mode
	Files        []File     // files in multi-file mode
}

// File represents a file in a multi-file torrent
type File struct {
	Length int64    // size of the file in bytes
	Path   []string // path components
}

// ParseFromFile parses a .torrent file from disk
func ParseFromFile(path string) (*Torrent, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open torrent file: %w", err)
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("failed to read torrent file: %w", err)
	}

	return ParseFromBytes(data)
}

// ParseFromBytes parses a .torrent file from bencoded bytes
func ParseFromBytes(data []byte) (*Torrent, error) {
	// Decode the top-level dictionary
	decoded, err := bencode.Decode(data)
	if err != nil {
		return nil, fmt.Errorf("failed to decode torrent data: %w", err)
	}

	// The top-level must be a map
	torrentMap, ok := decoded.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("top-level torrent data must be a dictionary")
	}

	t := &Torrent{
		Length: -1, // indicate no single-file length yet
	}

	// Parse announce (required)
	if announce, ok := torrentMap["announce"].(string); ok {
		t.Announce = announce
	}

	// Parse announce-list (optional, BEP 12)
	if announceList, ok := torrentMap["announce-list"].([]interface{}); ok {
		t.AnnounceList = parseAnnounceTiers(announceList)
	}

	// Parse info dictionary (required)
	infoRaw, ok := torrentMap["info"]
	if !ok {
		return nil, fmt.Errorf("missing required 'info' field")
	}

	infoMap, ok := infoRaw.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("'info' field must be a dictionary")
	}

	// Compute InfoHash - SHA1 of the bencoded info dict
	// We need to re-encode the info dict to compute the hash
	infoBencoded, err := bencode.Encode(infoMap)
	if err != nil {
		return nil, fmt.Errorf("failed to re-encode info dict for hash: %w", err)
	}
	hash := sha1.Sum(infoBencoded)
	t.InfoHash = hash

	// Parse piece length (required)
	if pieceLen, ok := infoMap["piece length"].(int); ok {
		t.PieceLength = pieceLen
	} else if piececLen, ok := infoMap["piece length"].(int64); ok {
		t.PieceLength = int(piececLen)
	} else {
		return nil, fmt.Errorf("missing or invalid 'piece length' in info dict")
	}

	// Parse pieces - concatenated 20-byte SHA1 hashes (required)
	if piecesStr, ok := infoMap["pieces"].(string); ok {
		t.Pieces = parsePieces(piecesStr)
	} else {
		return nil, fmt.Errorf("missing or invalid 'pieces' field in info dict")
	}

	// Parse name (required)
	if name, ok := infoMap["name"].(string); ok {
		t.Name = name
	} else {
		return nil, fmt.Errorf("missing or invalid 'name' field in info dict")
	}

	// Parse either length (single-file) or files (multi-file)
	if length, ok := infoMap["length"].(int64); ok {
		t.Length = length
	} else if length, ok := infoMap["length"].(int); ok {
		t.Length = int64(length)
	} else if filesRaw, ok := infoMap["files"].([]interface{}); ok {
		// Multi-file mode
		t.Files, err = parseFiles(filesRaw)
		if err != nil {
			return nil, err
		}
	} else {
		return nil, fmt.Errorf("missing both 'length' and 'files' fields in info dict")
	}

	return t, nil
}

// TotalLength returns the total size of the torrent in bytes
func (t *Torrent) TotalLength() int64 {
	if t.IsMultiFile() {
		total := int64(0)
		for _, file := range t.Files {
			total += file.Length
		}
		return total
	}
	return t.Length
}

// IsMultiFile returns true if the torrent is a multi-file torrent
func (t *Torrent) IsMultiFile() bool {
	return len(t.Files) > 0
}

// parseAnnounceTiers parses announce-list tiers
func parseAnnounceTiers(raw []interface{}) [][]string {
	var tiers [][]string
	for _, tier := range raw {
		if tierList, ok := tier.([]interface{}); ok {
			var tierURLs []string
			for _, url := range tierList {
				if urlStr, ok := url.(string); ok {
					tierURLs = append(tierURLs, urlStr)
				}
			}
			if len(tierURLs) > 0 {
				tiers = append(tiers, tierURLs)
			}
		}
	}
	return tiers
}

// parsePieces extracts individual 20-byte piece hashes from concatenated string
func parsePieces(piecesStr string) [][20]byte {
	var pieces [][20]byte
	for i := 0; i+20 <= len(piecesStr); i += 20 {
		var hash [20]byte
		copy(hash[:], piecesStr[i:i+20])
		pieces = append(pieces, hash)
	}
	return pieces
}

// parseFiles parses the files list from multi-file torrent
func parseFiles(filesRaw []interface{}) ([]File, error) {
	var files []File
	for _, fileRaw := range filesRaw {
		fileMap, ok := fileRaw.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("file entry must be a dictionary")
		}

		var length int64
		if len, ok := fileMap["length"].(int64); ok {
			length = len
		} else if len, ok := fileMap["length"].(int); ok {
			length = int64(len)
		} else {
			return nil, fmt.Errorf("missing or invalid 'length' in file entry")
		}

		pathList, ok := fileMap["path"].([]interface{})
		if !ok {
			return nil, fmt.Errorf("missing or invalid 'path' in file entry")
		}

		var pathComponents []string
		for _, component := range pathList {
			if str, ok := component.(string); ok {
				pathComponents = append(pathComponents, str)
			} else {
				return nil, fmt.Errorf("path component must be a string")
			}
		}

		files = append(files, File{
			Length: length,
			Path:   pathComponents,
		})
	}
	return files, nil
}
