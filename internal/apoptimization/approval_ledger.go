package apoptimization

// Ledger persists *trusted server-generated* radio proposals, not user-supplied
// telemetry. This package deliberately exposes no HTTP proposal-create route.
import (
 "context"
 "crypto/sha256"
 "database/sql"
 "encoding/hex"
 "encoding/json"
 "errors"
 "regexp"
 "time"

 "mega-magic/internal/database"
)

var ledgerUUID=regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
var ledgerSHA=regexp.MustCompile(`^[0-9a-f]{64}$`)

type Ledger struct { DB *database.DB }

// SealProposal revalidates the trusted policy output and binds the immutable
// client review snapshot to a SHA-256 digest. It excludes hidden tenant ID
// from JSON while retaining it in the tenant-scoped database column.
func SealProposal(tenant string,p Proposal,now time.Time)([]byte,string,error){
 if !ledgerUUID.MatchString(tenant)||!ledgerUUID.MatchString(p.DeviceID)||
  p.TenantID!=tenant||!ledgerSHA.MatchString(p.Fingerprint)||
  !p.ApprovalRequired||(p.Mode!="manual"&&p.Mode!="automatic")||
  p.BeforeChannel<=0||p.AfterChannel<=0||
  p.BeforeChannel==p.AfterChannel&&p.BeforePowerDBm==p.AfterPowerDBm||
  p.CreatedAt.IsZero()||p.ExpiresAt.IsZero()||
  p.CreatedAt.After(now.Add(time.Second))||now.After(p.CreatedAt.Add(time.Minute))||
  !p.ExpiresAt.After(p.CreatedAt)||p.ExpiresAt.Sub(p.CreatedAt)>5*time.Minute||
  !now.Before(p.ExpiresAt) {
  return nil,"",errors.New("AP proposal not trusted, fresh, or owner-reviewable")
 }
 raw,err:=json.Marshal(p)
 if err!=nil{return nil,"",err}
 sha:=sha256.Sum256(raw)
 return raw,hex.EncodeToString(sha[:]),nil
}

func (l *Ledger) RecordTrustedProposal(ctx context.Context,tenant string,p Proposal)(string,error){
 if l==nil||l.DB==nil{return "",errors.New("AP approval database unavailable")}
 raw,hash,err:=SealProposal(tenant,p,time.Now())
 if err!=nil{return "",err}
 tx,err:=l.DB.BeginTx(ctx,nil)
 if err!=nil{return "",err}
 defer tx.Rollback()
 var id string
 err=database.WithTenant(tx,tenant,func()error{
  return tx.QueryRowContext(ctx,`INSERT INTO ap_radio_proposals
    (business_id,device_id,proposal,proposal_hash,fingerprint,created_at,expires_at)
   VALUES ($1,$2,$3::jsonb,$4,$5,$6,$7) RETURNING id::text`,
   tenant,p.DeviceID,string(raw),hash,p.Fingerprint,p.CreatedAt,p.ExpiresAt).Scan(&id)
 })
 if err!=nil{return "",err}
 if err=tx.Commit();err!=nil{return "",err}
 return id,nil
}

// ConsumeOwnerApproval is the only DB path allowed to turn an owner decision
// into a one-time execution authorization. It atomically transitions the row
// before any external AP request, so a retry cannot reapply an uncertain write.
//
// This API remains UNWIRED until a verified vendor adapter is enrolled. It
// cannot on its own change an AP and must not be used to skip model readback.
func (l *Ledger) ConsumeOwnerApproval(ctx context.Context,tenant,proposalID string,now time.Time)(Proposal,error){
 if l==nil||l.DB==nil||!ledgerUUID.MatchString(tenant)||!ledgerUUID.MatchString(proposalID){
  return Proposal{},ErrUnapproved
 }
 tx,err:=l.DB.BeginTx(ctx,nil)
 if err!=nil{return Proposal{},err}
 defer tx.Rollback()
 var p Proposal
 err=database.WithTenant(tx,tenant,func()error{
  var raw []byte
  var savedHash,fingerprint string
  var expiry time.Time
  err:=tx.QueryRowContext(ctx,`SELECT proposal,proposal_hash,fingerprint,expires_at
   FROM ap_radio_proposals WHERE business_id=$1 AND id=$2 AND status='approved'
     AND expires_at>$3 FOR UPDATE`,tenant,proposalID,now).
   Scan(&raw,&savedHash,&fingerprint,&expiry)
  if err==sql.ErrNoRows{return ErrUnapproved}
  if err!=nil{return err}
  if err=json.Unmarshal(raw,&p);err!=nil{return ErrStale}
  // PostgreSQL jsonb reorders object keys. Re-marshal the known typed
  // proposal to reproduce the trusted canonical Go JSON before comparing
  // the proposal hash (never hash Postgres' textual JSONB representation).
  canonical,err:=json.Marshal(p)
  if err!=nil{return ErrStale}
  sum:=sha256.Sum256(canonical)
  if hex.EncodeToString(sum[:])!=savedHash||!ledgerSHA.MatchString(fingerprint){
   return ErrStale
  }
  // TenantID is deliberately not serialized. Re-bind from the transaction's
  // tenant context; refuse any mismatching device/profile identity.
  p.TenantID=tenant
  if p.Fingerprint!=fingerprint||p.ExpiresAt.After(expiry.Add(time.Millisecond))||
   p.DeviceID==""||!p.ApprovalRequired||p.ExpiresAt.Before(now) {return ErrStale}
  result,err:=tx.ExecContext(ctx,`UPDATE ap_radio_proposals SET
   status='consumed',consumed_at=clock_timestamp()
   WHERE business_id=$1 AND id=$2 AND status='approved'
     AND expires_at>$3`,tenant,proposalID,now)
  if err!=nil{return err}
  rows,err:=result.RowsAffected()
  if err!=nil{return err}
  if rows!=1{return ErrUnapproved}
  return nil
 })
 if err!=nil{return Proposal{},err}
 if err=tx.Commit();err!=nil{return Proposal{},err}
 return p,nil
}
