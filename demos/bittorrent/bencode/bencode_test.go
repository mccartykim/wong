package bencode

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

// Test encoding and decoding integers
func TestIntegerEncode(t *testing.T) {
	tests := []struct {
		name     string
		value    interface{}
		expected string
	}{
		{"positive int", 42, "i42e"},
		{"positive int64", int64(42), "i42e"},
		{"negative int", -3, "i-3e"},
		{"zero int", 0, "i0e"},
		{"large positive", int64(9223372036854775807), "i9223372036854775807e"},
		{"large negative", int64(-9223372036854775808), "i-9223372036854775808e"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := Encode(tc.value)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if string(result) != tc.expected {
				t.Fatalf("expected %q, got %q", tc.expected, string(result))
			}
		})
	}
}

func TestIntegerDecode(t *testing.T) {
	tests := []struct {
		name     string
		data     string
		expected int64
	}{
		{"positive int", "i42e", 42},
		{"negative int", "i-3e", -3},
		{"zero", "i0e", 0},
		{"large positive", "i9223372036854775807e", 9223372036854775807},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := Decode([]byte(tc.data))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			val, ok := result.(int64)
			if !ok {
				t.Fatalf("expected int64, got %T", result)
			}
			if val != tc.expected {
				t.Fatalf("expected %d, got %d", tc.expected, val)
			}
		})
	}
}

// Test encoding and decoding strings
func TestStringEncode(t *testing.T) {
	tests := []struct {
		name     string
		value    interface{}
		expected string
	}{
		{"simple string", "spam", "4:spam"},
		{"empty string", "", "0:"},
		{"string with digits", "hello123", "8:hello123"},
		{"string with spaces", "hello world", "11:hello world"},
		{"bytes", []byte("hello"), "5:hello"},
		{"empty bytes", []byte{}, "0:"},
		{"binary bytes", []byte{0, 1, 2, 3}, "4:\x00\x01\x02\x03"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := Encode(tc.value)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if string(result) != tc.expected {
				t.Fatalf("expected %q, got %q", tc.expected, string(result))
			}
		})
	}
}

func TestStringDecode(t *testing.T) {
	tests := []struct {
		name     string
		data     string
		expected string
	}{
		{"simple string", "4:spam", "spam"},
		{"empty string", "0:", ""},
		{"string with spaces", "11:hello world", "hello world"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := Decode([]byte(tc.data))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			val, ok := result.(string)
			if !ok {
				t.Fatalf("expected string, got %T", result)
			}
			if val != tc.expected {
				t.Fatalf("expected %q, got %q", tc.expected, val)
			}
		})
	}
}

// Test encoding and decoding lists
func TestListEncode(t *testing.T) {
	tests := []struct {
		name     string
		value    []interface{}
		expected string
	}{
		{"empty list", []interface{}{}, "le"},
		{"list of ints", []interface{}{1, 2, 3}, "li1ei2ei3ee"},
		{"list of strings", []interface{}{"a", "b"}, "l1:a1:be"},
		{"mixed list", []interface{}{1, "two", 3}, "li1e3:twoi3ee"},
		{"nested list", []interface{}{[]interface{}{1, 2}, 3}, "lli1ei2eei3ee"},
		{"list with empty list", []interface{}{[]interface{}{}, 42}, "llegi42ee"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := Encode(tc.value)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if string(result) != tc.expected {
				t.Fatalf("expected %q, got %q", tc.expected, string(result))
			}
		})
	}
}

func TestListDecode(t *testing.T) {
	tests := []struct {
		name     string
		data     string
		check    func(interface{}) bool
	}{
		{"empty list", "le", func(v interface{}) bool {
			list, ok := v.([]interface{})
			return ok && len(list) == 0
		}},
		{"list of ints", "li1ei2ei3ee", func(v interface{}) bool {
			list, ok := v.([]interface{})
			if !ok || len(list) != 3 {
				return false
			}
			return list[0].(int64) == 1 && list[1].(int64) == 2 && list[2].(int64) == 3
		}},
		{"mixed list", "li1e3:twoi3ee", func(v interface{}) bool {
			list, ok := v.([]interface{})
			if !ok || len(list) != 3 {
				return false
			}
			return list[0].(int64) == 1 && list[1].(string) == "two" && list[2].(int64) == 3
		}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := Decode([]byte(tc.data))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !tc.check(result) {
				t.Fatalf("check failed for %v", result)
			}
		})
	}
}

