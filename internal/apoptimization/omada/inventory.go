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

// ControllerInventory is controller-reported device inventory, never a verified RF scan.
type ControllerInventory struct {
 Source string `json:"source"`
 ObservedAt time.Time `json:"observed_at"`
 Devices []ControllerDevice `json:"devices"`
 RadioOptimizationSupported bool `json:"radio_optimization_supported"`
 Reason string `json:"reason"`
}
type ControllerDevice struct {
 MAC string `json:"mac"`
 Model string `json:"model,omitempty"`
 Name string `json:"name,omitempty"`
 Firmware string `json:"firmware,omitempty"`
 DeviceType string `json:"device_type,omitempty"`
 Status int `json:"status"`
}

// ReadControllerInventory reads only the documented Open API site devices list.
// It never asserts a physical connection, RF channel optimization capability,
// or device management authorization.
func(c *Client) ReadControllerInventory(ctx context.Context)(ControllerInventory,error){
 if c==nil || c.root==nil {return ControllerInventory{},ErrUnconfigured}
 token,err:=c.postToken(ctx)
 if err!=nil{return ControllerInventory{},err}
 const pageSize=100
 const maxPages=10
 out:=ControllerInventory{Source:"tp-link-omada-openapi-v1-devices",
  ObservedAt:time.Now().UTC(),Devices:[]ControllerDevice{},RadioOptimizationSupported:false,
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
  if err!=nil{return ControllerInventory{},err}
  result,err:=checkEnvelope(payload)
  if err!=nil{return ControllerInventory{},err}
  var body struct{
   Data []struct{
    MAC string `json:"mac"`
    Name string `json:"name"`
    Model string `json:"model"`
    Firmware string `json:"firmwareVersion"`
    Status int `json:"status"`
    Type string `json:"type"`
   } `json:"data"`
  }
  if err=json.Unmarshal(result,&body);err!=nil || body.Data==nil || len(body.Data)>pageSize {
   return ControllerInventory{},ErrUnavailable
  }
  for _,item:=range body.Data {
   if !apMAC.MatchString(item.MAC){return ControllerInventory{},ErrUnavailable}
   mac:=strings.ToUpper(strings.ReplaceAll(item.MAC,":","-"))
   if seen[mac]{return ControllerInventory{},ErrUnavailable}
   seen[mac]=true
   out.Devices=append(out.Devices,ControllerDevice{MAC:mac,Name:item.Name,Model:item.Model,
    Firmware:item.Firmware,DeviceType:item.Type,Status:item.Status})
  }
  if len(body.Data)<pageSize{return out,nil}
 }
 return ControllerInventory{},errors.New("omada: truncated inventory exceeds 1000 devices")
}
