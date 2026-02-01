package bencode

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Decoder reads and decodes bencode values from an input stream.
type Decoder struct {
	r *bufio.Reader
}

// NewDecoder creates a new Decoder that reads from r.
func NewDecoder(r io.Reader) *Decoder {
	return &Decoder{
		r: bufio.NewReader(r),
	}
}

// Decode reads and decodes a single bencode value.
func (d *Decoder) Decode() (interface{}, error) {
	return d.decode()
}

// Decode is a standalone function that decodes bencode data from a byte slice.
func Decode(data []byte) (interface{}, error) {
	d := NewDecoder(strings.NewReader(string(data)))
	return d.Decode()
}

func (d *Decoder) decode() (interface{}, error) {
	ch, err := d.r.ReadByte()
	if err != nil {
		return nil, fmt.Errorf("unexpected EOF: %w", err)
	}

	switch {
	case ch >= '0' && ch <= '9':
		// String: read length, then data
		d.r.UnreadByte()
		return d.decodeString()
	case ch == 'i':
		// Integer: i<num>e
		return d.decodeInt()
	case ch == 'l':
		// List: l...e
		return d.decodeList()
	case ch == 'd':
		// Dict: d...e
		return d.decodeDict()
	default:
		return nil, fmt.Errorf("unexpected character: %c", ch)
	}
}

func (d *Decoder) decodeString() (string, error) {
	// Read length
	var lenStr strings.Builder
	for {
		ch, err := d.r.ReadByte()
		if err != nil {
			return "", fmt.Errorf("error reading string length: %w", err)
		}
		if ch == ':' {
			break
		}
		if ch < '0' || ch > '9' {
			return "", fmt.Errorf("invalid string length character: %c", ch)
		}
		lenStr.WriteByte(ch)
	}

	length, err := strconv.Atoi(lenStr.String())
	if err != nil {
		return "", fmt.Errorf("invalid string length: %w", err)
	}

	if length < 0 {
		return "", fmt.Errorf("negative string length: %d", length)
	}

	// Read data
	data := make([]byte, length)
	n, err := io.ReadFull(d.r, data)
	if err != nil {
		return "", fmt.Errorf("error reading string data: %w", err)
	}
	if n != length {
		return "", fmt.Errorf("incomplete string data: expected %d, got %d", length, n)
	}

	return string(data), nil
}

func (d *Decoder) decodeInt() (int64, error) {
	// Read integer: i<num>e
	var numStr strings.Builder
	for {
		ch, err := d.r.ReadByte()
		if err != nil {
			return 0, fmt.Errorf("error reading integer: %w", err)
		}
		if ch == 'e' {
			break
		}
		numStr.WriteByte(ch)
	}

	numStrVal := numStr.String()
	if numStrVal == "" {
		return 0, fmt.Errorf("empty integer")
	}

	// Check for leading zero (invalid unless it's just "0")
	if len(numStrVal) > 1 && numStrVal[0] == '0' {
		return 0, fmt.Errorf("leading zero in integer: %s", numStrVal)
	}
	if len(numStrVal) > 2 && numStrVal[0] == '-' && numStrVal[1] == '0' {
		return 0, fmt.Errorf("leading zero in negative integer: %s", numStrVal)
	}

	num, err := strconv.ParseInt(numStrVal, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid integer: %w", err)
	}

	return num, nil
}

func (d *Decoder) decodeList() ([]interface{}, error) {
	// Read list: l...e
	list := make([]interface{}, 0)

	for {
		ch, err := d.r.ReadByte()
		if err != nil {
			return nil, fmt.Errorf("error reading list: %w", err)
		}

		if ch == 'e' {
			break
		}

		// Put the byte back and decode the next element
		d.r.UnreadByte()
		val, err := d.decode()
		if err != nil {
			return nil, fmt.Errorf("error decoding list element: %w", err)
		}
		list = append(list, val)
	}

	return list, nil
}

func (d *Decoder) decodeDict() (map[string]interface{}, error) {
	// Read dict: d...e with string keys
	dict := make(map[string]interface{})

	for {
		ch, err := d.r.ReadByte()
		if err != nil {
			return nil, fmt.Errorf("error reading dict: %w", err)
		}

		if ch == 'e' {
			break
		}

		// Put the byte back and decode the key (must be string)
		d.r.UnreadByte()
		key, err := d.decodeString()
		if err != nil {
			return nil, fmt.Errorf("error decoding dict key: %w", err)
		}

		// Decode the value
		val, err := d.decode()
		if err != nil {
			return nil, fmt.Errorf("error decoding dict value for key %q: %w", key, err)
		}

		dict[key] = val
	}

	return dict, nil
}