// Test encoding and decoding dictionaries
func TestDictEncode(t *testing.T) {
	tests := []struct {
		name     string
		value    map[string]interface{}
		expected string
	}{
		{"empty dict", map[string]interface{}{}, "de"},
		{"simple dict", map[string]interface{}{"key": "value"}, "d3:key5:valuee"},
		{"dict with int", map[string]interface{}{"num": int64(42)}, "d3:numi42ee"},
		{"dict multiple keys", map[string]interface{}{"a": 1, "b": 2}, "d1:ai1e1:bi2ee"},
		{"nested dict", map[string]interface{}{"nested": map[string]interface{}{"inner": 1}}, "d6:nestedd5:inneri1eeee"},
		{"dict with list", map[string]interface{}{"list": []interface{}{1, 2}}, "d4:listli1ei2eee"},
		{"complex dict", map[string]interface{}{
			"spam": "eggs",
			"cow":  "moo",
			"num":  42,
		}, "d3:cow3:moo3:num i42e4:spam4:eggse"}, // Note: space in "i42e" - keys alphabetically sorted: cow, num, spam
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := Encode(tc.value)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if string(result) != tc.expected {
				t.Fatalf("expected %q, got %q", tc.expected, string(result))
			}
		})
	}
}

func TestDictDecode(t *testing.T) {
	tests := []struct {
		name     string
		data     string
		checkKey string
		expected interface{}
	}{
		{"simple dict", "d3:key5:valuee", "key", "value"},
		{"dict with int", "d3:numi42ee", "num", int64(42)},
		{"empty dict", "de", "", nil},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := Decode([]byte(tc.data))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			dict, ok := result.(map[string]interface{})
			if !ok {
				t.Fatalf("expected map, got %T", result)
			}
			if tc.checkKey != "" {
				val, exists := dict[tc.checkKey]
				if !exists {
					t.Fatalf("key %q not found in dict", tc.checkKey)
				}
				if val != tc.expected {
					t.Fatalf("expected %v, got %v", tc.expected, val)
				}
			}
		})
	}
}

// Test key sorting in dicts
func TestDictKeySorting(t *testing.T) {
	// Encode a dict with keys that need to be sorted
	input := map[string]interface{}{
		"zebra":  1,
		"apple":  2,
		"middle": 3,
	}

	result, err := Encode(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	encoded := string(result)
	// Keys should be sorted: apple, middle, zebra
	// Expected: d5:applei2e6:middlei3e5:zebrai1ee
	expected := "d5:applei2e6:middlei3e5:zebrai1ee"
	if encoded != expected {
		t.Fatalf("expected %q, got %q", expected, encoded)
	}

	// Decode should verify key sorting
	decoded, err := Decode(result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	dict, ok := decoded.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map, got %T", decoded)
	}
	if len(dict) != 3 {
		t.Fatalf("expected 3 keys, got %d", len(dict))
	}
}

// Test key sorting validation during decode
func TestDictKeySortingValidation(t *testing.T) {
	// Try to decode a dict with unsorted keys (should fail)
	unsortedData := "d6:secondi2e5:firsti1ee" // 'second' comes before 'first' - wrong order

	_, err := Decode([]byte(unsortedData))
	if err == nil {
		t.Fatalf("expected error for unsorted keys")
	}
}

// Test round-trip encoding and decoding
func TestRoundTrip(t *testing.T) {
	tests := []struct {
		name  string
		value interface{}
	}{
		{"int", int64(42)},
		{"string", "hello"},
		{"empty list", []interface{}{}},
		{"list", []interface{}{1, "two", 3}},
		{"empty dict", map[string]interface{}{}},
		{"dict", map[string]interface{}{"a": 1, "b": "two"}},
		{"nested structure", map[string]interface{}{
			"list": []interface{}{1, 2, 3},
			"dict": map[string]interface{}{"x": "y"},
			"name": "test",
		}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Encode
			encoded, err := Encode(tc.value)
			if err != nil {
				t.Fatalf("encode failed: %v", err)
			}

			// Decode
			decoded, err := Decode(encoded)
			if err != nil {
				t.Fatalf("decode failed: %v", err)
			}

			// Verify by re-encoding
			reencoded, err := Encode(decoded)
			if err != nil {
				t.Fatalf("re-encode failed: %v", err)
			}

			if string(encoded) != string(reencoded) {
				t.Fatalf("round-trip mismatch:\noriginal:  %q\nre-encoded: %q", string(encoded), string(reencoded))
			}
		})
	}
}

