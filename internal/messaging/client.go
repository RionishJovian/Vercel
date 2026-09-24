package messaging

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/nats-io/nats.go"

	"mega-magic/internal/config"
	"mega-magic/internal/database"
	"mega-magic/internal/model"
)

type Client struct {
	conn     *nats.Conn
	js       nats.JetStreamContext
	repo     *database.NATSSubjectRepository
	subs     []*nats.Subscription
	subjects map[string]bool
	prefix   string
}

func NewClient(cfg config.NATSConfig, repo *database.NATSSubjectRepository) (*Client, error) {
	opts := []nats.Option{
		nats.Name("mega-magic-backend"),
		nats.MaxReconnects(cfg.MaxReconnects),
		nats.ReconnectWait(cfg.ReconnectWait),
		nats.PingInterval(cfg.PingInterval),
		nats.MaxPingsOutstanding(cfg.MaxOutstandingPings),
		nats.ErrorHandler(func(_ *nats.Conn, sub *nats.Subscription, err error) {
			if sub != nil {
				log.Printf("nats: error on subject %q: %v", sub.Subject, err)
			} else {
				log.Printf("nats: connection error: %v", err)
			}
		}),
		nats.ReconnectHandler(func(nc *nats.Conn) {
			log.Printf("nats: reconnected to %s", nc.ConnectedUrl())
		}),
		nats.DisconnectErrHandler(func(_ *nats.Conn, err error) {
			if err != nil {
				log.Printf("nats: disconnected: %v", err)
			}
		}),
	}

	conn, err := nats.Connect(cfg.URL, opts...)
	if err != nil {
		return nil, fmt.Errorf("nats connect: %w", err)
	}

	var js nats.JetStreamContext
	if cfg.JetStream {
		js, err = conn.JetStream()
		if err != nil {
			conn.Close()
			return nil, fmt.Errorf("nats jetstream: %w", err)
		}
	}

	log.Printf("nats: connected to %s (jetstream=%v)", conn.ConnectedUrl(), cfg.JetStream)

	return &Client{
		conn:     conn,
		js:       js,
		repo:     repo,
		subs:     make([]*nats.Subscription, 0),
		subjects: make(map[string]bool),
		prefix:   "mm",
	}, nil
}

func (c *Client) SubjectKey(businessID, subject string) string {
	return fmt.Sprintf("%s.%s.%s", c.prefix, businessID, subject)
}

func (c *Client) Publish(businessID, subject string, data []byte) error {
	key := c.SubjectKey(businessID, subject)
	if err := c.conn.Publish(key, data); err != nil {
		return fmt.Errorf("nats publish %s: %w", key, err)
	}

	if c.repo != nil {
		_, _ = c.repo.Register(&model.NATSSubject{
			BusinessID:    businessID,
			SubjectPrefix: subject,
		})
	}

	return nil
}

func (c *Client) PublishJetStream(businessID, subject string, data []byte, opts ...nats.PubOpt) error {
	if c.js == nil {
		return fmt.Errorf("jetstream not enabled")
	}
	key := c.SubjectKey(businessID, subject)

	_, err := c.js.Publish(key, data, opts...)
	if err != nil {
		return fmt.Errorf("nats js publish %s: %w", key, err)
	}

	if c.repo != nil {
		_, _ = c.repo.Register(&model.NATSSubject{
			BusinessID:    businessID,
			SubjectPrefix: subject,
		})
	}

	return nil
}

func (c *Client) Subscribe(businessID, subject string, handler func(msg *nats.Msg)) error {
	key := c.SubjectKey(businessID, subject)
	sub, err := c.conn.Subscribe(key, handler)
	if err != nil {
		return fmt.Errorf("nats subscribe %s: %w", key, err)
	}
	c.subs = append(c.subs, sub)
	c.subjects[key] = true
	return nil
}

func (c *Client) QueueSubscribe(businessID, subject, queue string, handler func(msg *nats.Msg)) error {
	key := c.SubjectKey(businessID, subject)
	sub, err := c.conn.QueueSubscribe(key, queue, handler)
	if err != nil {
		return fmt.Errorf("nats queue subscribe %s: %w", key, err)
	}
	c.subs = append(c.subs, sub)
	c.subjects[key] = true
	return nil
}

// SubscribePattern subscribes to a raw NATS subject pattern (wildcards allowed),
// bypassing the per-business prefix. Used by cross-tenant workers (e.g. the
// telemetry evaluator) that must see every tenant's traffic.
func (c *Client) SubscribePattern(pattern string, handler func(msg *nats.Msg)) error {
	sub, err := c.conn.Subscribe(pattern, handler)
	if err != nil {
		return fmt.Errorf("nats subscribe pattern %s: %w", pattern, err)
	}
	c.subs = append(c.subs, sub)
	c.subjects[pattern] = true
	return nil
}

func (c *Client) SubscribeJetStream(businessID, subject, durable string, handler func(msg *nats.Msg)) error {
	if c.js == nil {
		return fmt.Errorf("jetstream not enabled")
	}
	key := c.SubjectKey(businessID, subject)

	sub, err := c.js.Subscribe(key, handler, nats.Durable(durable), nats.ManualAck())
	if err != nil {
		return fmt.Errorf("nats js subscribe %s: %w", key, err)
	}
	c.subs = append(c.subs, sub)
	c.subjects[key] = true
	return nil
}

func (c *Client) Request(ctx context.Context, businessID, subject string, data []byte, timeout time.Duration) (*nats.Msg, error) {
	key := c.SubjectKey(businessID, subject)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	msg, err := c.conn.RequestWithContext(ctx, key, data)
	if err != nil {
		return nil, fmt.Errorf("nats request %s: %w", key, err)
	}
	return msg, nil
}

func (c *Client) IsConnected() bool {
	return c.conn != nil && c.conn.IsConnected()
}

func (c *Client) Close() {
	for _, sub := range c.subs {
		_ = sub.Unsubscribe()
	}
	c.conn.Close()
}