package server

import (
 "errors"
 "strings"

 "github.com/gin-gonic/gin"
 "mega-magic/internal/auth"
 "mega-magic/internal/apoptimization/omada"
 "mega-magic/internal/secretbox"
 "mega-magic/internal/config"
 "mega-magic/internal/database"
 "mega-magic/internal/handler"
 "mega-magic/internal/model"
)

// Owner approvals are always inert until a separate verified vendor adapter
// and a one-time server-side consumption workflow are connected and accepted.
func registerAPOptimizationRoutes(r *gin.Engine,cfg *config.Config,db *database.DB,svc *auth.Service)error{
 if !cfg.APOptimization.Enabled {return nil}
 if db==nil||svc==nil{return errors.New("AP proposal ledger dependencies unavailable")}
 var ready bool
 err:=db.QueryRow(`SELECT EXISTS(
  SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
  WHERE n.nspname='public' AND c.relname='ap_radio_proposals' AND c.relkind='r'
   AND c.relrowsecurity AND c.relforcerowsecurity
   AND EXISTS(SELECT 1 FROM pg_trigger t WHERE t.tgrelid=c.oid
    AND t.tgname='ap_radio_proposal_guard' AND NOT t.tgisinternal AND t.tgenabled IN('O','A'))
   AND EXISTS(SELECT 1 FROM pg_policy p WHERE p.polrelid=c.oid
    AND p.polname='ap_radio_proposals_tenant' AND p.polcmd='*'
    AND p.polqual IS NOT NULL AND p.polwithcheck IS NOT NULL)
   AND has_table_privilege('authenticated',c.oid,'SELECT')
   AND has_table_privilege('authenticated',c.oid,'UPDATE')
 )`).Scan(&ready)
 if err!=nil||!ready{return errors.New("AP proposal forced-RLS migration/role grants are not ready")}
 h:=&handler.APOptimizationHandler{DB:db, OmadaInventoryEnabled:cfg.APOptimization.OmadaInventoryEnabled}
 group:=r.Group("/api/v1/ap-optimization",auth.CookieOrigin(),
  auth.Middleware(svc),auth.RequireActiveTenant(db),auth.RequireMinRole(model.RoleOwner))
 group.GET("/capabilities",h.Capabilities)
 group.GET("/proposals",h.List)
 group.POST("/proposals/:id/decision",h.Decide)
 if cfg.APOptimization.OmadaInventoryEnabled {
  var exists bool
  err:=db.QueryRow(`SELECT EXISTS(
   SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
   WHERE n.nspname='public' AND c.relname='ap_omada_connections' AND c.relkind='r'
    AND c.relrowsecurity AND c.relforcerowsecurity
    AND EXISTS(SELECT 1 FROM pg_policy p WHERE p.polrelid=c.oid
     AND p.polname='ap_omada_connections_tenant' AND p.polcmd='*'
     AND p.polqual IS NOT NULL AND p.polwithcheck IS NOT NULL)
    AND has_table_privilege('authenticated',c.oid,'SELECT')
    AND has_table_privilege('authenticated',c.oid,'INSERT')
    AND has_table_privilege('authenticated',c.oid,'DELETE')
  )`).Scan(&exists)
  if err!=nil||!exists{return errors.New("Omada connector migration or tenant RLS unavailable")}
  box,err:=secretbox.New(cfg.APOptimization.OmadaCredentialKey)
  if err!=nil{return errors.New("Omada connector encryption unavailable")}
  caller,err:=omada.RestrictedCaller(cfg.APOptimization.OmadaAllowedCIDRs)
  if err!=nil{return err}
  origins:=map[string]bool{}
  for _,rawOrigin:=range cfg.APOptimization.OmadaAllowedOrigins {
   origin:=strings.TrimSuffix(rawOrigin,"/")
   if origin=="" {return errors.New("Omada controller origin empty")}
   c,err:=omada.New(omada.Settings{BaseURL:origin,OmadaID:"controller",
    SiteID:"site",ClientID:"config-validation",ClientSecret:"config-validation"},
    caller)
   if err!=nil||c==nil{return errors.New("Omada controller origin invalid or unsafe")}
   origins[origin]=true
  }
  if len(origins)==0{return errors.New("Omada controller origin allowlist empty")}
  oh:=&handler.OmadaControllerHandler{DB:db,Box:box,Caller:caller,AllowedOrigins:origins}
  group.GET("/omada/origins",oh.Origins)
  group.GET("/omada",oh.List)
  group.POST("/omada",oh.Create)
  group.GET("/omada/:id/devices",oh.Inventory)
  group.DELETE("/omada/:id",oh.Delete)
 }
 // Deliberately NO radio Apply route. Model/firmware support and owner-
 // authorized reversible AP writes have not been hardware accepted.
 return nil
}
