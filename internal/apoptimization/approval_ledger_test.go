package apoptimization

import (
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "strings"
 "testing"
 "time"
)

func trustedProposal(now time.Time) Proposal {
 return Proposal{
  TenantID:"11111111-1111-4111-8111-111111111111",
  DeviceID:"22222222-2222-4222-8222-222222222222",
  Vendor:"tplink",Model:"EAP650",Band:Band24,Mode:"automatic",
  BeforeChannel:11,AfterChannel:1,BeforePowerDBm:17,AfterPowerDBm:17,
  Fingerprint:strings.Repeat("a",64),
  CreatedAt:now.Add(-15*time.Second),
  ExpiresAt:now.Add(4*time.Minute+45*time.Second),
  ApprovalRequired:true,
  Warnings:[]string{"Owner review required"},
 }
}

func TestTrustedProposalSealedForOwnerReview(t *testing.T){
 now:=time.Now().UTC()
 p:=trustedProposal(now)
 raw,hash,err:=SealProposal(p.TenantID,p,now)
 if err!=nil{t.Fatal(err)}
 if len(hash)!=64 || strings.Contains(string(raw),p.TenantID){
  t.Fatal("review must be sealed without leaking tenant identity")
 }
 var parsed Proposal
 if err=json.Unmarshal(raw,&parsed);err!=nil{t.Fatal(err)}
 if parsed.AfterChannel!=1 || parsed.Fingerprint!=p.Fingerprint{
  t.Fatal("typed proposal lost immutable review fields")
 }
 // The database stores JSONB and may reorder keys: the digest is calculated
 // from the typed JSON, not a textual Postgres JSONB string.
 normalized,err:=json.Marshal(parsed)
 if err!=nil{t.Fatal(err)}
 digest:=sha256.Sum256(normalized)
 if hex.EncodeToString(digest[:])!=hash{t.Fatal("proposal digest changed after JSON roundtrip")}
}

func TestTrustedProposalRejectsStaleForgeryAndNoop(t *testing.T){
 now:=time.Now().UTC()
 base:=trustedProposal(now)
 for _,tc:=range []struct{name string;change func(*Proposal)}{
  {"wrong tenant",func(p *Proposal){p.TenantID="33333333-3333-4333-8333-333333333333"}},
  {"not explicitly approved",func(p *Proposal){p.ApprovalRequired=false}},
  {"short digest",func(p *Proposal){p.Fingerprint="deadbeef"}},
  {"expired",func(p *Proposal){p.ExpiresAt=now.Add(-time.Second)}},
  {"too long",func(p *Proposal){p.ExpiresAt=now.Add(8*time.Minute)}},
  {"too old",func(p *Proposal){p.CreatedAt=now.Add(-2*time.Minute)}},
  {"future",func(p *Proposal){p.CreatedAt=now.Add(3*time.Minute)}},
  {"no change",func(p *Proposal){p.AfterChannel=p.BeforeChannel}},
  {"unrecognized mode",func(p *Proposal){p.Mode="auto_apply"}},
  {"wrong device identity",func(p *Proposal){p.DeviceID="not-a-uuid"}},
 }{
  t.Run(tc.name,func(t *testing.T){
   p:=base
   tc.change(&p)
   if _,_,err:=SealProposal(base.TenantID,p,now);err==nil{
    t.Fatal("untrusted or stale AP proposal accepted")
   }
  })
 }
}
