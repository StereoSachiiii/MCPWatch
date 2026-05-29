package transport

import (
	"reflect"
	"testing"
)

func TestExtractJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    []string 
		expected []string 
	}{
		{
			name:     "Single complete object",
			input:    []string{`{"jsonrpc":"2.0","method":"ping"}`},
			expected: []string{`{"jsonrpc":"2.0","method":"ping"}`},
		},
		{
			name:     "Single complete array",
			input:    []string{`[1, 2, 3]`},
			expected: []string{`[1, 2, 3]`},
		},
		{
			name:     "Fragmented object in two writes",
			input:    []string{`{"jsonrpc":"2.0",`, `"method":"ping"}`},
			expected: []string{`{"jsonrpc":"2.0","method":"ping"}`},
		},
		{
			name:     "Brackets inside string values are ignored",
			input:    []string{`{"message": "Hello {world} [test]"}`},
			expected: []string{`{"message": "Hello {world} [test]"}`},
		},
		{
			name:     "Escaped quotes in strings",
			input:    []string{`{"quote": "He said, \"hello\"."}`},
			expected: []string{`{"quote": "He said, \"hello\"."}`},
		},
		{
			name:     "Leading garbage is discarded",
			input:    []string{`Server started on port 3000` + "\n" + `{"jsonrpc":"2.0"}`},
			expected: []string{`{"jsonrpc":"2.0"}`},
		},
		{
			name:     "Leading garbage fragmented",
			input:    []string{`Server started `, `on port 3000` + "\n" + `{"jsonrpc":"2.0"}`},
			expected: []string{`{"jsonrpc":"2.0"}`},
		},
		{
			name:     "Multiple objects sequentially",
			input:    []string{`{"msg":1}{"msg":2}`},
			expected: []string{`{"msg":1}`, `{"msg":2}`},
		},
		{
			name: "Multiple objects with trailing garbage and spacing",
			input: []string{
				`  ` + "\n" + `{"msg":1}` + "\n" + `Some random logs` + "\n" + `{"msg":2}`,
				`Some trailing garbage`,
			},
			expected: []string{`{"msg":1}`, `{"msg":2}`},
		},
		{
			name:     "Complex nested structure",
			input:    []string{`{"a": {"b": [1, {"c": 2}]}}`},
			expected: []string{`{"a": {"b": [1, {"c": 2}]}}`},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sb := &streamBuffer{}
			var actual []string

			for _, chunk := range tc.input {
				sb.buf.WriteString(chunk)
				msgs := sb.extractJSON()
				actual = append(actual, msgs...)
			}

			if !reflect.DeepEqual(actual, tc.expected) {
				t.Errorf("expected: %v, got: %v", tc.expected, actual)
			}
		})
	}
}
