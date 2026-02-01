package bencode

import (
	"bytes"
	"strings"
	"testing"
)

func TestEncodeString(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello", "5:hello"},
		{"", "0:"},
		{"spam", "4:spam"},
		{"a", "1:a"},
		{"hello world", "11:hello world"},
		{"\x00\x01\x02", "3:\x00\x01\x02"}, // Binary data
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, err := Encode(tt.input)
			if err != nil {
				t.Fatalf("Encode failed: %v", err)
			}
			if string(result) != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, string(result))
			}
		})
	}
}

func TestEncodeInt(t *testing.T) {
	tests := []struct {
		input    int64
		expected string
	}{
		{0, "i0e"},
		{42, "i42e"},
		{-42, "i-42e"},
		{1234567890, "i1234567890e"},
		{-1234567890, "i-1234567890e"},
	}

	for _, tt := range tests {
		t.Run(string(rune(tt.input)), func(t *testing.T) {
			result, err := Encode(tt.input)
			if err != nil {
				t.Fatalf("Encode failed: %v", err)
			}
			if string(result) != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, string(result))
			}
		})
	}
}

func TestEncodeList(t *testing.T) {
	tests := []struct {
		name     string
		input    []interface{}
		expected string
	}{
		{"empty list", []interface{}{}, "le"},
		{"single int", []interface{}{int64(42)}, "li42ee"},
		{"single string", []interface{}{"hello"}, "l5:helloe"},
		{"mixed", []interface{}{"spam", int64(42)}, "l4:spami42ee"},
		{"nested", []interface{}{[]interface{}{"a"}, int64(1)}, "ll1:aei1ee"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Encode(tt.input)
			if err != nil {
				t.Fatalf("Encode failed: %v", err)
			}
			if string(result) != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, string(result))
			}
		})
	}
}

func TestEncodeDict(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]interface{}
		expected string
	}{
		{"empty dict", map[string]interface{}{}, "de"},
		{
			"single key-value",
			map[string]interface{}{"key": "value"},
			"d3:key5:valuee",
		},
		{
			"integer value",
			map[string]interface{}{"num": int64(42)},
			"d3:numi42ee",
		},
		{
			"sorted keys",
			map[string]interface{}{
				"zebra": "z",
				"apple": "a",
				"banana": "b",
			},
			"d5:apple1:a6:banana1:b5:zebra1:ze",
		},
		{
			"nested dict",
			map[string]interface{}{
				"outer": map[string]interface{}{
					"inner": "value",
				},
			},
			"d5:outerd5:inner5:valueee",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Encode(tt.input)
			if err != nil {
				t.Fatalf("Encode failed: %v", err)
			}
			if string(result) != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, string(result))
			}
		})
	}
}

func TestDecodeString(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"5:hello", "hello"},
		{"0:", ""},
		{"4:spam", "spam"},
		{"1:a", "a"},
		{"11:hello world", "hello world"},
		{"3:\x00\x01\x02", "\x00\x01\x02"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, err := Decode([]byte(tt.input))
			if err != nil {
				t.Fatalf("Decode failed: %v", err)
			}
			str, ok := result.(string)
			if !ok {
				t.Fatalf("expected string, got %T", result)
			}
			if str != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, str)
			}
		})
	}
}

func TestDecodeInt(t *testing.T) {
	tests := []struct {
		input    string
		expected int64
	}{
		{"i0e", 0},
		{"i42e", 42},
		{"i-42e", -42},
		{"i1234567890e", 1234567890},
		{"i-1234567890e", -1234567890},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, err := Decode([]byte(tt.input))
			if err != nil {
				t.Fatalf("Decode failed: %v", err)
			}
			num, ok := result.(int64)
			if !ok {
				t.Fatalf("expected int64, got %T", result)
			}
			if num != tt.expected {
				t.Errorf("expected %d, got %d", tt.expected, num)
			}
		})
	}
}

func TestDecodeList(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []interface{}
	}{
		{"empty list", "le", []interface{}{}},
		{"single int", "li42ee", []interface{}{int64(42)}},
		{"single string", "l5:helloe", []interface{}{"hello"}},
		{"mixed", "l4:spami42ee", []interface{}{"spam", int64(42)}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Decode([]byte(tt.input))
			if err != nil {
				t.Fatalf("Decode failed: %v", err)
			}
			list, ok := result.([]interface{})
			if !ok {
				t.Fatalf("expected []interface{}, got %T", result)
			}
			if len(list) != len(tt.expected) {
				t.Errorf("expected list length %d, got %d", len(tt.expected), len(list))
			}
			for i, v := range list {
				if v != tt.expected[i] {
					t.Errorf("element %d: expected %v, got %v", i, tt.expected[i], v)
				}
			}
		})
	}
}

func TestDecodeDict(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected map[string]interface{}
	}{
		{"empty dict", "de", map[string]interface{}{}},
		{
			"single key-value",
			"d3:key5:valuee",
			map[string]interface{}{"key": "value"},
		},
		{
			"integer value",
			"d3:numi42ee",
			map[string]interface{}{"num": int64(42)},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Decode([]byte(tt.input))
			if err != nil {
				t.Fatalf("Decode failed: %v", err)
			}
			dict, ok := result.(map[string]interface{})
			if !ok {
				t.Fatalf("expected map[string]interface{}, got %T", result)
			}
			if len(dict) != len(tt.expected) {
				t.Errorf("expected dict length %d, got %d", len(tt.expected), len(dict))
			}
			for k, v := range dict {
				if v != tt.expected[k] {
					t.Errorf("key %q: expected %v, got %v", k, tt.expected[k], v)
				}
			}
		})
	}
}

