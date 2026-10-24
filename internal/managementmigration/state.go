// Package managementmigration coordinates a pinned management-address move.
// Durable checkpoints retain the old path until the new path is verified.
package managementmigration

import (
	"context"
	"errors"
	"fmt"
)

type Job struct {
	ID          string `json:"id"`
	BusinessID  string `json:"business_id"`
	DeviceID    string `json:"device_id"`
	RequestedBy string `json:"requested_by"`
	OldAddress  string `json:"old_address"`
	NewAddress  string `json:"new_address"`
	Interface   string `json:"interface"`
	OldRecord   string `json:"old_record"`
	Fingerprint string `json:"-"`
	RouterKey   string `json:"-"`
	Guard       string `json:"-"`
	Backup      string `json:"backup,omitempty"`
	Phase       string `json:"phase"`
	Revision    int    `json:"revision"`
	ExpiresAt   string `json:"expires_at"`
}
type Store interface {
	Save(context.Context, *Job, string) error
	SwitchConnection(context.Context, *Job) error
}
type Hub interface {
	Stage(context.Context, *Job) error
	Commit(context.Context, *Job) error
	Finalize(context.Context, *Job) error
	Abort(context.Context, *Job) error
}
type Router interface {
	VerifyOld(context.Context, *Job) error
	Stage(context.Context, *Job) error
	VerifyNew(context.Context, *Job) error
	RetainNew(context.Context, *Job) error
	RemoveOld(context.Context, *Job) error
	Rollback(context.Context, *Job) error
}
type Engine struct {
	Store  Store
	Hub    Hub
	Router Router
}

var ErrRecoveryRequired = errors.New("migration needs recovery; refresh its durable status before retrying")

func (e Engine) checkpoint(ctx context.Context, j *Job, next string) error {
	previous := j.Phase
	j.Phase = next
	if err := e.Store.Save(ctx, j, previous); err != nil {
		j.Phase = previous
		return err
	}
	return nil
}
func (e Engine) Apply(ctx context.Context, j *Job) error {
	if j.Phase != "planned" {
		return fmt.Errorf("migration already started; refresh status and use recovery")
	}
	if err := e.Router.VerifyOld(ctx, j); err != nil {
		return err
	}
	if err := e.checkpoint(ctx, j, "starting"); err != nil {
		return err
	}
	steps := []struct {
		phase string
		run   func(context.Context, *Job) error
	}{{"hub_staged", e.Hub.Stage}, {"router_staged", e.Router.Stage}, {"new_verified", e.Router.VerifyNew}, {"hub_committed", e.Hub.Commit}, {"router_retained", e.Router.RetainNew}}
	for _, s := range steps {
		if err := s.run(ctx, j); err != nil {
			return fmt.Errorf("%w at %s", ErrRecoveryRequired, s.phase)
		}
		if err := e.checkpoint(ctx, j, s.phase); err != nil {
			return ErrRecoveryRequired
		}
	}
	// Target and connection_saved are one transaction. An uncertain commit must
	// be resolved by reloading the journal, never by blindly rolling back.
	if err := e.Store.SwitchConnection(ctx, j); err != nil {
		return ErrRecoveryRequired
	}
	return e.finish(ctx, j)
}
func (e Engine) finish(ctx context.Context, j *Job) error {
	if err := e.Router.VerifyNew(ctx, j); err != nil {
		return ErrRecoveryRequired
	}
	if err := e.Router.RemoveOld(ctx, j); err != nil {
		return ErrRecoveryRequired
	}
	if err := e.checkpoint(ctx, j, "router_cleaned"); err != nil {
		return ErrRecoveryRequired
	}
	if err := e.Hub.Finalize(ctx, j); err != nil {
		return ErrRecoveryRequired
	}
	return e.checkpoint(ctx, j, "completed")
}

// Recover requires a freshly loaded job and the same per-device lease.
func (e Engine) Recover(ctx context.Context, j *Job) error {
	switch j.Phase {
	case "completed", "rolled_back":
		return nil
	case "connection_saved", "router_cleaned":
		return e.finish(ctx, j)
	case "planned":
		return e.checkpoint(ctx, j, "rolled_back")
	default:
		if err := e.Router.VerifyOld(ctx, j); err != nil {
			return ErrRecoveryRequired
		}
		if err := e.Hub.Abort(ctx, j); err != nil {
			return ErrRecoveryRequired
		}
		if err := e.Router.Rollback(ctx, j); err != nil {
			return ErrRecoveryRequired
		}
		return e.checkpoint(ctx, j, "rolled_back")
	}
}

// UserError contains only an explicit, reviewed product message. Transport,
// credential and database errors must never be echoed directly to the browser.
type UserError struct{ Code, Message string }

func (e *UserError) Error() string       { return e.Message }
func Explain(code, message string) error { return &UserError{code, message} }
