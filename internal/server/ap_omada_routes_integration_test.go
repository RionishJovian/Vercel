package server

import (
 "context"
 "database/sql"
 "encoding/json"
 "net/http/httptest"
 "os"
 "strings"
 "testing"
 "time"

 "github.com/gin-gonic/gin"
 "mega-magic/internal/auth"
 "mega-magic/internal/config"
 "mega-magic/internal/database"
 "mega-magic/internal/model"
)

func TestOmadaControllerSecretSealedAndTenantScoped(t *testing.T){
 db:=subscriptionHTTPDatabase(t)
 for _,path:=range []string{
  "../../db/migrations/20261108190000_ap_radio_owner_approval.sql",
  "../../db/migrations/20261108193000_ap_omada_enrollment.sql",
 }{
  data,e:=os.ReadFile(path);if e!=nil{t.Fatal(e)}
  if _,e=db.Exec(string(data));e!=nil{t.Fatalf("migration %s failed: %v",path,e)}
 }
 svc,e:=auth.NewService("disposable-omada-enrollment-fixture-secret",time.Hour,time.Hour,4)
 if e!=nil{t.Fatal(e)}
 svc.UseSessions(db)
 user:=func(email string,role model.UserRole,biz string)(*model.User,string){
  t.Helper()
  u:=&model.User{Email:email,UserRole:role,PasswordHash:"fixture",BusinessID:biz}
  if biz==""{
   if e=db.QueryRow(`INSERT INTO businesses(company_name,owner_email,access_tier,device_quota_limit)
    VALUES('Controller fixture',$1,'SMALL_WISP',10) RETURNING business_id`,email).Scan(&u.BusinessID);e!=nil{t.Fatal(e)}
  }
  if e=db.QueryRow(`INSERT INTO users(business_id,email,full_name,password_hash,user_role)
   VALUES($1,$2,'Fixture','fixture',$3) RETURNING user_id`,
   u.BusinessID,email,role).Scan(&u.UserID);e!=nil{t.Fatal(e)}
  token,_,e:=svc.StartSession(u)
  if e!=nil{t.Fatal(e)}
  return u,token
 }
 owner,ownerToken:=user("omada-owner@example.test",model.RoleOwner,"")
 _,foreignToken:=user("omada-foreign@example.test",model.RoleOwner,"")
 _,adminToken:=user("omada-admin@example.test",model.RoleAdmin,owner.BusinessID)
 cfg:=&config.Config{}
 cfg.APOptimization=config.APOptimizationConfig{
  Enabled:true,OmadaInventoryEnabled:true,
  OmadaCredentialKey:"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
  OmadaAllowedOrigins:[]string{"https://omada.example.test"},
  OmadaAllowedCIDRs:[]string{"127.0.0.1/32"},
 }
 r:=gin.New()
 if e=registerAPOptimizationRoutes(r,cfg,db,svc);e!=nil{t.Fatal(e)}
 request:=func(method,path,token,body string)*httptest.ResponseRecorder{
  w:=httptest.NewRecorder()
  req:=httptest.NewRequest(method,"https://ap.test"+path,strings.NewReader(body))
  req.Header.Set("Authorization","Bearer "+token)
  req.Header.Set("Content-Type","application/json")
  req.Header.Set("Origin","https://ap.test")
  req.Header.Set("X-MM-CSRF","1")
  r.ServeHTTP(w,req)
  return w
 }
 path:="/api/v1/ap-optimization/omada"
 secret:="A-tenant-owned-Omada-OpenAPI-secret-for-tests"
 payload,_:=json.Marshal(map[string]string{
  "base_url":"https://omada.example.test","omada_id":"controller",
  "site_id":"site","client_id":"client-id","client_secret":secret,
 })
 if w:=request("POST",path,adminToken,string(payload));w.Code!=403{
  t.Fatalf("admin saved AP credentials: %d",w.Code)
 }
 wrong:=strings.ReplaceAll(string(payload),"https://omada.example.test","https://evil.example.test")
 if w:=request("POST",path,ownerToken,wrong);w.Code!=400{
  t.Fatalf("unapproved host enrolled: %d",w.Code)
 }
 w:=request("POST",path,ownerToken,string(payload))
 if w.Code!=201{t.Fatalf("owner enrollment %d: %s",w.Code,w.Body.String())}
 if strings.Contains(w.Body.String(),secret){t.Fatal("API echoed controller secret")}
 var created struct{ ID string `json:"id"`;State string `json:"state"` }
 if e=json.Unmarshal(w.Body.Bytes(),&created);e!=nil||created.ID==""||created.State!="saved_unverified"{
  t.Fatal("enrollment was incorrectly claimed live/verified")
 }
 // Inspect ciphertext through an explicitly tenant-bound transaction, never
 // with the owner DB pool's unscoped RLS bypass.
 tx,e:=db.BeginTx(context.Background(),&sql.TxOptions{ReadOnly:true})
 if e!=nil{t.Fatal(e)}
 defer tx.Rollback()
 var sealed string
 e=database.WithTenant(tx,owner.BusinessID,func()error{
  return tx.QueryRow(`SELECT client_secret_ciphertext FROM ap_omada_connections
   WHERE business_id=$1 AND id=$2`,owner.BusinessID,created.ID).Scan(&sealed)
 })
 if e!=nil{t.Fatal(e)}
 if sealed==secret||strings.Contains(sealed,secret)||len(sealed)<40{
  t.Fatal("client secret stored as plaintext or truncated")
 }
 if w=request("GET",path,foreignToken,"");w.Code!=200||strings.Contains(w.Body.String(),created.ID){
  t.Fatalf("foreign tenant saw controller: %d %s",w.Code,w.Body.String())
 }
 if w=request("GET",path,ownerToken,"");w.Code!=200||!strings.Contains(w.Body.String(),created.ID)||strings.Contains(w.Body.String(),secret){
  t.Fatalf("owner controller list unsafe: %d %s",w.Code,w.Body.String())
 }
 if w=request("DELETE",path+"/"+created.ID,foreignToken,"");w.Code!=404{
  t.Fatalf("foreign tenant revoked controller: %d",w.Code)
 }
 if w=request("DELETE",path+"/"+created.ID,ownerToken,"");w.Code!=200{
  t.Fatalf("owner failed to revoke controller: %d",w.Code)
 }
 if w=request("GET",path,ownerToken,"");w.Code!=200||strings.Contains(w.Body.String(),created.ID){
  t.Fatal("removed controller still listed")
 }
}
