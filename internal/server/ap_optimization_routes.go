package server

import (
 "errors"

 "github.com/gin-gonic/gin"
 "mega-magic/internal/auth"
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
 h:=&handler.APOptimizationHandler{DB:db}
 group:=r.Group("/api/v1/ap-optimization",auth.CookieOrigin(),
  auth.Middleware(svc),auth.RequireActiveTenant(db),auth.RequireMinRole(model.RoleOwner))
 group.GET("/capabilities",h.Capabilities)
 group.GET("/proposals",h.List)
 group.POST("/proposals/:id/decision",h.Decide)
 // Deliberately NO Create/Apply route. Firmware, vendor enrollment and
 // owner-authorized reversible AP writes have not been hardware accepted.
 return nil
}