// Test error cases
func TestDecodeErrors(t *testing.T) {
	tests := []struct {
		name     string
		data     string
		shouldErr bool
	}{
		{"invalid start", "x", true},
		{"unclosed integer", "i42", true},
		{"empty integer", "ie", true},
		{"invalid integer", "i-e", true},
		{"integer with leading zeros", "i042e", true},
		{"negative with leading zeros", "i-042e", true},
		{"unclosed string", "5:spam", true},
		{"string too short", "10:hello", true},
		{"unclosed list", "li1e", true},
		{"unclosed dict", "d1:ae", true},
		{"dict with non-string key", "di42e1:ve", true},
		{"unexpected EOF", "i42", true},
		{"valid integer", "i42e", false},
		{"valid string", "4:spam", false},
		{"valid list", "le", false},
		{"valid dict", "de", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Decode([]byte(tc.data))
			if tc.shouldErr && err == nil {
				t.Fatalf("expected error but got nil")
			}
			if !tc.shouldErr && err != nil {
				t.Fatalf("expected no error but got: %v", err)
			}
		})
	}
}

// Test DecodeReader with io.Reader
func TestDecodeReader(t *testing.T) {
	tests := []struct {
		name     string
		data     string
		expected interface{}
		checkFn  func(interface{}) bool
	}{
		{"int from reader", "i42e", int64(42), func(v interface{}) bool {
			return v.(int64) == 42
		}},
		{"string from reader", "5:hello", "hello", func(v interface{}) bool {
			return v.(string) == "hello"
		}},
		{"list from reader", "li1ei2ee", nil, func(v interface{}) bool {
			list, ok := v.([]interface{})
			return ok && len(list) == 2 && list[0].(int64) == 1 && list[1].(int64) == 2
		}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			reader := strings.NewReader(tc.data)
			result, err := DecodeReader(reader)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !tc.checkFn(result) {
				t.Fatalf("check failed for %v", result)
			}
		})
	}
}

// Test binary data in strings
func TestBinaryData(t *testing.T) {
	// Test encoding binary data
	binaryData := []byte{0x00, 0x01, 0x02, 0xFF, 0xFE, 0xFD}
	encoded, err := Encode(binaryData)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Decode and verify
	decoded, err := Decode(encoded)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The decoded value is a string (but contains binary data)
	decodedStr, ok := decoded.(string)
	if !ok {
		t.Fatalf("expected string, got %T", decoded)
	}

	if decodedStr != string(binaryData) {
		t.Fatalf("binary data mismatch: expected %v, got %v", binaryData, []byte(decodedStr))
	}
}

// Test struct encoding with bencode tags
func TestStructEncode(t *testing.T) {
	type TestStruct struct {
		Name  string `bencode:"name"`
		Value int64  `bencode:"value"`
		Skip  string `bencode:"-"`
	}

	s := TestStruct{
		Name:  "test",
		Value: 42,
		Skip:  "ignored",
	}

	encoded, err := Encode(s)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	decoded, err := Decode(encoded)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	dict, ok := decoded.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map, got %T", decoded)
	}

	if dict["name"] != "test" {
		t.Fatalf("expected name=test, got %v", dict["name"])
	}
	if dict["value"] != int64(42) {
		t.Fatalf("expected value=42, got %v", dict["value"])
	}
	if _, exists := dict["Skip"]; exists {
		t.Fatalf("Skip field should not be in dict")
	}
}

