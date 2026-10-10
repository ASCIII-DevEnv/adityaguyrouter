package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNotifierSendWebhook(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	n := NewNotifier()
	err := n.SendWebhook(context.Background(), server.URL, "Test Title", "Test Message")
	if err != nil {
		t.Fatalf("expected webhook success, got: %v", err)
	}
	if !called {
		t.Fatalf("expected webhook server to be called")
	}
}
