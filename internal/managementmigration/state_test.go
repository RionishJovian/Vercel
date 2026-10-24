package managementmigration

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

type simulation struct {
	job                   Job
	target                string
	old, new, timer, dual bool
	fail                  string
	lostSave              string
	lostSwitch            bool
	log                   []string
}

func (s *simulation) event(name string) error {
	s.log = append(s.log, name)
	if s.fail == name {
		return errors.New("injected lost response")
	}
	return nil
}
func (s *simulation) Save(_ context.Context, j *Job, previous string) error {
	if s.job.Phase != previous || s.job.Revision != j.Revision {
		return errors.New("CAS conflict")
	}
	s.job = *j
	s.job.Revision++
	if s.lostSave == j.Phase {
		return errors.New("commit response lost")
	}
	j.Revision++
	return nil
}
func (s *simulation) SwitchConnection(_ context.Context, j *Job) error {
	if e := s.event("save_target"); e != nil {
		return e
	}
	if s.timer || !s.new || !s.dual || !s.old {
		return errors.New("unsafe switch")
	}
	s.target = "new"
	j.Phase = "connection_saved"
	j.Revision++
	s.job = *j
	if s.lostSwitch {
		return errors.New("commit response lost")
	}
	return nil
}
func (s *simulation) Stage(_ context.Context, _ *Job) error {
	s.dual = true
	return s.event("hub_stage")
}
func (s *simulation) Commit(_ context.Context, _ *Job) error { return s.event("hub_commit") }
func (s *simulation) Finalize(_ context.Context, _ *Job) error {
	s.dual = false
	return s.event("hub_finalize")
}
func (s *simulation) Abort(_ context.Context, _ *Job) error {
	if s.target != "old" {
		return errors.New("rollback after target switch")
	}
	s.dual = false
	return s.event("hub_abort")
}

type simRouter struct{ *simulation }

func (s simRouter) VerifyOld(context.Context, *Job) error {
	if !s.old {
		return errors.New("old absent")
	}
	return s.event("verify_old")
}
func (s simRouter) Stage(context.Context, *Job) error {
	s.new = true
	s.timer = true
	return s.event("router_stage")
}
func (s simRouter) VerifyNew(context.Context, *Job) error {
	if !s.new {
		return errors.New("new absent")
	}
	return s.event("verify_new")
}
func (s simRouter) RetainNew(context.Context, *Job) error {
	s.timer = false
	return s.event("retain_new")
}
func (s simRouter) RemoveOld(context.Context, *Job) error {
	if s.target != "new" || !s.new {
		return errors.New("unsafe cleanup")
	}
	s.old = false
	return s.event("remove_old")
}
func (s simRouter) Rollback(context.Context, *Job) error {
	if s.target != "old" || !s.old {
		return errors.New("unsafe rollback")
	}
	s.new = false
	return s.event("router_rollback")
}
func fixture() (Engine, *simulation, *Job) {
	j := Job{ID: "fixture", Phase: "planned"}
	s := &simulation{job: j, target: "old", old: true}
	return Engine{s, s, simRouter{s}}, s, &j
}
func TestManagementMoveVerifiedOrdering(t *testing.T) {
	e, s, j := fixture()
	if err := e.Apply(context.Background(), j); err != nil {
		t.Fatal(err)
	}
	want := []string{"verify_old", "hub_stage", "router_stage", "verify_new", "hub_commit", "retain_new", "save_target", "verify_new", "remove_old", "hub_finalize"}
	if !reflect.DeepEqual(s.log, want) || s.job.Phase != "completed" || s.target != "new" || s.old || s.timer {
		t.Fatalf("%+v", s)
	}
}
func TestManagementMoveFailuresBeforeSwitchRecoverOld(t *testing.T) {
	for _, step := range []string{"hub_stage", "router_stage", "verify_new", "hub_commit", "retain_new", "save_target"} {
		t.Run(step, func(t *testing.T) {
			e, s, j := fixture()
			s.fail = step
			if e.Apply(context.Background(), j) == nil {
				t.Fatal("failure ignored")
			}
			s.fail = ""
			reloaded := s.job
			if err := e.Recover(context.Background(), &reloaded); err != nil {
				t.Fatal(err)
			}
			if s.target != "old" || !s.old || s.new || s.dual || s.job.Phase != "rolled_back" {
				t.Fatalf("unsafe recovery %+v", s)
			}
		})
	}
}
func TestManagementMoveLostCheckpointResponseReloadsDurablePhase(t *testing.T) {
	for _, phase := range []string{"starting", "hub_staged", "router_staged", "new_verified", "hub_committed", "router_retained"} {
		t.Run(phase, func(t *testing.T) {
			e, s, j := fixture()
			s.lostSave = phase
			if e.Apply(context.Background(), j) == nil {
				t.Fatal("lost save ignored")
			}
			s.lostSave = ""
			fresh := s.job
			if err := e.Recover(context.Background(), &fresh); err != nil {
				t.Fatal(err)
			}
			if s.target != "old" || !s.old || s.new {
				t.Fatal("old path lost")
			}
		})
	}
}
func TestManagementMoveLostTargetCommitNeverRollsBackBlindly(t *testing.T) {
	e, s, j := fixture()
	s.lostSwitch = true
	if e.Apply(context.Background(), j) == nil {
		t.Fatal("uncertain commit ignored")
	}
	fresh := s.job
	if fresh.Phase != "connection_saved" {
		t.Fatal("missing atomic checkpoint")
	}
	if err := e.Recover(context.Background(), &fresh); err != nil {
		t.Fatal(err)
	}
	if s.target != "new" || s.old || s.job.Phase != "completed" {
		t.Fatalf("wrong recovery %+v", s)
	}
	for _, entry := range s.log {
		if entry == "hub_abort" || entry == "router_rollback" {
			t.Fatal("blind rollback")
		}
	}
}
func TestManagementMoveCleanupFailureResumesForward(t *testing.T) {
	for _, step := range []string{"remove_old", "hub_finalize"} {
		t.Run(step, func(t *testing.T) {
			e, s, j := fixture()
			s.fail = step
			if e.Apply(context.Background(), j) == nil {
				t.Fatal("failure ignored")
			}
			s.fail = ""
			fresh := s.job
			if err := e.Recover(context.Background(), &fresh); err != nil {
				t.Fatal(err)
			}
			if s.target != "new" || s.old || s.dual || s.job.Phase != "completed" {
				t.Fatalf("bad cleanup %+v", s)
			}
		})
	}
}
