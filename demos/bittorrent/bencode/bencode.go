package bencode

import (
	"bytes"
	"fmt"
	"sort"
	"strconv"
)

// Decode decodes bencoded data into an interface.
// Returns the decoded value and an error if decoding fails.
func Decode(data []byte) (interface{}, error) {
	d := &decoder{data: data, pos: 0}
	return d.decode()
}

// Encode encodes a value into bencoded bytes.
// Returns the bencoded bytes and an error if encoding fails.
func Encode(v interface{}) ([]byte, error) {
	var buf bytes.Buffer
	err := encodeValue(&buf, v)
	return buf.Bytes(), err
}

type decoder struct {
	data []byte
	pos  int
}

func (d *decoder) decode() (interface{}, error) {
	if d.pos >= len(d.data) {
		return nil, fmt.Errorf("unexpected end of data")
	}

	switch d.data[d.pos] {
	case 'i':
		return d.decodeInt()
	case 'l':
		return d.decodeList()
	case 'd':
		return d.decodeDict()
	case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
		return d.decodeString()
	default:
		return nil, fmt.Errorf("invalid bencode character: %c", d.data[d.pos])
	}
}

func (d *decoder) decodeInt() (interface{}, error) {
	if d.data[d.pos] != 'i' {
		return nil, fmt.Errorf("expected 'i'")
	}
	d.pos++

	start := d.pos
	for d.pos < len(d.data) && d.data[d.pos] != 'e' {
		d.pos++
	}

	if d.pos >= len(d.data) {
		return nil, fmt.Errorf("unterminated integer")
	}

	numStr := string(d.data[start:d.pos])
	d.pos++ // skip 'e'

	val, err := strconv.ParseInt(numStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid integer: %v", err)
	}

	return val, nil
}

func (d *decoder) decodeString() (interface{}, error) {
	start := d.pos
	for d.pos < len(d.data) && d.data[d.pos] != ':' {
		d.pos++
	}

	if d.pos >= len(d.data) {
		return nil, fmt.Errorf("unterminated string length")
	}

	lenStr := string(d.data[start:d.pos])
	d.pos++ // skip ':'

	length, err := strconv.ParseInt(lenStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid string length: %v", err)
	}

	end := d.pos + int(length)
	if end > len(d.data) {
		return nil, fmt.Errorf("string extends beyond data")
	}

	result := string(d.data[d.pos:end])
	d.pos = end

	return result, nil
}

func (d *decoder) decodeList() (interface{}, error) {
	if d.data[d.pos] != 'l' {
		return nil, fmt.Errorf("expected 'l'")
	}
	d.pos++

	var list []interface{}
	for d.pos < len(d.data) && d.data[d.pos] != 'e' {
		val, err := d.decode()
		if err != nil {
			return nil, err
		}
		list = append(list, val)
	}

	if d.pos >= len(d.data) {
		return nil, fmt.Errorf("unterminated list")
	}

	d.pos++ // skip 'e'
	return list, nil
}

func (d *decoder) decodeDict() (interface{}, error) {
	if d.data[d.pos] != 'd' {
		return nil, fmt.Errorf("expected 'd'")
	}
	d.pos++

	dict := make(map[string]interface{})
	for d.pos < len(d.data) && d.data[d.pos] != 'e' {
		// Keys must be strings
		keyVal, err := d.decode()
		if err != nil {
			return nil, err
		}

		key, ok := keyVal.(string)
		if !ok {
			return nil, fmt.Errorf("dictionary key must be a string")
		}

		val, err := d.decode()
		if err != nil {
			return nil, err
		}

		dict[key] = val
	}

	if d.pos >= len(d.data) {
		return nil, fmt.Errorf("unterminated dictionary")
	}

	d.pos++ // skip 'e'
	return dict, nil
}

func encodeValue(buf *bytes.Buffer, v interface{}) error {
	switch val := v.(type) {
	case int:
		fmt.Fprintf(buf, "i%de", val)
	case int64:
		fmt.Fprintf(buf, "i%de", val)
	case string:
		fmt.Fprintf(buf, "%d:%s", len(val), val)
	case []interface{}:
		buf.WriteByte('l')
		for _, item := range val {
			if err := encodeValue(buf, item); err != nil {
				return err
			}
		}
		buf.WriteByte('e')
	case map[string]interface{}:
		buf.WriteByte('d')
		// Sort keys for consistent encoding
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(buf, "%d:%s", len(k), k)
			if err := encodeValue(buf, val[k]); err != nil {
				return err
			}
		}
		buf.WriteByte('e')
	default:
		return fmt.Errorf("unsupported type for bencode: %T", v)
	}
	return nil
}
