package handler

// APOptimizationHandler serves review-only operations. There is deliberately
// no HTTP endpoint that accepts arbitrary RF measurements or writes AP radios.
// A trusted, enrolled vendor adapter must produce immutable proposals first.
import (
 "database/sql"
 "encoding/json"
 "net/http"
 "regexp"
 "time"

 "github.com/gin-gonic/gin"
 "mega-magic/internal/auth"
 "mega-magic/internal/database"
)

var apProposalID = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
var apReviewHash = regexp.MustCompile(`^[0-9a-f]{64}$`)

type APOptimizationHandler struct { DB *database.DB }
type APReviewRow struct {
 ID string `json:"id"`
 DeviceID string `json:"device_id"`
 Status string `json:"status"`
 Proposal json.RawMessage `json:"proposal"`
 ProposalHash string `json:"proposal_hash"`
 Fingerprint string `json:"fingerprint"`
 ExpiresAt time.Time `json:"expires_at"`
 DecidedAt sql.NullTime `json:"-"`
}

// ownerOnly explicitly refuses admin, developer, viewer and unknown roles.
// Generic "at least owner" middleware is insufficient: the user requires the
// physical SME owner to authorize disruptive AP radio changes.
func apOwnerOnly(c *gin.Context)(string,bool) {
 biz:=auth.GetBusinessID(c)
 if biz==""{c.JSON(http.StatusUnauthorized,gin.H{"error":"tenant session required"});return "",false}
 if auth.GetUserRole(c)!="OWNER"{
  c.JSON(http.StatusForbidden,gin.H{"error":"only the owner may review or approve AP changes"})
  return "",false
 }
 return biz,true
}

func (h *APOptimizationHandler) Capabilities(c *gin.Context) {
 c.Header("Cache-Control","no-store")
 _,ok:=apOwnerOnly(c);if !ok{return}
 c.JSON(http.StatusOK,gin.H{
  "read_only_inventory":true,
  "proposal_review":h!=nil && h.DB!=nil,
  "automatic_proposals":false,
  "manual_proposals":false,
  "live_apply":false,
  "reason":"No verified model-specific RF observer, durable provider enrollment or live radio writer is connected",
 })
}

func(h *APOptimizationHandler) List(c *gin.Context) {
 c.Header("Cache-Control","no-store")
 biz,ok:=apOwnerOnly(c);if !ok{return}
 if h==nil||h.DB==nil{c.JSON(503,gin.H{"error":"AP approval store unavailable"});return}
 out:=make([]APReviewRow,0)
 err:=runInTenant(c,h.DB,biz,func(tx *sql.Tx)error{
  rows,e:=tx.QueryContext(c.Request.Context(),`SELECT id::text,device_id::text,status,
   proposal,proposal_hash,fingerprint,expires_at
   FROM ap_radio_proposals WHERE business_id=$1
   ORDER BY created_at DESC LIMIT 100`,biz)
  if e!=nil{return e}
  defer rows.Close()
  for rows.Next(){
   var row APReviewRow
   if e:=rows.Scan(&row.ID,&row.DeviceID,&row.Status,
    &row.Proposal,&row.ProposalHash,&row.Fingerprint,&row.ExpiresAt);e!=nil{return e}
   out=append(out,row)
  }
  return rows.Err()
 })
 if err!=nil{c.JSON(503,gin.H{"error":"AP proposal ledger unavailable"});return}
 c.JSON(http.StatusOK,gin.H{"proposals":out,"live_apply_enabled":false})
}

func(h *APOptimizationHandler) Decide(c *gin.Context) {
 c.Header("Cache-Control","no-store")
 biz,ok:=apOwnerOnly(c);if !ok{return}
 if h==nil||h.DB==nil{c.JSON(503,gin.H{"error":"AP approval store unavailable"});return}
 id:=c.Param("id")
 if !apProposalID.MatchString(id){c.JSON(400,gin.H{"error":"invalid proposal ID"});return}
 var body struct{
  Approve *bool `json:"approve"`
  ProposalHash string `json:"proposal_hash"`
  Fingerprint string `json:"fingerprint"`
 }
 if c.ShouldBindJSON(&body)!=nil||body.Approve==nil||
  !apReviewHash.MatchString(body.ProposalHash)||!apReviewHash.MatchString(body.Fingerprint){
  c.JSON(400,gin.H{"error":"explicit decision and exact reviewed hashes required"});return
 }
 ownerID:=auth.GetUserID(c)
 if !apProposalID.MatchString(ownerID){c.JSON(401,gin.H{"error":"owner identity unavailable"});return}
 var state string
 conflict:=false
 missing:=false
 err:=runInTenant(c,h.DB,biz,func(tx *sql.Tx)error{
  var saved APReviewRow
  e:=tx.QueryRowContext(c.Request.Context(),`SELECT status,proposal_hash,fingerprint,expires_at
   FROM ap_radio_proposals WHERE business_id=$1 AND id=$2 FOR UPDATE`,biz,id).
   Scan(&saved.Status,&saved.ProposalHash,&saved.Fingerprint,&saved.ExpiresAt)
  if e==sql.ErrNoRows{missing=true;return nil}
  if e!=nil{return e}
  if saved.Status!="pending"||!time.Now().Before(saved.ExpiresAt)||
   saved.ProposalHash!=body.ProposalHash||saved.Fingerprint!=body.Fingerprint{
   conflict=true;return nil
  }
  state="rejected";if *body.Approve{state="approved"}
  result,e:=tx.ExecContext(c.Request.Context(),`UPDATE ap_radio_proposals
   SET status=$4,decided_by=$5,decided_at=clock_timestamp()
   WHERE business_id=$1 AND id=$2 AND status='pending'
     AND expires_at>clock_timestamp()
     AND proposal_hash=$3 AND fingerprint=$6`,
   biz,id,body.ProposalHash,state,ownerID,body.Fingerprint)
  if e!=nil{return e}
  n,e:=result.RowsAffected();if e!=nil{return e}
  if n!=1{conflict=true}
  return nil
 })
 if missing{c.JSON(404,gin.H{"error":"proposal not found"});return}
 if conflict{c.JSON(409,gin.H{"error":"proposal expired, changed or already decided; review a fresh proposal"});return}
 if err!=nil{c.JSON(503,gin.H{"error":"AP decision could not be persisted"});return}
 c.JSON(200,gin.H{"id":id,"status":state,"live_ap_write":false})
}
