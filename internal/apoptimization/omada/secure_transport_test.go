package omada

import (
 "context"
 "crypto/x509"
 "io"
 "net/http"
 "net/http/httptest"
 "strings"
 "testing"
)

func TestOmadaEgressRejectsBroadAndUnconfiguredCIDRs(t *testing.T){
 for _,cidrs:=range [][]string{nil,{},{"0.0.0.0/0"},{"::/0"},{"invalid"},{"127.0.0.1/8"}}{
  if c,e:=RestrictedCaller(cidrs);e==nil||c!=nil{
   t.Fatalf("unbounded or malformed policy accepted: %v",cidrs)
  }
 }
}

func TestOmadaEgressRequiresAllowedResolvedIPAndTLS(t *testing.T){
 ts:=httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  _,_=io.WriteString(w,"ok")
 }))
 defer ts.Close()
 blocked,err:=RestrictedCaller([]string{"192.0.2.0/24"})
 if err!=nil{t.Fatal(err)}
 req,err:=http.NewRequestWithContext(context.Background(),"GET",ts.URL,nil)
 if err!=nil{t.Fatal(err)}
 if resp,err:=blocked.Do(req);err==nil{
  resp.Body.Close()
  t.Fatal("controller outside allowlist was dialed")
 }
 allowed,err:=RestrictedCaller([]string{"127.0.0.1/32"})
 if err!=nil{t.Fatal(err)}
 // The production client trusts only the system CA set; tests must explicitly
 // trust the disposable server's cert without weakening production TLS.
 pool:=x509.NewCertPool()
 pool.AddCert(ts.Certificate())
 tr:=allowed.Transport.(*http.Transport)
 tr.TLSClientConfig.RootCAs=pool
 resp,err:=allowed.Do(req)
 if err!=nil{t.Fatalf("allowlisted controller with trusted TLS failed: %v",err)}
 defer resp.Body.Close()
 if resp.StatusCode!=200{t.Fatalf("unexpected status %d",resp.StatusCode)}
 if tr.Proxy!=nil{t.Fatal("proxy could bypass pinned address policy")}
 if allowed.CheckRedirect==nil{t.Fatal("redirect could escape approved host")}
}

func TestOmadaEgressIgnoresEnvironmentProxy(t *testing.T){
 t.Setenv("HTTPS_PROXY","http://127.0.0.1:1")
 c,err:=RestrictedCaller([]string{"192.0.2.0/24"})
 if err!=nil{t.Fatal(err)}
 req,_:=http.NewRequest("GET","https://127.0.0.1:443/",nil)
 _,err=c.Do(req)
 if err==nil||!strings.Contains(err.Error(),"approved management CIDRs"){
  t.Fatalf("policy bypassed by proxy environment: %v",err)
 }
}
