package omada

import (
 "context"
 "encoding/json"
 "fmt"
 "net/http"
 "net/http/httptest"
 "strings"
 "sync/atomic"
 "testing"
)

func TestReadOnlyInventoryUsesDocumentedOpenAPI(t *testing.T) {
 var count atomic.Int32
 ts:=httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  count.Add(1)
  w.Header().Set("Content-Type","application/json")
  switch r.URL.Path {
  case "/openapi/authorize/token":
   if r.Method!="POST" || r.URL.Query().Get("grant_type")!="client_credentials"{
    t.Errorf("bad auth request: %s %s",r.Method,r.URL.String())
   }
   if r.Header.Get("Authorization")!=""{t.Error("auth request leaked a token")}
   var input map[string]string
   if err:=json.NewDecoder(r.Body).Decode(&input);err!=nil{t.Error(err)}
   if input["client_secret"]!="private-test-secret" || input["omadacId"]!="controller"{
    t.Errorf("unexpected auth payload")
   }
   fmt.Fprint(w,`{"errorCode":0,"result":{"accessToken":"AT-valid-token"}}`)
  case "/openapi/v1/controller/sites/site/devices":
   if r.Method!="GET"||r.URL.Query().Get("page")!="1"||
     r.URL.Query().Get("pageSize")!="100"||r.Header.Get("Authorization")!="AccessToken=AT-valid-token"{
    t.Errorf("unexpected inventory request: %s %s",r.Method,r.URL.String())
   }
   fmt.Fprint(w,`{"errorCode":0,"result":{"data":[{"mac":"AA:BB:CC:DD:EE:01","name":"Lobby AP","model":"EAP650","firmwareVersion":"1.2.3","status":1,"type":"ap"}]}}`)
  default: t.Errorf("unapproved endpoint used: %s",r.URL.Path);w.WriteHeader(404)
  }
 }))
 defer ts.Close()
 c,err:=New(Settings{BaseURL:ts.URL,OmadaID:"controller",SiteID:"site",
  ClientID:"test-client",ClientSecret:"private-test-secret"},ts.Client())
 if err!=nil{t.Fatal(err)}
 inv,err:=c.ReadDeviceInventory(context.Background())
 if err!=nil{t.Fatal(err)}
 if count.Load()!=2 {t.Fatalf("expected token plus one read, got %d",count.Load())}
 if inv.RadioOptimizationSupported ||len(inv.Devices)!=1 ||inv.Devices[0].MAC!="AA-BB-CC-DD-EE-01"||inv.Devices[0].DeviceType!="ap" {
  t.Fatalf("inventory must be read-only with correct identity: %+v",inv)
 }
 if _,err=c.ReadAPRadios(context.Background(),"AA:BB:CC:DD:EE:01");err==nil{
  t.Fatal("unverified radio endpoint must fail closed")
 }
}
func TestOmadaControllerRefusesUnsafeBasesAndSecrets(t *testing.T){
 for _,base:=range []string{"http://controller.example","https://me:pass@controller.example",
  "https://controller.example/secret","https://controller.example?token=x","ftp://controller.example"}{
  _,err:=New(Settings{BaseURL:base,OmadaID:"controller",SiteID:"site",ClientID:"id",ClientSecret:"secret"},nil)
  if err==nil{t.Errorf("accepted unsafe base %q",base)}
 }
}
func TestOmadaRefusesRedirectAndMalformedInventory(t *testing.T){
 ts:=httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  if strings.Contains(r.URL.Path,"authorize"){
   fmt.Fprint(w,`{"errorCode":0,"result":{"accessToken":"AT-valid-token"}}`);return
  }
  w.Header().Set("Location","https://attacker.example/collect")
  w.WriteHeader(http.StatusFound)
 }))
 defer ts.Close()
 caller:=ts.Client()
 caller.CheckRedirect=func(*http.Request,[]*http.Request)error{return http.ErrUseLastResponse}
 c,err:=New(Settings{BaseURL:ts.URL,OmadaID:"controller",SiteID:"site",ClientID:"id",ClientSecret:"secret"},caller)
 if err!=nil{t.Fatal(err)}
 if _,err=c.ReadDeviceInventory(context.Background());err==nil{
  t.Fatal("redirected inventory must fail closed")
 }
}
