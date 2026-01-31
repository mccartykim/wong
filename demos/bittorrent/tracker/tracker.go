package tracker

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/mccartykim/wong-bittorrent/bencode"
)

// Peer represents a peer in the swarm
type Peer struct {
	IP   net.IP
	Port uint16
}

// AnnounceRequest represents a tracker announce request
type AnnounceRequest struct {
	AnnounceURL string
	InfoHash    [20]byte
	PeerID      [20]byte
	Port        uint16
	Uploaded    int64
	Downloaded  int64
	Left        int64
	Event       string // "started", "completed", "stopped", or ""
}

// AnnounceResponse represents a tracker announce response
type AnnounceResponse struct {
	Interval int
	Peers    []Peer
}

// Announce sends an announce request to the tracker
func Announce(ctx context.Context, req *AnnounceRequest) (*AnnounceResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("announce request cannot be nil")
	}

	// Build the announce URL with query parameters
	u, err := url.Parse(req.AnnounceURL)
	if err != nil {
		return nil, fmt.Errorf("invalid announce URL: %w", err)
	}

	q := u.Query()
	// URL-encode the info_hash as raw bytes
	q.Set("info_hash", string(req.InfoHash[:]))
	q.Set("peer_id", string(req.PeerID[:]))
	q.Set("port", strconv.FormatUint(uint64(req.Port), 10))
	q.Set("uploaded", strconv.FormatInt(req.Uploaded, 10))
	q.Set("downloaded", strconv.FormatInt(req.Downloaded, 10))
	q.Set("left", strconv.FormatInt(req.Left, 10))
	q.Set("compact", "1")
	if req.Event != "" {
		q.Set("event", req.Event)
	}
	u.RawQuery = q.Encode()

	// Create HTTP request with context
	httpReq, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Send the request
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("tracker request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("tracker returned status %d: %s", resp.StatusCode, string(body))
	}

	// Read and decode the response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Decode bencode response
	decoded, err := bencode.Decode(body)
	if err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Parse the bencode response
	respDict, ok := decoded.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("tracker response is not a dict")
	}

	// Check for failure reason
	if failureReason, ok := respDict["failure reason"]; ok {
		return nil, fmt.Errorf("tracker failure: %v", failureReason)
	}

	// Extract interval
	var interval int
	if iv, ok := respDict["interval"]; ok {
		switch v := iv.(type) {
		case int:
			interval = v
		case int64:
			interval = int(v)
		default:
			return nil, fmt.Errorf("invalid interval type: %T", v)
		}
	}

	// Extract and parse peers
	var peers []Peer

	// Try compact format first
	if peersData, ok := respDict["peers"]; ok {
		switch v := peersData.(type) {
		case []byte:
			// Compact format: 6 bytes per peer (4 bytes IP + 2 bytes port)
			if len(v)%6 != 0 {
				return nil, fmt.Errorf("invalid compact peers format: length not multiple of 6")
			}
			for i := 0; i < len(v); i += 6 {
				ip := net.IPv4(v[i], v[i+1], v[i+2], v[i+3])
				port := binary.BigEndian.Uint16(v[i+4 : i+6])
				peers = append(peers, Peer{IP: ip, Port: port})
			}
		case []interface{}:
			// Non-compact format: list of dicts with "ip" and "port"
			for _, peerIface := range v {
				peerDict, ok := peerIface.(map[string]interface{})
				if !ok {
					return nil, fmt.Errorf("peer is not a dict")
				}

				// Extract IP
				ipStr, ok := peerDict["ip"].(string)
				if !ok {
					return nil, fmt.Errorf("peer ip is not a string")
				}
				ip := net.ParseIP(ipStr)
				if ip == nil {
					return nil, fmt.Errorf("invalid peer IP: %s", ipStr)
				}

				// Extract port
				var port uint16
				switch portVal := peerDict["port"].(type) {
				case int:
					port = uint16(portVal)
				case int64:
					port = uint16(portVal)
				default:
					return nil, fmt.Errorf("invalid port type: %T", portVal)
				}

				peers = append(peers, Peer{IP: ip, Port: port})
			}
		default:
			return nil, fmt.Errorf("invalid peers type: %T", v)
		}
	}

	return &AnnounceResponse{
		Interval: interval,
		Peers:    peers,
	}, nil
}

// GeneratePeerID generates a random peer ID with format "-WG0001-" + 12 random bytes
func GeneratePeerID() [20]byte {
	var peerID [20]byte
	// Set the prefix "-WG0001-"
	prefix := []byte("-WG0001-")
	copy(peerID[:], prefix)

	// Fill the remaining 12 bytes with random data
	randomBytes := make([]byte, 12)
	_, err := rand.Read(randomBytes)
	if err != nil {
		// Fallback: use time-based random seed
		rng := rand.New(rand.NewSource(time.Now().UnixNano()))
		for i := 0; i < 12; i++ {
			randomBytes[i] = byte(rng.Intn(256))
		}
	}
	copy(peerID[8:], randomBytes)

	return peerID
}
