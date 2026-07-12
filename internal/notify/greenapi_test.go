package notify

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGreenAPISend(t *testing.T) {
	var gotPath, gotChatID, gotMsg string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		gotChatID, gotMsg = body["chatId"], body["message"]
		w.WriteHeader(200)
	}))
	defer srv.Close()

	cfg := `{"instance_id":"7103","token":"tok123","recipient":"972501234567","api_url":"` + srv.URL + `"}`
	n, err := NewGreenAPI(cfg)
	if err != nil {
		t.Fatalf("NewGreenAPI: %v", err)
	}
	if err := n.Send(context.Background(), Message{Title: "linkDown", Severity: "High", Emoji: "🟠"}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if gotPath != "/waInstance7103/sendMessage/tok123" {
		t.Errorf("path = %q", gotPath)
	}
	if gotChatID != "972501234567@c.us" {
		t.Errorf("chatId = %q", gotChatID)
	}
	if !strings.Contains(gotMsg, "linkDown") {
		t.Errorf("message = %q", gotMsg)
	}
}

func TestGreenAPIGroupRecipientPassThrough(t *testing.T) {
	var gotChatID string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		gotChatID = body["chatId"]
		w.WriteHeader(200)
	}))
	defer srv.Close()

	// A recipient already containing "@" must not get "@c.us" appended.
	cfg := `{"instance_id":"7103","token":"t","recipient":"120363000000000000@g.us","api_url":"` + srv.URL + `"}`
	n, err := NewGreenAPI(cfg)
	if err != nil {
		t.Fatalf("NewGreenAPI: %v", err)
	}
	if err := n.Send(context.Background(), Message{Title: "t"}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if gotChatID != "120363000000000000@g.us" {
		t.Errorf("chatId = %q (should not double-append @c.us)", gotChatID)
	}
}

func TestGreenAPITrimsFields(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(200)
	}))
	defer srv.Close()

	// Whitespace around instance_id/token would corrupt the URL if not trimmed.
	cfg := `{"instance_id":"  7103 ","token":" tok123 ","recipient":" 972501234567 ","api_url":" ` + srv.URL + ` "}`
	n, err := NewGreenAPI(cfg)
	if err != nil {
		t.Fatalf("NewGreenAPI: %v", err)
	}
	if err := n.Send(context.Background(), Message{Title: "t"}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if gotPath != "/waInstance7103/sendMessage/tok123" {
		t.Errorf("path = %q (fields not trimmed)", gotPath)
	}
}

func TestGreenAPIDefaultAPIURL(t *testing.T) {
	n, err := NewGreenAPI(`{"instance_id":"7103","token":"t","recipient":"972501234567"}`)
	if err != nil {
		t.Fatalf("NewGreenAPI: %v", err)
	}
	if n.cfg.APIURL != "https://api.green-api.com" {
		t.Errorf("default api_url = %q", n.cfg.APIURL)
	}
}

func TestGreenAPIRequiresFields(t *testing.T) {
	cases := []string{
		`{"token":"t","recipient":"r"}`,        // no instance_id
		`{"instance_id":"7103","recipient":"r"}`, // no token
		`{"instance_id":"7103","token":"t"}`,   // no recipient
	}
	for _, c := range cases {
		if _, err := NewGreenAPI(c); err == nil {
			t.Errorf("expected error for %s", c)
		}
	}
}

func TestGreenAPIGatewayError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad instance", 400)
	}))
	defer srv.Close()
	n, _ := NewGreenAPI(`{"instance_id":"7103","token":"t","recipient":"r","api_url":"` + srv.URL + `"}`)
	if err := n.Send(context.Background(), Message{Title: "t"}); err == nil {
		t.Fatal("expected gateway error")
	}
}

func TestBuildNotifierKnowsGreenAPI(t *testing.T) {
	if _, err := BuildNotifier(KindGreenAPI, `{"instance_id":"7103","token":"t","recipient":"r"}`); err != nil {
		t.Errorf("greenapi: %v", err)
	}
}
