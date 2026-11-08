package handler

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"mega-magic/internal/auth"
	"mega-magic/internal/database"
	"mega-magic/internal/voucher"
)

// VIPWiFiProvisioner has its own explicit server seam. It never alters
// ordinary guest-voucher login profiles or accounting.
type VIPWiFiProvisioner interface {
	EnsureVIPWiFiCode(context.Context, string, string, VIPWiFiSpec) error
	RevokeVIPWiFiCode(context.Context, string, string, VIPWiFiSpec) error
}
type VIPWiFiSpec struct {
	ID string
	Code string
	MaxDevices int
}

type VIPWiFiHandler struct {
	DB *database.DB
	Router VIPWiFiProvisioner
}

type vipRecord struct {
	ID string
	DeviceID string
	RequestID string
	Code string
	MaxDevices int
	Status string
	LeaseToken sql.NullString
	LeaseUntil sql.NullTime
}
func readVIP(row interface{ Scan(...any) error }) (vipRecord, error) {
	var v vipRecord
	err := row.Scan(&v.ID,&v.DeviceID,&v.RequestID,&v.Code,&v.MaxDevices,&v.Status,&v.LeaseToken,&v.LeaseUntil)
	return v,err
}
const vipCols = "id::text,device_id::text,request_id::text,code,max_devices,status,lease_token,lease_until"

func (h *VIPWiFiHandler) ready() bool { return h != nil && h.DB != nil && h.Router != nil }

func vipOwner(c *gin.Context) (string,bool) {
	biz := auth.GetBusinessID(c)
	if biz == "" { c.JSON(http.StatusUnauthorized,gin.H{"error":"unauthorized"});return "",false }
	if auth.GetUserRole(c) != "OWNER" {
		c.JSON(http.StatusForbidden,gin.H{"error":"only the tenant owner may manage staff VIP Wi-Fi access"})
		return "",false
	}
	return biz,true
}

func (h *VIPWiFiHandler) List(c *gin.Context) {
	c.Header("Cache-Control","no-store")
	biz,ok := vipOwner(c);if !ok{return}
	if !h.ready(){c.JSON(503,gin.H{"error":"VIP Wi-Fi unavailable"});return}
	type summary struct {
		ID string `json:"id"`
		DeviceID string `json:"device_id"`
		MaxDevices int `json:"max_devices"`
		Status string `json:"status"`
		Code string `json:"code,omitempty"`
	}
	out:=make([]summary,0)
	err:=runInTenant(c,h.DB,biz,func(tx *sql.Tx)error{
		rows,err:=tx.Query(`SELECT id::text,device_id::text,max_devices,status,CASE WHEN status='ready' THEN code ELSE '' END
		  FROM vip_wifi_codes WHERE business_id=$1 ORDER BY created_at DESC LIMIT 100`,biz)
		if err!=nil{return err}
		defer rows.Close()
		for rows.Next(){var v summary;if err:=rows.Scan(&v.ID,&v.DeviceID,&v.MaxDevices,&v.Status,&v.Code);err!=nil{return err};out=append(out,v)}
		return rows.Err()
	})
	if err!=nil{c.JSON(503,gin.H{"error":"VIP Wi-Fi records unavailable"});return}
	c.JSON(200,gin.H{"codes":out,"max_devices_allowed":8,"unlimited_time_and_data":true})
}

