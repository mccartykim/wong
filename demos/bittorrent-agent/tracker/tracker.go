package tracker

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/example/bittorrent/bencode"
)

// Announce represents the announce request to a tracker.
type Announce struct {
	InfoHash [20]byte
	PeerID   [20]byte
	Port     uint16
	Uploaded int64
	Downloaded int64
	Left     int64
	Event    string // "started", "completed", or "" (regular)
}

// PeerInfo represents a peer address and port.
type PeerInfo struct {
	IP   net.IP
	Port uint16
}

// AnnounceResponse contains the response from a tracker announce.
type AnnounceResponse struct {
	Interval int64
	Peers    []PeerInfo
}

// Client is a tracker client for HTTP trackers.
type Client struct {
	url    string
	client *http.Client
}

// NewClient creates a new tracker client.
func NewClient(announceURL string) *Client {
	return &Client{
		url: announceURL,
		client: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// Announce sends an announce request to the tracker.
func (c *Client) Announce(req *Announce) (*AnnounceResponse, error) {
	// Build query string manually to avoid double-encoding binary fields.
	// url.Values.Encode() would re-encode the percent-encoded info_hash/peer_id.
	params := []string{
		"info_hash=" + urlEncode(req.InfoHash[:]),
		"peer_id=" + urlEncode(req.PeerID[:]),
		"port=" + strconv.FormatUint(uint64(req.Port), 10),
		"uploaded=" + strconv.FormatInt(req.Uploaded, 10),
		"downloaded=" + strconv.FormatInt(req.Downloaded, 10),
		"left=" + strconv.FormatInt(req.Left, 10),
		"compact=1",
	}
	if req.Event != "" {
		params = append(params, "event="+url.QueryEscape(req.Event))
	}

	announceURL := c.url + "?" + strings.Join(params, "&")

	resp, err := c.client.Get(announceURL)
	if err != nil {
		return nil, fmt.Errorf("tracker request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read tracker response: %w", err)
	}

	decoded, err := bencode.Decode(body)
	if err != nil {
		return nil, fmt.Errorf("failed to decode tracker response: %w", err)
	}

	respDict, ok := decoded.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("tracker response is not a dict")
	}

	// Check for failure reason
	if failureReason, ok := respDict["failure reason"].(string); ok {
		return nil, fmt.Errorf("tracker failure: %s", failureReason)
	}

	// Get interval
	interval, ok := respDict["interval"].(int64)
	if !ok {
		return nil, fmt.Errorf("missing or invalid interval in tracker response")
	}

	// Parse peers (compact format)
	peersData, ok := respDict["peers"].(string)
	if !ok {
		// Try non-compact format (list of dicts)
		if peersList, ok := respDict["peers"].([]interface{}); ok {
			return parsePeersList(interval, peersList)
		}
		return nil, fmt.Errorf("invalid peers format in tracker response")
	}

	peers, err := parseCompactPeers(peersData)
	if err != nil {
		return nil, fmt.Errorf("failed to parse peers: %w", err)
	}

	return &AnnounceResponse{
		Interval: interval,
		Peers:    peers,
	}, nil
}

// parseCompactPeers parses compact peer format (6 bytes per peer: 4 IP + 2 port).
func parseCompactPeers(data string) ([]PeerInfo, error) {
	if len(data)%6 != 0 {
		return nil, fmt.Errorf("invalid compact peers length: %d", len(data))
	}

	var peers []PeerInfo
	for i := 0; i < len(data); i += 6 {
		ip := net.IPv4(data[i], data[i+1], data[i+2], data[i+3])
		port := binary.BigEndian.Uint16([]byte(data[i+4 : i+6]))
		peers = append(peers, PeerInfo{IP: ip, Port: port})
	}
	return peers, nil
}

// parsePeersList parses non-compact peer format.
func parsePeersList(interval int64, peersList []interface{}) (*AnnounceResponse, error) {
	var peers []PeerInfo
	for _, item := range peersList {
		peerDict, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		ipStr, ok := peerDict["ip"].(string)
		if !ok {
			continue
		}

		port, ok := peerDict["port"].(int64)
		if !ok {
			continue
		}

		ip := net.ParseIP(ipStr)
		if ip == nil {
			continue
		}

		peers = append(peers, PeerInfo{IP: ip, Port: uint16(port)})
	}

	return &AnnounceResponse{
		Interval: interval,
		Peers:    peers,
	}, nil
}

// urlEncode performs URL encoding for binary data (byte-by-byte).
func urlEncode(data []byte) string {
	result := ""
	for _, b := range data {
		result += fmt.Sprintf("%%%02x", b)
	}
	return result
}

// GeneratePeerID generates a unique 20-byte peer ID.
func GeneratePeerID() [20]byte {
	var id [20]byte
	copy(id[:], "-GO0001-")
	// Fill the rest with random bytes
	for i := 8; i < 20; i++ {
		id[i] = byte(time.Now().UnixNano()%256) + byte(i)
	}
	return id
}