func TestRoundTrip(t *testing.T) {
	tests := []struct {
		name  string
		value interface{}
	}{
		{"string", "hello"},
		{"int", int64(42)},
		{"negative int", int64(-42)},
		{"empty string", ""},
		{"zero", int64(0)},
		{
			"list",
			[]interface{}{"spam", int64(42)},
		},
		{
			"dict",
			map[string]interface{}{"key": "value", "num": int64(42)},
		},
		{
			"nested",
			map[string]interface{}{
				"list": []interface{}{int64(1), int64(2)},
				"dict": map[string]interface{}{"inner": "value"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Encode
			encoded, err := Encode(tt.value)
			if err != nil {
				t.Fatalf("Encode failed: %v", err)
			}

			// Decode
			decoded, err := Decode(encoded)
			if err != nil {
				t.Fatalf("Decode failed: %v", err)
			}

			// Verify round-trip
			if !deepEqual(decoded, tt.value) {
				t.Errorf("round-trip failed: expected %v, got %v", tt.value, decoded)
			}

			// Encode again and verify identical bytes
			encoded2, err := Encode(decoded)
			if err != nil {
				t.Fatalf("Re-encode failed: %v", err)
			}
			if !bytes.Equal(encoded, encoded2) {
				t.Errorf("re-encoding produced different bytes: %q vs %q", string(encoded), string(encoded2))
			}
		})
	}
}

func TestLeadingZeros(t *testing.T) {
	tests := []struct {
		input string
		valid bool
	}{
		{"i0e", true},
		{"i00e", false}, // Leading zero
		{"i01e", false}, // Leading zero
		{"i-0e", true},  // Allowed: -0
		{"-00e", false}, // Leading zero in negative
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			_, err := Decode([]byte(tt.input))
			if tt.valid && err != nil {
				t.Errorf("expected valid, got error: %v", err)
			}
			if !tt.valid && err == nil {
				t.Errorf("expected error for invalid input")
			}
		})
	}
}

func TestMalformedInput(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"empty input", ""},
		{"unclosed string", "5:hell"},
		{"invalid string length", "x:hello"},
		{"unclosed int", "i42"},
		{"unclosed list", "l1:a"},
		{"unclosed dict", "d3:key"},
		{"unexpected char", "x1:a"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Decode([]byte(tt.input))
			if err == nil {
				t.Errorf("expected error for malformed input")
			}
		})
	}
}

func TestDictKeySorting(t *testing.T) {
	// Create dict with intentionally unordered keys
	dict := map[string]interface{}{
		"zebra":  "z",
		"apple":  "a",
		"banana": "b",
	}

	encoded, err := Encode(dict)
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	// Expected: d5:apple1:a6:banana1:b5:zebra1:ze
	expected := "d5:apple1:a6:banana1:b5:zebra1:ze"
	if string(encoded) != expected {
		t.Errorf("expected %q, got %q", expected, string(encoded))
	}

	// Decode and re-encode to verify idempotence
	decoded, err := Decode(encoded)
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	reencoded, err := Encode(decoded)
	if err != nil {
		t.Fatalf("Re-encode failed: %v", err)
	}

	if !bytes.Equal(encoded, reencoded) {
		t.Errorf("dict re-encoding not idempotent: %q vs %q", string(encoded), string(reencoded))
	}
}

func TestStreamingDecoder(t *testing.T) {
	input := "l4:spami42ee"
	decoder := NewDecoder(strings.NewReader(input))
	result, err := decoder.Decode()
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	list, ok := result.([]interface{})
	if !ok {
		t.Fatalf("expected []interface{}, got %T", result)
	}

	if len(list) != 2 || list[0] != "spam" || list[1] != int64(42) {
		t.Errorf("unexpected result: %v", list)
	}
}

func TestDeepNesting(t *testing.T) {
	// Create deeply nested structure
	inner := map[string]interface{}{"value": "test"}
	outer := map[string]interface{}{"level1": inner}
	root := map[string]interface{}{"level0": outer}

	// Encode and decode
	encoded, err := Encode(root)
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	decoded, err := Decode(encoded)
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	if !deepEqual(decoded, root) {
		t.Errorf("deep nesting round-trip failed")
	}
}

// Helper function for deep equality comparison
func deepEqual(a, b interface{}) bool {
	switch av := a.(type) {
	case string:
		bv, ok := b.(string)
		return ok && av == bv
	case int64:
		bv, ok := b.(int64)
		return ok && av == bv
	case []interface{}:
		bv, ok := b.([]interface{})
		if !ok || len(av) != len(bv) {
			return false
		}
		for i := range av {
			if !deepEqual(av[i], bv[i]) {
				return false
			}
		}
		return true
	case map[string]interface{}:
		bv, ok := b.(map[string]interface{})
		if !ok || len(av) != len(bv) {
			return false
		}
		for k, v := range av {
			if !deepEqual(v, bv[k]) {
				return false
			}
		}
		return true
	default:
		return a == b
	}
}
