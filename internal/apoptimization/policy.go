// Package apoptimization defines the shared read/review/owner-approve contract
// for third-party Wi-Fi access-point radios. There is NO generic RouterOS
// write path here: each vendor/model must provide its own verified adapter.
package apoptimization

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"
)

var (
	ErrUnsupported = errors.New("AP radio API is not verified for this exact model")
	ErrStale = errors.New("AP radio settings changed since owner review")
	ErrUnapproved = errors.New("explicit owner approval of reviewed AP change required")
	ErrNoEvidence = errors.New("fresh AP radio measurements unavailable")
)

type Band string
const (
	Band24 Band="2.4GHz"
	Band5 Band="5GHz"
)

type ChannelScore struct {
	Channel int `json:"channel"`
	Utilization int `json:"utilization_percent"`
	Interference int `json:"interference_percent"`
}

type RadioState struct {
	TenantID string `json:"-"`
	DeviceID string `json:"device_id"`
	Vendor string `json:"vendor"`
	Model string `json:"model"`
	Band Band `json:"band"`
	Channel int `json:"channel"`
	PowerDBm int `json:"power_dbm"`
	AllowedChannels []int `json:"allowed_channels"`
	Candidates []ChannelScore `json:"candidates"`
	ObservedAt time.Time `json:"observed_at"`
	VerifiedController bool `json:"verified_controller"`
	APIRevision string `json:"api_revision"`
}

