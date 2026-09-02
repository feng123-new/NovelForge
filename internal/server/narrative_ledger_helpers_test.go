package server

import (
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/voocel/ainovel-cli/internal/narrativeledger"
)

func TestNarrativeLedgerStrictJSON(t *testing.T) {
	request := httptest.NewRequest("POST", "/", strings.NewReader(`{"chapter":1,"key":"seal","unknown":true}`))
	response := httptest.NewRecorder()
	var input foreshadowRequest
	err := decodeLedgerJSON(response, request, &input)
	if !errors.Is(err, narrativeledger.ErrValidation) {
		t.Fatalf("unknown field was accepted: %v", err)
	}

	request = httptest.NewRequest("POST", "/", strings.NewReader(`{"chapter":1,"key":"seal"} {"chapter":2}`))
	response = httptest.NewRecorder()
	err = decodeLedgerJSON(response, request, &input)
	if !errors.Is(err, narrativeledger.ErrValidation) {
		t.Fatalf("multiple JSON objects were accepted: %v", err)
	}
}

func TestNarrativeLedgerIdempotencyHeader(t *testing.T) {
	request := httptest.NewRequest("POST", "/", nil)
	response := httptest.NewRecorder()
	if _, ok := requireLedgerIdempotency(response, request); ok || response.Code != 400 {
		t.Fatalf("missing Idempotency-Key was accepted: status=%d", response.Code)
	}
	request = httptest.NewRequest("POST", "/", nil)
	request.Header.Set("Idempotency-Key", "ledger-test-1")
	response = httptest.NewRecorder()
	value, ok := requireLedgerIdempotency(response, request)
	if !ok || value != "ledger-test-1" {
		t.Fatalf("valid Idempotency-Key was rejected: %q", value)
	}
}
