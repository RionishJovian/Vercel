package messaging

import (
	"testing"

	"github.com/nats-io/nats.go"
)

// TestClient_Reply_requiresReplySubject proves the answering half of
// request/reply fails closed on a message with no reply subject, without needing
// a live NATS connection.
func TestClient_Reply_requiresReplySubject(t *testing.T) {
	c := &Client{}
	if err := c.Reply(nil, []byte("x")); err == nil {
		t.Fatal("nil message must error")
	}
	if err := c.Reply(&nats.Msg{}, []byte("x")); err == nil {
		t.Fatal("message without a reply subject must error")
	}
}
