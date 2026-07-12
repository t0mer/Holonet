// Package notify delivers classified events to notification channels
// (design §3.5). A Notifier abstracts one channel kind; the Dispatcher fans a
// Message out to many channels concurrently with per-channel timeout and retry.
package notify

import (
	"context"
	"strings"
)

// Channel kinds.
const (
	KindShoutrrr = "shoutrrr"
	KindWhatsApp = "whatsapp"
	KindGreenAPI = "greenapi"
	KindWebhook  = "webhook"
)

// Field is a labeled value shown in a message body.
type Field struct {
	Name  string
	Value string
}

// Message is a channel-agnostic notification payload.
type Message struct {
	Title    string
	Body     string
	Severity string // severity level name, e.g. "High"
	Emoji    string // severity emoji
	Fields   []Field
}

// Notifier delivers a Message to a single configured destination.
type Notifier interface {
	Send(ctx context.Context, msg Message) error
	Kind() string
}

// renderPlain renders a Message as plain text suitable for most channels.
func renderPlain(msg Message) string {
	var b strings.Builder
	if msg.Emoji != "" {
		b.WriteString(msg.Emoji)
		b.WriteString(" ")
	}
	if msg.Severity != "" {
		b.WriteString("[")
		b.WriteString(msg.Severity)
		b.WriteString("] ")
	}
	b.WriteString(msg.Title)
	if msg.Body != "" {
		b.WriteString("\n")
		b.WriteString(msg.Body)
	}
	for _, f := range msg.Fields {
		b.WriteString("\n")
		b.WriteString(f.Name)
		b.WriteString(": ")
		b.WriteString(f.Value)
	}
	return b.String()
}
