package telegram

import (
	"testing"

	"github.com/h1v3-io/h1v3/internal/connector"
)

// Verify Connector implements connector.Connector at compile time.
var _ connector.Connector = (*Connector)(nil)

func TestContains(t *testing.T) {
	ids := []int64{100, 200, 300}

	if !contains(ids, 200) {
		t.Error("expected 200 to be found")
	}
	if contains(ids, 999) {
		t.Error("expected 999 to not be found")
	}
	if contains(nil, 100) {
		t.Error("expected nil slice to return false")
	}
}
