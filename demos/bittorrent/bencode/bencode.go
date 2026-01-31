package bencode

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strconv"
)

// Decode decodes bencoded data into an interface.
// Returns: int64 for integers, string for strings, []interface{} for lists, map[string]interface{} for dicts.
func Decode(data []byte) (interface{}, error) {
	r := bytes.NewReader(data)
	return DecodeReader(r)
}

// DecodeReader decodes bencode from an io.Reader.
func DecodeReader(r io.Reader) (interface{}, error) {
	d := &readerDecoder{r: r}
	return d.decode()
}

// Encode encodes a Go value to bencode format.
// Supported types: int, int64, string, []byte, []interface{}, map[string]interface{}, and structs
func Encode(v interface{}) ([]byte, error) {
	var buf bytes.Buffer
	err := encodeValue(&buf, v)
	return buf.Bytes(), err
}

type readerDecoder struct {
	r   io.Reader
	buf []byte
	pos int
}

func (d *readerDecoder) readOne() (byte, error) {
	tmp := make([]byte, 1)
	n, err := d.r.Read(tmp)
	if n == 0 && err == nil {
		err = io.EOF
	}
	if err != nil {
		return 0, err
	}
	d.buf = append(d.buf, tmp[0])
	return tmp[0], nil
}

func (d *readerDecoder) peek() (byte, error) {
	for d.pos >= len(d.buf) {
		_, err := d.readOne()
		if err != nil {
			return 0, err
		}
	}
	return d.buf[d.pos], nil
}

func (d *readerDecoder) next() (byte, error) {
	b, err := d.peek()
	if err != nil {
		return 0, err
	}
	d.pos++
	return b, nil
}

func (d *readerDecoder) readUntil(end byte) (string, error) {
	var result bytes.Buffer
	for {
		b, err := d.next()
		if err != nil {
			return "", err
		}
		if b == end {
			return result.String(), nil
		}
		result.WriteByte(b)
	}
}

func (d *readerDecoder) decode() (interface{}, error) {
	b, err := d.peek()
	if err != nil {
		return nil, err
	}

	switch {
	case b == 'i':
		return d.decodeInt()
	case b >= '0' && b <= '9':
		return d.decodeString()
	case b == 'l':
		return d.decodeList()
	case b == 'd':
		return d.decodeDict()
	default:
		return nil, fmt.Errorf("bencode: unexpected byte: %c", b)
	}
}

func (d *readerDecoder) decodeInt() (int64, error) {
	b, err := d.next()
	if err != nil {
		return 0, err
	}
	if b != 'i' {
		return 0, fmt.Errorf("bencode: expected 'i', got %c", b)
	}

	numStr, err := d.readUntil('e')
	if err != nil {
		return 0, err
	}

	if numStr == "" {
		return 0, errors.New("bencode: empty integer")
	}

	// Check for invalid leading zeros
	if len(numStr) > 1 && numStr[0] == '0' {
		return 0, errors.New("bencode: integer with leading zeros")
	}
	if len(numStr) > 2 && numStr[0] == '-' && numStr[1] == '0' {
		return 0, errors.New("bencode: negative integer with leading zeros")
	}

	i, err := strconv.ParseInt(numStr, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("bencode: invalid integer: %v", err)
	}

	return i, nil
}

func (d *readerDecoder) decodeString() (interface{}, error) {
	lenStr, err := d.readUntil(':')
	if err != nil {
		return nil, err
	}

	if lenStr == "" {
		return nil, errors.New("bencode: empty string length")
	}

	length, err := strconv.Atoi(lenStr)
	if err != nil || length < 0 {
		return nil, fmt.Errorf("bencode: invalid string length: %s", lenStr)
	}

	buf := make([]byte, length)
	n, err := io.ReadFull(d.r, buf)
	if err != nil {
		return nil, fmt.Errorf("bencode: unexpected EOF in string: %v", err)
	}
	if n != length {
		return nil, fmt.Errorf("bencode: short read in string")
	}

	d.buf = append(d.buf, buf...)
	d.pos += length  // Advance position past the bytes we just read

	return string(buf), nil
}

func (d *readerDecoder) decodeList() ([]interface{}, error) {
	b, err := d.next()
	if err != nil {
		return nil, err
	}
	if b != 'l' {
		return nil, fmt.Errorf("bencode: expected 'l', got %c", b)
	}

	var list []interface{}
	for {
		b, err := d.peek()
		if err != nil {
			return nil, err
		}
		if b == 'e' {
			_, _ = d.next() // consume 'e'
			return list, nil
		}

		item, err := d.decode()
		if err != nil {
			return nil, err
		}
		list = append(list, item)
	}
}

func (d *readerDecoder) decodeDict() (map[string]interface{}, error) {
	b, err := d.next()
	if err != nil {
		return nil, err
	}
	if b != 'd' {
		return nil, fmt.Errorf("bencode: expected 'd', got %c", b)
	}

	dict := make(map[string]interface{})
	var lastKey string

	for {
		b, err := d.peek()
		if err != nil {
			return nil, err
		}
		if b == 'e' {
			_, _ = d.next() // consume 'e'
			return dict, nil
		}

		// Keys must be strings
		keyVal, err := d.decodeString()
		if err != nil {
			return nil, err
		}
		keyStr := keyVal.(string)

		// Verify keys are sorted
		if lastKey != "" && keyStr <= lastKey {
			return nil, errors.New("bencode: dictionary keys not sorted")
		}
		lastKey = keyStr

		value, err := d.decode()
		if err != nil {
			return nil, err
		}

		dict[keyStr] = value
	}
}

func encodeValue(buf *bytes.Buffer, v interface{}) error {
	if v == nil {
		return errors.New("bencode: cannot encode nil")
	}

	switch val := v.(type) {
	case int:
		fmt.Fprintf(buf, "i%de", val)
		return nil
	case int64:
		fmt.Fprintf(buf, "i%de", val)
		return nil
	case string:
		fmt.Fprintf(buf, "%d:%s", len(val), val)
		return nil
	case []byte:
		fmt.Fprintf(buf, "%d:", len(val))
		_, err := buf.Write(val)
		return err
	case []interface{}:
		buf.WriteByte('l')
		for _, item := range val {
			if err := encodeValue(buf, item); err != nil {
				return err
			}
		}
		buf.WriteByte('e')
		return nil
	case map[string]interface{}:
		buf.WriteByte('d')
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
		return nil
	default:
		// Try to handle structs with bencode tags
		rv := reflect.ValueOf(v)
		if rv.Kind() == reflect.Struct {
			return encodeStruct(buf, rv)
		}
		return fmt.Errorf("bencode: unsupported type: %T", v)
	}
}

func encodeStruct(buf *bytes.Buffer, rv reflect.Value) error {
	rt := rv.Type()
	m := make(map[string]interface{})

	for i := 0; i < rt.NumField(); i++ {
		field := rt.Field(i)
		tag := field.Tag.Get("bencode")
		if tag == "" || tag == "-" {
			continue
		}
		fieldVal := rv.Field(i)
		if !fieldVal.IsZero() {
			m[tag] = fieldVal.Interface()
		}
	}

	return encodeValue(buf, m)
}
