package omada

import (
 "context"
 "crypto/tls"
 "errors"
 "fmt"
 "net"
 "net/http"
 "net/netip"
 "strconv"
 "strings"
 "time"
)

// RestrictedCaller confines controller Open API traffic to an explicit list of
// management IP networks. The owner cannot expand that allowlist through an
// API request. DNS names are resolved and checked on EVERY actual dial, so
// rebinding to loopback, cloud metadata or a foreign network fails closed.
// A production deployment must configure specific CIDRs, never 0.0.0.0/0.
func RestrictedCaller(allowedCIDRs []string)(*http.Client,error){
 if len(allowedCIDRs)==0{return nil,errors.New("omada: explicit management egress allowlist required")}
 allowed:=make([]netip.Prefix,0,len(allowedCIDRs))
 for _,raw:=range allowedCIDRs {
  p,e:=netip.ParsePrefix(strings.TrimSpace(raw))
  if e!=nil||p.Bits()==0||p!=p.Masked(){return nil,errors.New("omada: invalid or broad management CIDR")}
  allowed=append(allowed,p)
 }
 dialer:=&net.Dialer{Timeout:5*time.Second,KeepAlive:30*time.Second}
 transport:=&http.Transport{
  Proxy:nil, // never let HTTP(S)_PROXY bypass the CIDR boundary
  TLSClientConfig:&tls.Config{MinVersion:tls.VersionTLS12},
  MaxIdleConns:10,MaxIdleConnsPerHost:2,IdleConnTimeout:30*time.Second,
 }
 transport.DialContext=func(ctx context.Context,network,addr string)(net.Conn,error){
  host,port,err:=net.SplitHostPort(addr)
  if err!=nil{return nil,errors.New("omada: malformed outbound destination")}
  pn,err:=strconv.Atoi(port)
  if err!=nil||pn<1||pn>65535{return nil,errors.New("omada: malformed outbound port")}
  var addrs []netip.Addr
  if ip,err:=netip.ParseAddr(host);err==nil {
   addrs=[]netip.Addr{ip}
  }else{
   answers,err:=net.DefaultResolver.LookupNetIP(ctx,"ip",host)
   if err!=nil{return nil,errors.New("omada: controller DNS unavailable")}
   addrs=answers
  }
  // Never select a disallowed answer, even if DNS returned both approved and
  // unapproved IPs. Only trusted address ranges can be reached.
  for _,ip:=range addrs {
   if !ip.IsValid()||ip.IsMulticast()||ip.IsUnspecified() {continue}
   for _,prefix:=range allowed {
    if prefix.Contains(ip){
     return dialer.DialContext(ctx,network,net.JoinHostPort(ip.String(),port))
    }
   }
  }
  return nil,fmt.Errorf("omada: outbound controller address not in approved management CIDRs")
 }
 return &http.Client{
  Timeout:10*time.Second,Transport:transport,
  CheckRedirect:func(*http.Request,[]*http.Request)error{return http.ErrUseLastResponse},
 },nil
}
