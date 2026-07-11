package notify

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWhatsAppSend(t *testing.T) {
	var gotPhone, gotMsg, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/send/message" {
			t.Errorf("path = %s", r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		gotPhone, gotMsg = body["phone"], body["message"]
		w.WriteHeader(200)
	}))
	defer srv.Close()

	cfg := `{"base_url":"` + srv.URL + `","recipient":"972501234567","token":"tok123"}`
	n, err := NewWhatsApp(cfg)
	if err != nil {
		t.Fatalf("NewWhatsApp: %v", err)
	}
	if err := n.Send(context.Background(), Message{Title: "linkDown", Severity: "High", Emoji: "🟠"}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if gotPhone != "972501234567" {
		t.Errorf("phone = %q", gotPhone)
	}
	if !strings.Contains(gotMsg, "linkDown") {
		t.Errorf("message = %q", gotMsg)
	}
	if gotAuth != "Bearer tok123" {
		t.Errorf("auth = %q", gotAuth)
	}
}

func TestWhatsAppRequiresFields(t *testing.T) {
	if _, err := NewWhatsApp(`{"base_url":"http://x"}`); err == nil {
		t.Fatal("expected error without recipient")
	}
}

func TestWhatsAppGatewayError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad phone", 400)
	}))
	defer srv.Close()
	n, _ := NewWhatsApp(`{"base_url":"` + srv.URL + `","recipient":"x"}`)
	if err := n.Send(context.Background(), Message{Title: "t"}); err == nil {
		t.Fatal("expected gateway error")
	}
}

func TestWebhookDefaultJSONBody(t *testing.T) {
	var body map[string]any
	var ct string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ct = r.Header.Get("Content-Type")
		json.NewDecoder(r.Body).Decode(&body)
		w.WriteHeader(204)
	}))
	defer srv.Close()

	n, err := NewWebhook(`{"url":"` + srv.URL + `","headers":{"X-Token":"abc"}}`)
	if err != nil {
		t.Fatalf("NewWebhook: %v", err)
	}
	if err := n.Send(context.Background(), Message{Title: "linkDown", Severity: "High"}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if ct != "application/json" {
		t.Errorf("content-type = %q", ct)
	}
	if body["title"] != "linkDown" || body["severity"] != "High" {
		t.Errorf("body = %v", body)
	}
}

func TestWebhookCustomTemplate(t *testing.T) {
	var raw string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		raw = string(b)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	n, err := NewWebhook(`{"url":"` + srv.URL + `","body_template":"EVENT={{.Title}} SEV={{.Severity}}"}`)
	if err != nil {
		t.Fatalf("NewWebhook: %v", err)
	}
	n.Send(context.Background(), Message{Title: "coldStart", Severity: "High"})
	if raw != "EVENT=coldStart SEV=High" {
		t.Errorf("rendered body = %q", raw)
	}
}

func TestWebhookRequiresURL(t *testing.T) {
	if _, err := NewWebhook(`{}`); err == nil {
		t.Fatal("expected error without url")
	}
}

func TestBuildNotifierKnowsAllKinds(t *testing.T) {
	if _, err := BuildNotifier(KindWhatsApp, `{"base_url":"http://x","recipient":"y"}`); err != nil {
		t.Errorf("whatsapp: %v", err)
	}
	if _, err := BuildNotifier(KindWebhook, `{"url":"http://x"}`); err != nil {
		t.Errorf("webhook: %v", err)
	}
	if _, err := BuildNotifier("bogus", `{}`); err == nil {
		t.Error("expected error for unknown kind")
	}
}
