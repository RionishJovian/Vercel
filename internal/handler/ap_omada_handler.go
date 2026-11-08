package handler

// OmadaControllerHandler stores only owner-enrolled controller identifiers and
// AES-GCM ciphertext. It exposes read-only device inventory and NEVER an AP
// configuration endpoint. Configured server origin/CIDR allowlists bound egress.
import (
 "database/sql"
 "errors"
 "net/url"
 "strings"

 "github.com/gin-gonic/gin"
 "mega-magic/internal/apoptimization/omada"
 "mega-magic/internal/auth"
 "mega-magic/internal/database"
 "mega-magic/internal/secretbox"
)

type OmadaControllerHandler struct {
 DB *database.DB
 Box *secretbox.Box
 Caller omada.Caller
 AllowedOrigins map[string]bool
}
type omadaRow struct{
 ID string `json:"id"`
 BaseURL string `json:"base_url"`
 OmadaID string `json:"omada_id"`
 SiteID string `json:"site_id"`
 ClientID string `json:"-"`
 Secret string `json:"-"`
 CreatedAt string `json:"created_at"`
}
func (h *OmadaControllerHandler) ready()bool{return h!=nil&&h.DB!=nil&&h.Box!=nil&&h.Caller!=nil&&len(h.AllowedOrigins)>0}
func omadaOrigin(input string)string{
 u,e:=url.Parse(input)
 if e!=nil||u.Scheme!="https"||u.Host==""||u.User!=nil||u.RawQuery!=""||
  u.Fragment!=""||u.Path!=""&&u.Path!="/"||u.Opaque!=""{
  return ""
 }
 return strings.TrimRight(u.String(),"/")
}

func(h *OmadaControllerHandler) Origins(c *gin.Context){
 c.Header("Cache-Control","no-store")
 _,ok:=apOwnerOnly(c);if !ok{return}
 if !h.ready(){c.JSON(503,gin.H{"error":"Omada enrollment is unavailable"});return}
 origins:=make([]string,0,len(h.AllowedOrigins))
 for origin:=range h.AllowedOrigins{origins=append(origins,origin)}
 c.JSON(200,gin.H{"allowed_origins":origins,"read_only":true,"radio_apply_enabled":false})
}
func(h *OmadaControllerHandler) List(c *gin.Context){
 c.Header("Cache-Control","no-store")
 biz,ok:=apOwnerOnly(c);if !ok{return}
 if !h.ready(){c.JSON(503,gin.H{"error":"Omada enrollment is unavailable"});return}
 out:=make([]omadaRow,0)
 err:=runInTenant(c,h.DB,biz,func(tx *sql.Tx)error{
  rows,e:=tx.QueryContext(c.Request.Context(),`SELECT id::text,base_url,omada_id,site_id,created_at::text
   FROM ap_omada_connections WHERE business_id=$1 ORDER BY created_at DESC LIMIT 20`,biz)
  if e!=nil{return e}
  defer rows.Close()
  for rows.Next(){
   var row omadaRow
   if e:=rows.Scan(&row.ID,&row.BaseURL,&row.OmadaID,&row.SiteID,&row.CreatedAt);e!=nil{return e}
   out=append(out,row)
  }
  return rows.Err()
 })
 if err!=nil{c.JSON(503,gin.H{"error":"Omada connections unavailable"});return}
 c.JSON(200,gin.H{"controllers":out,"read_only":true})
}
func(h *OmadaControllerHandler) Create(c *gin.Context){
 c.Header("Cache-Control","no-store")
 biz,ok:=apOwnerOnly(c);if !ok{return}
 if !h.ready(){c.JSON(503,gin.H{"error":"Omada enrollment is unavailable"});return}
 var request struct{
  BaseURL string `json:"base_url"`
  OmadaID string `json:"omada_id"`
  SiteID string `json:"site_id"`
  ClientID string `json:"client_id"`
  ClientSecret string `json:"client_secret"`
 }
 if c.ShouldBindJSON(&request)!=nil{
  c.JSON(400,gin.H{"error":"invalid Omada connection request"});return
 }
 origin:=omadaOrigin(request.BaseURL)
 if origin==""||!h.AllowedOrigins[origin] {
  c.JSON(400,gin.H{"error":"controller origin is not enabled by the server operator"});return
 }
 if len(request.ClientSecret)<16||len(request.ClientSecret)>2048 ||
  strings.ContainsAny(request.ClientSecret,"\r\n"){
  c.JSON(400,gin.H{"error":"invalid Omada client secret"});return
 }
 // This is *only* a shape check. No live controller requests during enrollment
 // and no assertion that the AP, controller or API credentials work.
 if _,err:=omada.New(omada.Settings{BaseURL:origin,OmadaID:request.OmadaID,
  SiteID:request.SiteID,ClientID:request.ClientID,ClientSecret:request.ClientSecret},
  h.Caller);err!=nil{
  c.JSON(400,gin.H{"error":"invalid Omada controller identification"});return
 }
 sealed,err:=h.Box.Seal(request.ClientSecret)
 if err!=nil{c.JSON(503,gin.H{"error":"Omada encryption unavailable"});return}
 ownerID:=auth.GetUserID(c)
 if !apProposalID.MatchString(ownerID) {
  c.JSON(401,gin.H{"error":"owner identity unavailable"});return
 }
 var id string
 conflict:=false
 err=runInTenant(c,h.DB,biz,func(tx *sql.Tx)error{
  var count int
  if e:=tx.QueryRowContext(c.Request.Context(),`SELECT count(*) FROM ap_omada_connections
   WHERE business_id=$1`,biz).Scan(&count);e!=nil{return e}
  if count>=10{conflict=true;return nil}
  e:=tx.QueryRowContext(c.Request.Context(),`INSERT INTO ap_omada_connections
   (business_id,base_url,omada_id,site_id,client_id,client_secret_ciphertext,created_by)
   VALUES($1,$2,$3,$4,$5,$6,$7)
   ON CONFLICT(business_id,base_url,omada_id,site_id) DO NOTHING
   RETURNING id::text`,biz,origin,request.OmadaID,request.SiteID,
   request.ClientID,sealed,ownerID).Scan(&id)
  if errors.Is(e,sql.ErrNoRows){conflict=true;return nil}
  return e
 })
 if conflict{c.JSON(409,gin.H{"error":"controller already enrolled or tenant controller limit reached"});return}
 if err!=nil{c.JSON(503,gin.H{"error":"Omada connection could not be saved"});return}
 c.JSON(201,gin.H{"id":id,"state":"saved_unverified","read_only":true})
}

