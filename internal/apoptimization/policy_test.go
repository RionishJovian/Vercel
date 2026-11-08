package apoptimization

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeAP struct{
	state RadioState
	changes int
	ignore bool
	rollbackFail bool
}
func(f *fakeAP)Observe(context.Context,string,string,Band)(RadioState,error){return f.state,nil}
func(f *fakeAP)Apply(_ context.Context,from,to RadioState)error{
	f.changes++
	if f.rollbackFail && f.changes>1{return errors.New("rollback refused")}
	if !f.ignore {f.state=to}
	return nil
}
func exampleAP(now time.Time)RadioState{
	return RadioState{TenantID:"tenant-1",DeviceID:"ap-1",Vendor:"tplink",Model:"EAP",Band:Band24,
		Channel:11,PowerDBm:17,AllowedChannels:[]int{1,6,11},VerifiedController:true,
		APIRevision:"r1",ObservedAt:now.Add(-time.Minute),
		Candidates:[]ChannelScore{{Channel:1,Utilization:10,Interference:2},{Channel:6,Utilization:35,Interference:30},{Channel:11,Utilization:70,Interference:20}}}
}
func TestAutomaticProposalNeverAppliesWithoutOwner(t *testing.T){
	now:=time.Date(2026,11,8,6,0,0,0,time.UTC)
	s:=exampleAP(now)
	p,err:=ProposeAutomatic(s,now)
	if err!=nil{t.Fatal(err)}
	if p.AfterChannel!=1||p.Mode!="automatic"||!p.ApprovalRequired{t.Fatalf("bad proposal %#v",p)}
	f:=&fakeAP{state:s}
	if err:=ApplyApproved(context.Background(),f,p,now,false);!errors.Is(err,ErrUnapproved){t.Fatalf("unapproved apply=%v",err)}
	if f.changes!=0{t.Fatal("automatic proposal wrote to AP without owner")}
	if err:=ApplyApproved(context.Background(),f,p,now,true);err!=nil{t.Fatal(err)}
	if f.state.Channel!=1||f.changes!=1{t.Fatal("approved change not verified")}
}
func TestManualRejectsUnapprovedChannelsOrMissingEvidence(t *testing.T){
	now:=time.Now().UTC()
	s:=exampleAP(now)
	if _,err:=ProposeManual(s,13,17,now);err==nil{t.Fatal("country-disallowed channel accepted")}
	if _,err:=ProposeManual(s,6,17,now);err!=nil{t.Fatal(err)}
	s.VerifiedController=false
	if _,err:=ProposeAutomatic(s,now);!errors.Is(err,ErrNoEvidence){t.Fatalf("unsupported controller accepted: %v",err)}
	s.VerifiedController=true
	s.ObservedAt=now.Add(-10*time.Minute)
	if _,err:=ProposeAutomatic(s,now);!errors.Is(err,ErrNoEvidence){t.Fatalf("stale AP RF scan accepted: %v",err)}
}
func TestAPApprovalRefusesStaleStateAndExpiredProposal(t *testing.T){
	now:=time.Now().UTC()
	s:=exampleAP(now)
	p,err:=ProposeManual(s,6,17,now)
	if err!=nil{t.Fatal(err)}
	f:=&fakeAP{state:s}
	f.state.APIRevision="unreviewed-change"
	if err:=ApplyApproved(context.Background(),f,p,now,true);!errors.Is(err,ErrStale){t.Fatal(err)}
	if f.changes!=0{t.Fatal("stale review touched AP")}
	f.state=s
	if err:=ApplyApproved(context.Background(),f,p,now.Add(6*time.Minute),true);!errors.Is(err,ErrStale){t.Fatalf("expired review accepted: %v",err)}
	if f.changes!=0{t.Fatal("expired review touched AP")}
}
func TestAPApplyFailsClosedAndRollsBackUnverifiedChange(t *testing.T){
	now:=time.Now().UTC();s:=exampleAP(now)
	p,err:=ProposeManual(s,6,17,now)
	if err!=nil{t.Fatal(err)}
	f:=&fakeAP{state:s,ignore:true}
	if err:=ApplyApproved(context.Background(),f,p,now,true);err==nil{t.Fatal("unverified radio setting counted as success")}
	if f.changes!=2{t.Fatalf("verification failure must attempt rollback, got %d",f.changes)}
	f=&fakeAP{state:s,ignore:true,rollbackFail:true}
	if err:=ApplyApproved(context.Background(),f,p,now,true);err==nil{t.Fatal("failed rollback requires manual recovery")}
}
