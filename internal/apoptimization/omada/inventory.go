package omada

import (
 "context"
 "encoding/json"
 "errors"
 "fmt"
 "net/http"
 "net/url"
 "strings"
 "time"
)

// APInventory is controller-reported device inventory, never a verified RF scan.
type APInventory struct {
 Source string `json:"source"`
 ObservedAt time.Time `json:"observed_at"`
 Devices []APDevice `json:"devices"`
 RadioOptimizationSupported bool `json:"radio_optimization_supported"`
 Reason string `json:"reason"`
}
type APDevice struct {
 MAC string `json:"mac"`
 Model string `json:"model,omitempty"`
 Name string `json:"name,omitempty"`
 Firmware string `json:"firmware,omitempty"`
 Status int `json:"status"`
}

// ReadAPInventory reads only the documented Open API site devices list.
// It never asserts a physical connection, RF channel optimization capability,
// or device management authorization.
func(c *Client) ReadAPInventory(ctx context.Context)(APInventory,error){
 if c==nil || c.root==nil {return APInventory{},ErrUnconfigured}
 token,err:=c.postToken(ctx)
 if err!=nil{return APInventory{},err}
 const pageSize=100
 const maxPages=10
 out:=APInventory{Source:"tp-link-omada-openapi-v1-devices",
  ObservedAt:time.Now().UTC(),Devices:[]APDevice{},RadioOptimizationSupported:false,
  Reason:"No verified controller-specific RF observation or radio write API configured"}
 seen:=map[string]bool{}
 for page:=1;page<=maxPages;page++ {
  query:=url.Values{}
  query.Set("page",fmt.Sprint(page));query.Set("pageSize",fmt.Sprint(pageSize))
  endpoint:=c.root.ResolveReference(&url.URL{
   Path:fmt.Sprintf("/openapi/v1/%s/sites/%s/devices",c.settings.OmadaID,c.settings.SiteID),
   RawQuery:query.Encode(),
  })
  payload,err:=c.request(ctx,http.MethodGet,endpoint,nil,token)
  if err!=nil{return APInventory{},err}
  result,err:=checkEnvelope(payload)
  if err!=nil{return APInventory{},err}
  var body struct{
   Data []struct{
    MAC string `json:"mac"`
    Name string `json:"name"`
    Model string `json:"model"`
    Firmware string `json:"firmwareVersion"`
    Status int `json:"status"`
   } `json:"data"`
  }
  if err=json.Unmarshal(result,&body);err!=nil || body.Data==nil || len(body.Data)>pageSize {
   return APInventory{},ErrUnavailable
  }
  for _,item:=range body.Data {
   if !apMAC.MatchString(item.MAC){return APInventory{},ErrUnavailable}
   mac:=strings.ToUpper(strings.ReplaceAll(item.MAC,":","-"))
   if seen[mac]{return APInventory{},ErrUnavailable}
   seen[mac]=true
   out.Devices=append(out.Devices,APDevice{MAC:mac,Name:item.Name,Model:item.Model,
    Firmware:item.Firmware,Status:item.Status})
  }
  if len(body.Data)<pageSize{return out,nil}
 }
 return APInventory{},errors.New("omada: truncated inventory exceeds 1000 devices")
}