func(h *OmadaControllerHandler) Inventory(c *gin.Context){
 c.Header("Cache-Control","no-store")
 biz,ok:=apOwnerOnly(c);if !ok{return}
 if !h.ready(){c.JSON(503,gin.H{"error":"Omada enrollment unavailable"});return}
 id:=c.Param("id")
 if !apProposalID.MatchString(id){c.JSON(400,gin.H{"error":"invalid controller identity"});return}
 var row omadaRow
 err:=runInTenant(c,h.DB,biz,func(tx *sql.Tx)error{
  return tx.QueryRowContext(c.Request.Context(),`SELECT base_url,omada_id,site_id,
   client_id,client_secret_ciphertext FROM ap_omada_connections
   WHERE business_id=$1 AND id=$2`,biz,id).
   Scan(&row.BaseURL,&row.OmadaID,&row.SiteID,&row.ClientID,&row.Secret)
 })
 if errors.Is(err,sql.ErrNoRows){c.JSON(404,gin.H{"error":"controller not found"});return}
 if err!=nil{c.JSON(503,gin.H{"error":"controller identity unavailable"});return}
 if !h.AllowedOrigins[row.BaseURL]{
  c.JSON(403,gin.H{"error":"controller origin is no longer allowed"});return
 }
 secret,err:=h.Box.Open(row.Secret)
 if err!=nil{c.JSON(503,gin.H{"error":"controller credentials unavailable"});return}
 client,err:=omada.New(omada.Settings{BaseURL:row.BaseURL,OmadaID:row.OmadaID,
  SiteID:row.SiteID,ClientID:row.ClientID,ClientSecret:secret},h.Caller)
 if err!=nil{c.JSON(503,gin.H{"error":"controller settings incompatible"});return}
 inventory,err:=client.ReadDeviceInventory(c.Request.Context())
 if err!=nil{c.JSON(503,gin.H{"error":"Omada inventory read failed; verify access and controller API permissions"});return}
 c.JSON(200,inventory)
}

func(h *OmadaControllerHandler) Delete(c *gin.Context){
 c.Header("Cache-Control","no-store")
 biz,ok:=apOwnerOnly(c);if !ok{return}
 if !h.ready(){c.JSON(503,gin.H{"error":"Omada enrollment unavailable"});return}
 id:=c.Param("id")
 if !apProposalID.MatchString(id){c.JSON(400,gin.H{"error":"invalid controller identity"});return}
 var deleted bool
 err:=runInTenant(c,h.DB,biz,func(tx *sql.Tx)error{
  result,e:=tx.ExecContext(c.Request.Context(),`DELETE FROM ap_omada_connections
   WHERE business_id=$1 AND id=$2`,biz,id)
  if e!=nil{return e}
  count,e:=result.RowsAffected();deleted=count==1;return e
 })
 if err!=nil{c.JSON(503,gin.H{"error":"controller could not be revoked"});return}
 if !deleted{c.JSON(404,gin.H{"error":"controller not found"});return}
 c.JSON(200,gin.H{"revoked":true})
}
