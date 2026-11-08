package handler

import (
 "net/http"
 "net/http/httptest"
 "strings"
 "testing"

 "github.com/gin-gonic/gin"
 "mega-magic/internal/auth"
)

func TestAPApprovalOwnerOnly(t *testing.T){
 gin.SetMode(gin.TestMode)
 for _,tc:=range []struct{role,method,path,body string; want int}{
  {"VIEWER","GET","/capabilities","",http.StatusForbidden},
  {"ADMIN","GET","/proposals","",http.StatusForbidden},
  {"DEVELOPER","POST","/proposals/11111111-1111-4111-8111-111111111111/decision",`{"approve":true}`,http.StatusForbidden},
  {"OWNER","GET","/capabilities","",http.StatusOK},
  {"OWNER","GET","/proposals","",http.StatusServiceUnavailable},
  {"OWNER","POST","/proposals/nope/decision",`{"approve":true}`,http.StatusServiceUnavailable},
 }{
  t.Run(tc.role+tc.method+tc.path,func(t *testing.T){
   r:=gin.New()
   r.Use(func(c *gin.Context){
    c.Set(auth.ContextKeyBusinessID,"11111111-1111-4111-8111-111111111111")
    c.Set(auth.ContextKeyUserRole,tc.role)
    c.Set(auth.ContextKeyUserID,"22222222-2222-4222-8222-222222222222")
   })
   h:=&APOptimizationHandler{}
   r.GET("/capabilities",h.Capabilities)
   r.GET("/proposals",h.List)
   r.POST("/proposals/:id/decision",h.Decide)
   w:=httptest.NewRecorder()
   req:=httptest.NewRequest(tc.method,tc.path,strings.NewReader(tc.body))
   req.Header.Set("Content-Type","application/json")
   r.ServeHTTP(w,req)
   if w.Code!=tc.want {t.Fatalf("status %d want %d body %s",w.Code,tc.want,w.Body.String())}
   if tc.role=="OWNER"&&tc.path=="/capabilities" &&
    (!strings.Contains(w.Body.String(),`"live_apply":false`)||
     !strings.Contains(w.Body.String(),`"automatic_proposals":false`)){
    t.Fatal("unwired capabilities cannot imply live AP management")
   }
  })
 }
}