func (h *VIPWiFiHandler) Create(c *gin.Context) {
	c.Header("Cache-Control","no-store")
	biz,ok:=vipOwner(c);if !ok{return}
	if !h.ready(){c.JSON(503,gin.H{"error":"VIP Wi-Fi unavailable"});return}
	var body struct {
		RequestID string `json:"request_id"`
		DeviceID string `json:"device_id"`
		MaxDevices int `json:"max_devices"`
		ConfirmUnlimited bool `json:"confirm_unlimited"`
	}
	if c.ShouldBindJSON(&body)!=nil || !guestVoucherUUID.MatchString(body.RequestID) ||
		!guestVoucherUUID.MatchString(body.DeviceID) || body.MaxDevices<2 || body.MaxDevices>8 || !body.ConfirmUnlimited {
		c.JSON(400,gin.H{"error":"valid request, gateway, device limit (2–8), and unlimited-access confirmation required"});return
	}
	body.RequestID=strings.ToLower(body.RequestID)
	body.DeviceID=strings.ToLower(body.DeviceID)
	newCode,err:=voucher.GenerateGuestCode(rand.Reader)
	if err!=nil{c.JSON(503,gin.H{"error":"secure code generation unavailable"});return}
	code:="VIP-"+newCode
	var rec vipRecord
	err=runInTenant(c,h.DB,biz,func(tx *sql.Tx)error{
		var exists bool
		if e:=tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM devices WHERE business_id=$1 AND device_id=$2 AND deployment_layer='MAIN_GATEWAY')`,biz,body.DeviceID).Scan(&exists);e!=nil{return e}
		if !exists{return sql.ErrNoRows}
		_,e:=tx.Exec(`INSERT INTO vip_wifi_codes(business_id,request_id,device_id,code,max_devices,created_by)
		 VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(business_id,request_id) DO NOTHING`,
			biz,body.RequestID,body.DeviceID,code,body.MaxDevices,auth.GetUserID(c))
		if e!=nil{return e}
		rec,e=readVIP(tx.QueryRow(`SELECT `+vipCols+` FROM vip_wifi_codes WHERE business_id=$1 AND request_id=$2 FOR UPDATE`,biz,body.RequestID))
		if e!=nil{return e}
		if rec.DeviceID!=body.DeviceID || rec.MaxDevices!=body.MaxDevices{return errGuestVoucherBatchMismatch}
		return nil
	})
	if errors.Is(err,sql.ErrNoRows){c.JSON(404,gin.H{"error":"gateway unavailable"});return}
	if errors.Is(err,errGuestVoucherBatchMismatch){c.JSON(409,gin.H{"error":"request ID already belongs to different VIP settings"});return}
	if err!=nil{c.JSON(503,gin.H{"error":"VIP request could not be persisted"});return}
	h.install(c,biz,rec.ID)
}

func (h *VIPWiFiHandler) Retry(c *gin.Context){
	c.Header("Cache-Control","no-store")
	biz,ok:=vipOwner(c);if !ok{return}
	if !h.ready(){c.JSON(503,gin.H{"error":"VIP Wi-Fi unavailable"});return}
	if !guestVoucherUUID.MatchString(c.Param("id")){c.JSON(400,gin.H{"error":"invalid VIP identity"});return}
	h.install(c,biz,c.Param("id"))
}

// install holds an immutable identity + bounded lease. Router I/O happens only
// after committing ownership; network uncertainty retains the same code.
func (h *VIPWiFiHandler) install(c *gin.Context,biz,id string){
	var tok [16]byte
	if _,err:=rand.Read(tok[:]);err!=nil{c.JSON(503,gin.H{"error":"secure lease unavailable"});return}
	token:=hex.EncodeToString(tok[:])
	var rec vipRecord
	claimed:=false
	err:=runInTenant(c,h.DB,biz,func(tx *sql.Tx)error{
		var e error
		rec,e=readVIP(tx.QueryRow(`SELECT `+vipCols+` FROM vip_wifi_codes WHERE business_id=$1 AND id=$2 FOR UPDATE`,biz,id))
		if e!=nil{return e}
		if rec.Status!="pending"{return nil}
		if rec.LeaseUntil.Valid && time.Now().Before(rec.LeaseUntil.Time){return nil}
		res,e:=tx.Exec(`UPDATE vip_wifi_codes SET lease_token=$3,lease_until=clock_timestamp()+interval '2 minutes',
		  updated_at=clock_timestamp() WHERE business_id=$1 AND id=$2 AND status='pending'`,biz,id,token)
		if e!=nil{return e}
		n,e:=res.RowsAffected();claimed=n==1;return e
	})
	if errors.Is(err,sql.ErrNoRows){c.JSON(404,gin.H{"error":"VIP code not found"});return}
	if err!=nil{c.JSON(503,gin.H{"error":"VIP installation state unavailable"});return}
	if rec.Status=="ready"{c.JSON(200,gin.H{"id":rec.ID,"state":"ready","code":rec.Code,"max_devices":rec.MaxDevices});return}
	if rec.Status=="revoked" || rec.Status=="revoking"{c.JSON(409,gin.H{"error":"VIP code is being retired or already revoked"});return}
	if !claimed{c.JSON(202,gin.H{"id":rec.ID,"state":"pending","message":"installation already in progress; retry this identity"});return}
	ctx,cancel:=context.WithTimeout(context.WithoutCancel(c.Request.Context()),45*time.Second)
	defer cancel()
	installErr:=h.Router.EnsureVIPWiFiCode(ctx,biz,rec.DeviceID,VIPWiFiSpec{ID:rec.ID,Code:rec.Code,MaxDevices:rec.MaxDevices})
	// Never mark ready on an uncertain SSH outcome.
	finishCtx,stop:=context.WithTimeout(context.Background(),5*time.Second);defer stop()
	tx,e:=h.DB.BeginTx(finishCtx,nil)
	if e!=nil{c.JSON(202,gin.H{"id":rec.ID,"state":"pending"});return}
	defer tx.Rollback()
	e=database.WithTenant(tx,biz,func()error{
		next:="pending";if installErr==nil{next="ready"}
		res,e:=tx.Exec(`UPDATE vip_wifi_codes SET status=$4,lease_token=NULL,lease_until=NULL,
		  updated_at=clock_timestamp() WHERE business_id=$1 AND id=$2 AND lease_token=$3
		  AND status='pending'`,biz,rec.ID,token,next)
		if e!=nil{return e}
		n,e:=res.RowsAffected();if e!=nil{return e};if n!=1{return errors.New("stale VIP lease")};return nil
	})
	if e==nil{e=tx.Commit()}
	if e!=nil || installErr!=nil{c.JSON(202,gin.H{"id":rec.ID,"state":"pending","message":"router confirmation incomplete; retry this identity"});return}
	c.JSON(201,gin.H{"id":rec.ID,"state":"ready","code":rec.Code,"max_devices":rec.MaxDevices,"unlimited_time_and_data":true})
}

func (h *VIPWiFiHandler) Revoke(c *gin.Context){
	c.Header("Cache-Control","no-store")
	biz,ok:=vipOwner(c);if !ok{return}
	if !h.ready(){c.JSON(503,gin.H{"error":"VIP Wi-Fi unavailable"});return}
	id:=c.Param("id")
	if !guestVoucherUUID.MatchString(id){c.JSON(400,gin.H{"error":"invalid VIP identity"});return}
	var rec vipRecord
	err:=runInTenant(c,h.DB,biz,func(tx *sql.Tx)error{
		var e error
		rec,e=readVIP(tx.QueryRow(`SELECT `+vipCols+` FROM vip_wifi_codes WHERE business_id=$1 AND id=$2 FOR UPDATE`,biz,id))
		if e!=nil{return e}
		if rec.Status=="revoked"{return nil}
		if rec.LeaseUntil.Valid && time.Now().Before(rec.LeaseUntil.Time){return errors.New("VIP operation in progress")}
		_,e=tx.Exec(`UPDATE vip_wifi_codes SET status='revoking',lease_token=NULL,lease_until=NULL,
		  updated_at=clock_timestamp() WHERE business_id=$1 AND id=$2`,biz,id)
		return e
	})
	if errors.Is(err,sql.ErrNoRows){c.JSON(404,gin.H{"error":"VIP identity not found"});return}
	if err!=nil{c.JSON(409,gin.H{"error":"VIP revoke cannot start while another operation is active"});return}
	if rec.Status=="revoked"{c.JSON(200,gin.H{"id":id,"state":"revoked"});return}
	ctx,cancel:=context.WithTimeout(context.WithoutCancel(c.Request.Context()),45*time.Second);defer cancel()
	if e:=h.Router.RevokeVIPWiFiCode(ctx,biz,rec.DeviceID,VIPWiFiSpec{ID:rec.ID,Code:rec.Code,MaxDevices:rec.MaxDevices});e!=nil{
		c.JSON(202,gin.H{"id":id,"state":"revoking","message":"router revoke not verified; retry the same operation"});return
	}
	finishCtx,stop:=context.WithTimeout(context.Background(),5*time.Second);defer stop()
	tx,e:=h.DB.BeginTx(finishCtx,nil)
	if e!=nil{c.JSON(202,gin.H{"id":id,"state":"revoking"});return}
	defer tx.Rollback()
	e=database.WithTenant(tx,biz,func()error{
		_,e:=tx.Exec(`UPDATE vip_wifi_codes SET status='revoked',updated_at=clock_timestamp()
		 WHERE business_id=$1 AND id=$2 AND status='revoking'`,biz,id);return e
	})
	if e==nil{e=tx.Commit()}
	if e!=nil{c.JSON(202,gin.H{"id":id,"state":"revoking"});return}
	c.JSON(200,gin.H{"id":id,"state":"revoked"})
}
