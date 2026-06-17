package engine

import (
	"encoding/json"
	"testing"
)

func TestParseJSONRPC(t *testing.T) {
	tests := []struct {
		name          string
		raw           string
		direction     string
		transport     string
		expectNil     bool
		expectType    MsgType
		expectMethod  string
		expectID      string
		expectHasErr  bool
		expectErrCode string
	}{
		{
			name:       "Valid request",
			raw:        `{"jsonrpc":"2.0","id":"123","method":"initialize","params":{}}`,
			direction:  "IN",
			transport:  "stdio",
			expectType: MsgTypeRequest,
			expectMethod: "initialize",
			expectID:   "123",
		},
		{
			name:       "Valid response success",
			raw:        `{"jsonrpc":"2.0","id":456,"result":{"protocolVersion":"2024-11-05"}}`,
			direction:  "OUT",
			transport:  "stdio",
			expectType: MsgTypeResponse,
			expectID:   "456",
		},
		{
			name:       "Valid response error",
			raw:        `{"jsonrpc":"2.0","id":"789","error":{"code":-32601,"message":"Method not found"}}`,
			direction:  "OUT",
			transport:  "stdio",
			expectType: MsgTypeResponse,
			expectID:   "789",
			expectHasErr: true,
			expectErrCode: "-32601",
		},
		{
			name:       "Notification",
			raw:        `{"jsonrpc":"2.0","method":"notifications/initialized"}`,
			direction:  "IN",
			transport:  "stdio",
			expectType: MsgTypeNotification,
			expectMethod: "notifications/initialized",
			expectID:   "",
		},
		{
			name:       "Invalid JSON",
			raw:        `{invalid json`,
			direction:  "IN",
			transport:  "stdio",
			expectNil:  true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			msg := ParseJSONRPC(tc.raw, tc.direction, tc.transport)
			if tc.expectNil {
				if msg != nil {
					t.Fatalf("expected nil message, got %v", msg)
				}
				return
			}

			if msg == nil {
				t.Fatalf("expected non-nil message")
			}

			if msg.Direction != tc.direction {
				t.Errorf("expected Direction %q, got %q", tc.direction, msg.Direction)
			}

			if msg.Transport != tc.transport {
				t.Errorf("expected Transport %q, got %q", tc.transport, msg.Transport)
			}

			if msg.MsgType != tc.expectType {
				t.Errorf("expected MsgType %q, got %q", tc.expectType, msg.MsgType)
			}

			if msg.Method != tc.expectMethod {
				t.Errorf("expected Method %q, got %q", tc.expectMethod, msg.Method)
			}

			if msg.JSONRPCID != tc.expectID {
				t.Errorf("expected JSONRPCID %q, got %q", tc.expectID, msg.JSONRPCID)
			}

			if tc.expectHasErr {
				if msg.ErrorData == "" || msg.ErrorData == "null" {
					t.Errorf("expected error data to be set")
				}
				if msg.ErrorCode != tc.expectErrCode {
					t.Errorf("expected error code %q, got %q", tc.expectErrCode, msg.ErrorCode)
				}
			}

			if msg.Raw != tc.raw {
				t.Errorf("expected Raw %q, got %q", tc.raw, msg.Raw)
			}

			// Validate SizeBytes
			if msg.SizeBytes != int64(len(tc.raw)) {
				t.Errorf("expected SizeBytes %d, got %d", len(tc.raw), msg.SizeBytes)
			}

			// Validate TokenEstimate
			expectedTokens := int64(len(tc.raw)) / 4
			if msg.TokenEstimate != expectedTokens {
				t.Errorf("expected TokenEstimate %d, got %d", expectedTokens, msg.TokenEstimate)
			}

			// Validate params and result parsing
			var obj map[string]interface{}
			_ = json.Unmarshal([]byte(tc.raw), &obj)
			if params, ok := obj["params"]; ok {
				var p interface{}
				_ = json.Unmarshal([]byte(msg.Params), &p)
				expectedP, _ := json.Marshal(params)
				actualP, _ := json.Marshal(p)
				if string(expectedP) != string(actualP) {
					t.Errorf("expected Params %s, got %s", string(expectedP), string(actualP))
				}
			}

			if result, ok := obj["result"]; ok {
				var r interface{}
				_ = json.Unmarshal([]byte(msg.Result), &r)
				expectedR, _ := json.Marshal(result)
				actualR, _ := json.Marshal(r)
				if string(expectedR) != string(actualR) {
					t.Errorf("expected Result %s, got %s", string(expectedR), string(actualR))
				}
			}
		})
	}
}
