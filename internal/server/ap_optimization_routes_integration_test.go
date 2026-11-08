package server

import (
 "context"
 "encoding/json"
 "net/http/httptest"
 "os"
 "strings"
 "testing"
 "time"

 "github.com/gin-gonic/gin"
 "mega-magic/internal/apoptimization"
 "mega-magic/internal/auth"
 "mega-magic/internal/config"
 "mega-magic/internal/model"
)

func TestAPApprovalOwnerOnlyAndTenantSafe(t *testing.T){
 db:=subscriptionHTTPDatabase(t)
 data,err:=os.ReadFile("../../db/migrations/20261108190000_ap_radio_owner_approval.sql")
 if err!=nil{t.Fatal(err)}
 if _,err=db.Exec(string(data));err!=nil{t.Fatal("AP ledger migration:",err)}
 svc,err:=auth.NewService("disposable-AP-approval-auth-fixture",time.Hour,time.Hour,4)
 if err!=nil{t.Fatal(err)}
 svc.UseSessions(db)
 makeUser:=func(email string,role model.UserRole)(*model.User,string){
  t.Helper()
  u:=&model.User{Email:email,UserRole:role,PasswordHash:"fixture"}
  if err=db.QueryRow(`INSERT INTO businesses(company_name,owner_email,access_tier,device_quota_limit)
   VALUES('AP fixture',$1,'SMALL_WISP',10) RETURNING business_id`,email).Scan(&u.BusinessID);err!=nil{t.Fatal(err)}
  if err=db.QueryRow(`INSERT INTO users(business_id,email,full_name,password_hash,user_role)
   VALUES($1,$2,'Fixture','fixture',$3) RETURNING user_id`,
   u.BusinessID,email,role).Scan(&u.UserID);err!=nil{t.Fatal(err)}
  token,_,err:=svc.StartSession(u)
  if err!=nil{t.Fatal(err)}
  return u,token
 }
 owner,ownerToken:=makeUser("ap-owner@example.test",model.RoleOwner)
 foreign,foreignToken:=makeUser("ap-foreign@example.test",model.RoleOwner)
 // Same tenant admin must not approve or reject the owner's AP plan.
 admin:=&model.User{BusinessID:owner.BusinessID,Email:"ap-admin@example.test",UserRole:model.RoleAdmin,PasswordHash:"fixture"}
 if err=db.QueryRow(`INSERT INTO users(business_id,email,full_name,password_hash,user_role)
   VALUES($1,$2,'Admin','fixture',$3) RETURNING user_id`,
   owner.BusinessID,admin.Email,admin.UserRole).Scan(&admin.UserID);err!=nil{t.Fatal(err)}
 adminToken,_,err:=svc.StartSession(admin)
 if err!=nil{t.Fatal(err)}
 var deviceID string
 if err=db.QueryRow(`INSERT INTO devices(business_id,hardware_name,transport,deployment_layer)
  VALUES($1,'Lab AP','direct','ACCESS_POINT') RETURNING device_id`,owner.BusinessID).Scan(&deviceID);err!=nil{t.Fatal(err)}
 now:=time.Now().UTC()
 proposal:=apoptimization.Proposal{
  TenantID:owner.BusinessID,DeviceID:deviceID,Vendor:"tplink",Model:"EAP650",Band:apoptimization.Band24,
  Mode:"manual",BeforeChannel:11,AfterChannel:6,BeforePowerDBm:17,AfterPowerDBm:17,
  Fingerprint:strings.Repeat("a",64),CreatedAt:now.Add(-5*time.Second),ExpiresAt:now.Add(4*time.Minute),
  ApprovalRequired:true,
 }
 id,err:=(&apoptimization.Ledger{DB:db}).RecordTrustedProposal(context.Background(),owner.BusinessID,proposal)
 if err!=nil{t.Fatal(err)}
 var savedHash string
 if err=db.QueryRow(`SELECT proposal_hash FROM ap_radio_proposals WHERE id=$1`,id).Scan(&savedHash);err!=nil{t.Fatal(err)}
 cfg:=&config.Config{}
 cfg.APOptimization.Enabled=true
 r:=gin.New()
 if err=registerAPOptimizationRoutes(r,cfg,db,svc);err!=nil{t.Fatal(err)}
 request:=func(method,path,token,body string) *httptest.ResponseRecorder{
  w:=httptest.NewRecorder()
  req:=httptest.NewRequest(method,"https://ap.test"+path,strings.NewReader(body))
  req.Header.Set("Authorization","Bearer "+token)
  req.Header.Set("Content-Type","application/json")
  req.Header.Set("Origin","https://ap.test")
  req.Header.Set("X-MM-CSRF","1")
  r.ServeHTTP(w,req)
  return w
 }
 path:="/api/v1/ap-optimization/proposals"
 if w:=request("GET",path,foreignToken,"");w.Code!=200 ||strings.Contains(w.Body.String(),id){
  t.Fatalf("foreign tenant saw proposal: %d %s",w.Code,w.Body.String())
 }
 if w:=request("GET",path,adminToken,"");w.Code!=403{t.Fatalf("admin viewed owner approval ledger: %d",w.Code)}
 if w:=request("GET",path,ownerToken,"");w.Code!=200||!strings.Contains(w.Body.String(),id){
  t.Fatalf("owner could not review plan: %d %s",w.Code,w.Body.String())
 }
 decision,_:=json.Marshal(map[string]any{"approve":true,"proposal_hash":savedHash,"fingerprint":proposal.Fingerprint})
 decisionPath:=path+"/"+id+"/decision"
 if w:=request("POST",decisionPath,adminToken,string(decision));w.Code!=403{
  t.Fatalf("admin approved radio change: %d",w.Code)
 }
 if w:=request("POST",decisionPath,foreignToken,string(decision));w.Code!=404{
  t.Fatalf("foreign tenant should not see proposal: %d %s",w.Code,w.Body.String())
 }
 bad,_:=json.Marshal(map[string]any{"approve":true,"proposal_hash":strings.Repeat("0",64),"fingerprint":proposal.Fingerprint})
 if w:=request("POST",decisionPath,ownerToken,string(bad));w.Code!=409{
  t.Fatalf("wrong reviewed digest accepted: %d",w.Code)
 }
 if w:=request("POST",decisionPath,ownerToken,string(decision));w.Code!=200{
  t.Fatalf("owner approval not recorded: %d %s",w.Code,w.Body.String())
 }
 if w:=request("POST",decisionPath,ownerToken,string(decision));w.Code!=409{
  t.Fatalf("replayed approval accepted: %d",w.Code)
 }
 var status string
 if err=db.QueryRow(`SELECT status FROM ap_radio_proposals WHERE id=$1`,id).Scan(&status);err!=nil||status!="approved"{
  t.Fatalf("wrong DB state %q %v",status,err)
 }
 if _,err=(&apoptimization.Ledger{DB:db}).ConsumeOwnerApproval(context.Background(),foreign.BusinessID,id,time.Now());err==nil{
  t.Fatal("foreign tenant consumed owner approval")
 }
 consumed,err:=(&apoptimization.Ledger{DB:db}).ConsumeOwnerApproval(context.Background(),owner.BusinessID,id,time.Now())
 if err!=nil||consumed.DeviceID!=deviceID{t.Fatalf("approved plan consumption failed: %v",err)}
 if _,err=(&apoptimization.Ledger{DB:db}).ConsumeOwnerApproval(context.Background(),owner.BusinessID,id,time.Now());err==nil{
  t.Fatal("used approval was replayed")
 }
}