// Proposal is immutable client-visible review evidence. Version and expiry
// protect against stale telemetry and approval of a different AP radio.
type Proposal struct {
	TenantID string `json:"-"`
	DeviceID string `json:"device_id"`
	Vendor string `json:"vendor"`
	Model string `json:"model"`
	Band Band `json:"band"`
	Mode string `json:"mode"`
	BeforeChannel int `json:"before_channel"`
	AfterChannel int `json:"after_channel"`
	BeforePowerDBm int `json:"before_power_dbm"`
	AfterPowerDBm int `json:"after_power_dbm"`
	Fingerprint string `json:"fingerprint"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
	Warnings []string `json:"warnings"`
	ApprovalRequired bool `json:"approval_required"`
}

// RadioAdapter is an explicitly enrolled model-specific provider. These are
// not generic SSH commands. Adapter implementations MUST supply a readback
// and reversible apply, and refuse unsupported firmware/models.
type RadioAdapter interface {
	Observe(ctx context.Context, tenantID, deviceID string, band Band) (RadioState,error)
	Apply(ctx context.Context, before RadioState, desired RadioState) error
}

func digest(s RadioState) (string,error) {
	// Exclude transient candidate scores and timestamp; only radio config
	// revision, tenant, identity, current state and legal channel set fence apply.
	shape:=struct{
		TenantID,DeviceID,Vendor,Model,APIRevision string
		Band Band
		Channel,PowerDBm int
		AllowedChannels []int
	}{s.TenantID,s.DeviceID,s.Vendor,s.Model,s.APIRevision,s.Band,s.Channel,s.PowerDBm,append([]int(nil),s.AllowedChannels...)}
	sort.Ints(shape.AllowedChannels)
	b,err:=json.Marshal(shape);if err!=nil{return "",err}
	sum:=sha256.Sum256(b)
	return hex.EncodeToString(sum[:]),nil
}

func allowed(s RadioState, channel int) bool {
	for _,c:=range s.AllowedChannels{if c==channel{return true}}
	return false
}

func validObservation(s RadioState,now time.Time) error {
	if s.TenantID==""||s.DeviceID==""||s.Vendor==""||s.Model==""||s.APIRevision==""||
		(s.Band!=Band24 && s.Band!=Band5)||!s.VerifiedController||
		s.Channel<=0||!allowed(s,s.Channel)||s.PowerDBm<0||s.PowerDBm>40||
		s.ObservedAt.After(now.Add(time.Second))||now.Sub(s.ObservedAt)>2*time.Minute{
		return ErrNoEvidence
	}
	return nil
}

// ProposeManual records a manually selected legal channel, never an automatic
// action. Does not change a radio.
func ProposeManual(s RadioState,channel,power int,now time.Time)(Proposal,error){
	if err:=validObservation(s,now);err!=nil{return Proposal{},err}
	if !allowed(s,channel)||power<0||power>40{return Proposal{},errors.New("channel or transmit power unavailable for this AP")}
	if channel==s.Channel && power==s.PowerDBm{return Proposal{},errors.New("no radio change requested")}
	return makeProposal(s,channel,power,"manual",now)
}

// ProposeAutomatic chooses only among measured allowed candidates. Without
// fresh measurements it REFUSES instead of inventing a best channel.
func ProposeAutomatic(s RadioState,now time.Time)(Proposal,error){
	if err:=validObservation(s,now);err!=nil{return Proposal{},err}
	type score struct{channel,value int}
	var choices []score
	seen:=map[int]bool{}
	for _,c:=range s.Candidates{
		if seen[c.Channel]||!allowed(s,c.Channel)||c.Utilization<0||c.Utilization>100||c.Interference<0||c.Interference>100{
			return Proposal{},ErrNoEvidence
		}
		seen[c.Channel]=true
		choices=append(choices,score{c.Channel,c.Utilization+c.Interference})
	}
	if len(choices)<2{return Proposal{},ErrNoEvidence}
	sort.Slice(choices,func(i,j int)bool{if choices[i].value==choices[j].value{return choices[i].channel<choices[j].channel};return choices[i].value<choices[j].value})
	cur:=0
	found:=false
	for _,v:=range choices{if v.channel==s.Channel{cur=v.value;found=true;break}}
	if !found{return Proposal{},ErrNoEvidence}
	best:=choices[0]
	// Difference under 10 combined points is weak evidence; no churn.
	if best.channel==s.Channel||cur-best.value<10{return Proposal{},errors.New("no supported improvement demonstrated by AP radio measurements")}
	return makeProposal(s,best.channel,s.PowerDBm,"automatic",now)
}

func makeProposal(s RadioState,channel,power int,mode string,now time.Time)(Proposal,error){
	hash,err:=digest(s);if err!=nil{return Proposal{},err}
	return Proposal{TenantID:s.TenantID,DeviceID:s.DeviceID,Vendor:s.Vendor,Model:s.Model,Band:s.Band,
		Mode:mode,BeforeChannel:s.Channel,AfterChannel:channel,BeforePowerDBm:s.PowerDBm,AfterPowerDBm:power,
		Fingerprint:hash,CreatedAt:now,ExpiresAt:now.Add(5*time.Minute),ApprovalRequired:true,
		Warnings:[]string{"Changing a radio channel may disconnect guests. Owner approval is required before any AP write.","Vendor capability and country/regulatory channel rules must be verified by the managed AP controller."}},nil
}

// ApplyApproved is the only model-independent live write coordinator.
// Callers must FIRST verify tenant-owner authorization and a one-time durable
// approval bound to this specific proposal; those checks are supplied as a
// fail-closed approval predicate. An automatic proposal never applies itself.
func ApplyApproved(ctx context.Context,adapter RadioAdapter,p Proposal,now time.Time,ownerApproved bool)(err error){
	if adapter==nil{return ErrUnsupported}
	if !ownerApproved||!p.ApprovalRequired{return ErrUnapproved}
	if now.Before(p.CreatedAt)||!now.Before(p.ExpiresAt)||p.TenantID==""||p.DeviceID==""||
		len(p.Fingerprint)!=64{return ErrStale}
	before,err:=adapter.Observe(ctx,p.TenantID,p.DeviceID,p.Band)
	if err!=nil{return fmt.Errorf("AP current radio read failed: %w",err)}
	if err=validObservation(before,now);err!=nil{return err}
	hash,err:=digest(before)
	if err!=nil{return err}
	if hash!=p.Fingerprint||before.Channel!=p.BeforeChannel||before.PowerDBm!=p.BeforePowerDBm||
		before.Vendor!=p.Vendor||before.Model!=p.Model{return ErrStale}
	if !allowed(before,p.AfterChannel)||p.AfterPowerDBm<0||p.AfterPowerDBm>40{return ErrStale}
	after:=before;after.Channel=p.AfterChannel;after.PowerDBm=p.AfterPowerDBm
	if err=adapter.Apply(ctx,before,after);err!=nil{return fmt.Errorf("AP change not confirmed: %w",err)}
	confirmed,e:=adapter.Observe(ctx,p.TenantID,p.DeviceID,p.Band)
	if e==nil&&confirmed.Channel==after.Channel&&confirmed.PowerDBm==after.PowerDBm&&confirmed.VerifiedController{return nil}
	// Bounded rollback uses captured before-state, never a guessed default.
	rollbackErr:=adapter.Apply(ctx,after,before)
	if rollbackErr!=nil{return errors.Join(errors.New("AP change not verified; manual recovery required"),rollbackErr)}
	recovered,e:=adapter.Observe(ctx,p.TenantID,p.DeviceID,p.Band)
	if e!=nil||recovered.Channel!=before.Channel||recovered.PowerDBm!=before.PowerDBm{
		return errors.New("AP rollback readback failed; recovery required")
	}
	return errors.New("AP change rolled back after failed readback")
}