// Test with real torrent-like data
func TestTorrentLikeData(t *testing.T) {
	// Simulate a simple torrent info dict
	torrentData := map[string]interface{}{
		"announce": "http://tracker.example.com:6969/announce",
		"info": map[string]interface{}{
			"name":         "test.txt",
			"length":       int64(1024),
			"piece length": int64(16384),
			"pieces":       "\x00\x01\x02\x03\x04\x05\x06\x07\x08\x09\x0a\x0b\x0c\x0d\x0e\x0f\x10\x11\x12\x13\x14\x15\x16\x17\x18\x19\x1a\x1b\x1c\x1d\x1e\x1f",
		},
	}

	encoded, err := Encode(torrentData)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	decoded, err := Decode(encoded)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	dict, ok := decoded.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map, got %T", decoded)
	}

	if dict["announce"] != "http://tracker.example.com:6969/announce" {
		t.Fatalf("announce mismatch")
	}

	info, ok := dict["info"].(map[string]interface{})
	if !ok {
		t.Fatalf("info should be a dict")
	}

	if info["name"] != "test.txt" {
		t.Fatalf("name mismatch")
	}
	if info["length"] != int64(1024) {
		t.Fatalf("length mismatch")
	}
}

// Benchmark encoding
func BenchmarkEncode(b *testing.B) {
	data := map[string]interface{}{
		"announce": "http://tracker.example.com:6969/announce",
		"info": map[string]interface{}{
			"name":         "test.txt",
			"length":       int64(1024),
			"piece length": int64(16384),
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = Encode(data)
	}
}

// Benchmark decoding
func BenchmarkDecode(b *testing.B) {
	data := []byte("d8:announce26:http://tracker.example.com:6969/announce4:infod6:lengthi1024e11:piece lengthi16384e4:name8:test.txtee")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = Decode(data)
	}
}

// Test special characters in strings
func TestSpecialCharacters(t *testing.T) {
	tests := []string{
		"hello\nworld",
		"tab\there",
		"quote\"here",
		"backslash\\here",
		"null\x00byte",
	}

	for _, test := range tests {
		encoded, err := Encode(test)
		if err != nil {
			t.Fatalf("encode failed for %q: %v", test, err)
		}

		decoded, err := Decode(encoded)
		if err != nil {
			t.Fatalf("decode failed for %q: %v", test, err)
		}

		decodedStr := decoded.(string)
		if decodedStr != test {
			t.Fatalf("mismatch for %q: got %q", test, decodedStr)
		}
	}
}

// Test nil handling
func TestNilHandling(t *testing.T) {
	_, err := Encode(nil)
	if err == nil {
		t.Fatalf("expected error for nil")
	}
}

// Test using Decode with trailing data
func TestDecodeWithTrailingData(t *testing.T) {
	data := []byte("i42eextra")
	result, err := Decode(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	val, ok := result.(int64)
	if !ok || val != 42 {
		t.Fatalf("expected int64(42), got %v", result)
	}
	// The trailing "extra" is not processed by Decode
	// This is expected behavior when using bytes.Reader
}

// Test DecodeReader stops at first complete value
func TestDecodeReaderStopsAtFirstValue(t *testing.T) {
	// Create a reader with multiple values
	data := "i42ei99e"
	reader := strings.NewReader(data)

	result, err := DecodeReader(reader)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	val, ok := result.(int64)
	if !ok || val != 42 {
		t.Fatalf("expected int64(42), got %v", result)
	}

	// Try to read the next value
	nextResult, err := DecodeReader(reader)
	if err != nil {
		t.Fatalf("unexpected error reading next: %v", err)
	}

	nextVal, ok := nextResult.(int64)
	if !ok || nextVal != 99 {
		t.Fatalf("expected int64(99), got %v", nextResult)
	}
}
