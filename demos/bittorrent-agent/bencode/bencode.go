package bencode

import (
	"bytes"
	"fmt"
	"io"
	"sort"
	"strconv"
)

// Encode converts a Go value to bencode format.
// Supports: string, int64, []interface{}, map[string]interface{}
func Encode(v interface{}) ([]byte, error) {
	var buf bytes.Buffer
	err := encode(&buf, v)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func encode(w io.Writer, v interface{}) error {
	switch val := v.(type) {
	case string:
		return encodeString(w, val)
	case int, int8, int16, int32, int64:
		var i int64
		switch vt := v.(type) {
		case int:
			i = int64(vt)
		case int8:
			i = int64(vt)
		case int16:
			i = int64(vt)
		case int32:
			i = int64(vt)
		case int64:
			i = vt
		}
		return encodeInt(w, i)
	case []interface{}:
		return encodeList(w, val)
	case map[string]interface{}:
		return encodeDict(w, val)
	default:
		return fmt.Errorf("unsupported type: %T", v)
	}
}

func encodeString(w io.Writer, s string) error {
	// Format: <length>:<data>
	b := []byte(s)
	prefix := strconv.Itoa(len(b)) + ":"
	_, err := io.WriteString(w, prefix)
	if err != nil {
		return err
	}
	_, err = w.Write(b)
	return err
}

func encodeInt(w io.Writer, i int64) error {
	// Format: i<num>e
	s := fmt.Sprintf("i%de", i)
	_, err := io.WriteString(w, s)
	return err
}

func encodeList(w io.Writer, list []interface{}) error {
	// Format: l...e
	if _, err := io.WriteString(w, "l"); err != nil {
		return err
	}
	for _, item := range list {
		if err := encode(w, item); err != nil {
			return err
		}
	}
	_, err := io.WriteString(w, "e")
	return err
}

func encodeDict(w io.Writer, dict map[string]interface{}) error {
	// Format: d...e with sorted keys
	if _, err := io.WriteString(w, "d"); err != nil {
		return err
	}

	// Sort keys lexicographically
	keys := make([]string, 0, len(dict))
	for k := range dict {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Encode each key-value pair in sorted order
	for _, key := range keys {
		if err := encodeString(w, key); err != nil {
			return err
		}
		if err := encode(w, dict[key]); err != nil {
			return err
		}
	}

	_, err := io.WriteString(w, "e")
	return err
}
