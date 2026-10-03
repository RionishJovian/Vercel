// Package relay provides bounded, ephemeral outbound-PC requests. No media or
// credentials are persisted; an API restart fails every outstanding request closed.
package relay

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"sync"
	"time"
)

var ErrUnavailable = errors.New("media computer unavailable")

type Task struct {
	ID        string    `json:"id"`
	Operation string    `json:"operation"`
	Library   string    `json:"library"`
	Item      string    `json:"item,omitempty"`
	Range     string    `json:"range,omitempty"`
	Start     int       `json:"start,omitempty"`
	Expires   time.Time `json:"expires"`
}
type Response struct {
	Status int
	Type   string
	Length int64
	Range  string
	Body   io.ReadCloser
}
type Metadata struct {
	Status        int    `json:"status"`
	ContentType   string `json:"content_type"`
	ContentLength int64  `json:"content_length"`
	ContentRange  string `json:"content_range,omitempty"`
}
type request struct {
	task                 Task
	business, credential string
	ctx                  context.Context
	check                func(context.Context) error
	leased, delivered    bool
	response             chan Response
	done                 chan struct{}
}
type Broker struct {
	mu       sync.Mutex
	requests map[string]*request
}

func New() *Broker { return &Broker{requests: make(map[string]*request)} }
func Secret() string {
	var b [32]byte
	if _, e := rand.Read(b[:]); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b[:])
}

// Request owns the lifetime of the upload. finish MUST run after consumption,
// including client cancellation; the connector cannot complete a different task.
func (b *Broker) Request(ctx context.Context, business string, task Task, check func(context.Context) error) (Response, func(), error) {
	if check == nil || check(ctx) != nil {
		return Response{}, nil, ErrUnavailable
	}
	task.ID = Secret()
	r := &request{task: task, business: business, ctx: ctx, check: check, response: make(chan Response), done: make(chan struct{})}
	b.mu.Lock()
	count := 0
	for _, v := range b.requests {
		if v.business == business {
			count++
		}
	}
	if len(b.requests) >= 64 || count >= 4 {
		b.mu.Unlock()
		return Response{}, nil, ErrUnavailable
	}
	b.requests[task.ID] = r
	b.mu.Unlock()
	var once sync.Once
	finish := func() { once.Do(func() { b.mu.Lock(); delete(b.requests, task.ID); close(r.done); b.mu.Unlock() }) }
	timer := time.NewTimer(15 * time.Second)
	defer timer.Stop()
	select {
	case response := <-r.response:
		return response, finish, nil
	case <-ctx.Done():
		finish()
		return Response{}, nil, ErrUnavailable
	case <-timer.C:
		finish()
		return Response{}, nil, ErrUnavailable
	}
}
func (b *Broker) Poll(business, credential string) (Task, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, r := range b.requests {
		if r.business == business && !r.leased && r.ctx.Err() == nil && time.Now().Before(r.task.Expires) {
			r.leased = true
			r.credential = credential
			return r.task, true
		}
	}
	return Task{}, false
}
func (b *Broker) lookup(business, credential, id string) *request {
	b.mu.Lock()
	defer b.mu.Unlock()
	r := b.requests[id]
	if r == nil || r.business != business || r.credential != credential || !r.leased || r.ctx.Err() != nil || !time.Now().Before(r.task.Expires) {
		return nil
	}
	return r
}
func (b *Broker) Check(ctx context.Context, business, credential, id string) error {
	r := b.lookup(business, credential, id)
	if r == nil {
		return ErrUnavailable
	}
	return r.check(ctx)
}
func (b *Broker) Context(business, credential, id string) (context.Context, error) {
	r := b.lookup(business, credential, id)
	if r == nil {
		return nil, ErrUnavailable
	}
	return r.ctx, nil
}
func (b *Broker) Deliver(ctx context.Context, business, credential, id string, response Response) error {
	r := b.lookup(business, credential, id)
	if r == nil || r.check(ctx) != nil {
		return ErrUnavailable
	}
	b.mu.Lock()
	if r.delivered {
		b.mu.Unlock()
		return ErrUnavailable
	}
	r.delivered = true
	b.mu.Unlock()
	select {
	case r.response <- response:
	case <-r.done:
		return ErrUnavailable
	case <-r.ctx.Done():
		return ErrUnavailable
	case <-ctx.Done():
		return ErrUnavailable
	}
	select {
	case <-r.done:
		return nil
	case <-r.ctx.Done():
		return ErrUnavailable
	case <-ctx.Done():
		return ErrUnavailable
	}
}
