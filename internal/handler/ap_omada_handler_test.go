package handler

import (
 "net/http"
 "net/http/httptest"
 "strings"
 "testing"

 "github.com/gin-gonic/gin"
 "mega-magic/internal/auth"
)

func TestOmadaControllerOriginRejectsUnsafeURLs(t *testing.T){
 for _,input:=range []string{
  "http://omada.example.test", "https://user:secret@omada.example.test",
  "https://omada.example.test/private", "https://omada.example.test/?secret=x",
  "https://omada.example.test/#fragment", "https://", "ftp://omada.example.test",
 }{
  if got:=omadaOrigin(input);got!=""{t.Fatalf("unsafe origin accepted: %q -> %q",input,got)}
 }
 for _,input:=range []string{"https://omada.example.test","https://omada.example.test/","https://omada.example.test:8043/"}{
  if got:=omadaOrigin(input);got==""{t.Fatalf("valid controller origin rejected: %q",input)}
 }
}
func TestOmadaEnrollmentDeniedWithoutOwnerAndBackend(t *testing.T){
 gin.SetMode(gin.TestMode)
 for _,tc:=range []struct{role,method,path,body string;want int}{
  {"VIEWER","GET","/omada","",403},
  {"ADMIN","POST","/omada",`{"base_url":"https://omada.example.test"}`,403},
  {"DEVELOPER","DELETE","/omada/11111111-1111-4111-8111-111111111111","",403},
  {"OWNER","GET","/omada","",503},
  {"OWNER","GET","/omada/origins","",503},
  {"OWNER","POST","/omada",`{"base_url":"https://omada.example.test"}`,503},
 }{
  t.Run(tc.role+tc.method+tc.path,func(t *testing.T){
   h:=&OmadaControllerHandler{}
   r:=gin.New()
   r.Use(func(c *gin.Context){
    c.Set(auth.ContextKeyBusinessID,"11111111-1111-4111-8111-111111111111")
    c.Set(auth.ContextKeyUserRole,tc.role)
   })
   r.GET("/omada",h.List)
   r.GET("/omada/origins",h.Origins)
   r.POST("/omada",h.Create)
   r.DELETE("/omada/:id",h.Delete)
   w:=httptest.NewRecorder()
   req:=httptest.NewRequest(tc.method,tc.path,strings.NewReader(tc.body))
   req.Header.Set("Content-Type","application/json")
   r.ServeHTTP(w,req)
   if w.Code!=tc.want{t.Fatalf("%d want %d body %s",w.Code,tc.want,w.Body.String())}
   if strings.Contains(w.Body.String(),"client_secret"){t.Fatal("secret echoed in denial")}
  })
 }
}
func TestOmadaSecretsNeverEchoedForMissingOwner(t *testing.T){
 h:=&OmadaControllerHandler{}
 r:=gin.New()
 r.POST("/omada",h.Create)
 w:=httptest.NewRecorder()
 req:=httptest.NewRequest(http.MethodPost,"/omada",strings.NewReader(
  `{"client_secret":"sensitive-test-password-not-to-echo"}`))
 req.Header.Set("Content-Type","application/json")
 r.ServeHTTP(w,req)
 if strings.Contains(w.Body.String(),"sensitive-test-password-not-to-echo"){
  t.Fatal("Omada API secret leaked in error response")
 }
}
