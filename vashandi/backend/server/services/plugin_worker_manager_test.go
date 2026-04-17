package services

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestWrapRpcErr(t *testing.T) {
	if err := wrapRpcErr(nil); err != nil {
		t.Errorf("expected nil for nil input, got %v", err)
	}

	standardErr := errors.New("test error")
	rpcError := wrapRpcErr(standardErr)
	if rpcError == nil {
		t.Fatal("expected non-nil *rpcErr")
	}
	if rpcError.Code != jsonrpcErrInternalError {
		t.Errorf("expected code %d, got %d", jsonrpcErrInternalError, rpcError.Code)
	}
	if rpcError.Message != "test error" {
		t.Errorf("expected message 'test error', got %q", rpcError.Message)
	}

	existingRpcErr := &rpcErr{Code: 123, Message: "custom"}
	wrappedAgain := wrapRpcErr(existingRpcErr)
	if wrappedAgain != existingRpcErr {
		t.Errorf("expected same pointer, got different: %v vs %v", wrappedAgain, existingRpcErr)
	}
	if wrappedAgain.Code != 123 || wrappedAgain.Message != "custom" {
		t.Errorf("expected code 123 and message 'custom', got %d and %q", wrappedAgain.Code, wrappedAgain.Message)
	}
}

func TestNormaliseID(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected int64
		ok       bool
	}{
		{"float64", float64(42), 42, true},
		{"int64", int64(100), 100, true},
		{"json.Number valid", json.Number("999"), 999, true},
		{"json.Number invalid", json.Number("abc"), 0, false},
		{"string", "not an id", -1, false},
		{"nil", nil, -1, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := normaliseID(tt.input)
			if ok != tt.ok {
				t.Errorf("expected ok=%v, got %v", tt.ok, ok)
			}
			if got != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, got)
			}
		})
	}
}
