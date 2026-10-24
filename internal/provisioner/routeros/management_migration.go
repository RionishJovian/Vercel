package routeros

import (
	"context"
	"crypto/sha256"
	"fmt"
	"mega-magic/internal/managementmigration"
	"net"
	"strings"
	"sync"
	"time"
)

type migrationTarget struct{ target *DeviceTarget }

func (s migrationTarget) TargetForDevice(context.Context, string, string) (*DeviceTarget, error) {
	return s.target, nil
}

type ManagementMigrationRouter struct {
	target  *DeviceTarget
	factory RunnerFactory
	allowed []string
	timeout time.Duration
}

func NewManagementMigrationRouter(ctx context.Context, source TargetSource, allowed []string, biz, device string, factory RunnerFactory) (*ManagementMigrationRouter, error) {
	policy, e := NewCIDRTargetSource(source, allowed)
	if e != nil {
		return nil, e
	}
	target, e := policy.TargetForDevice(ctx, biz, device)
	if e != nil {
		return nil, e
	}
	if target.DeviceID != device {
		return nil, fmt.Errorf("migration target mismatch")
	}
	if _, e = SSHConfigFromTarget(target, 10*time.Second); e != nil {
		return nil, e
	}
	if factory == nil {
		factory = func(cfg SSHConfig) CommandRunner { return NewSSHTransport(cfg) }
	}
	return &ManagementMigrationRouter{target: target, factory: factory, allowed: allowed, timeout: 10 * time.Second}, nil
}
func (r *ManagementMigrationRouter) runner(ctx context.Context, j *managementmigration.Job, address string) (CommandRunner, error) {
	host, port, e := net.SplitHostPort(r.target.Addr)
	if e != nil || r.target.Fingerprint != j.Fingerprint || (host != j.OldAddress && host != j.NewAddress) {
		return nil, fmt.Errorf("saved connection changed; recovery must inspect it")
	}
	clone := *r.target
	clone.Addr = net.JoinHostPort(address, port)
	policy, e := NewCIDRTargetSource(migrationTarget{&clone}, r.allowed)
	if e != nil {
		return nil, e
	}
	checked, e := policy.TargetForDevice(ctx, j.BusinessID, j.DeviceID)
	if e != nil {
		return nil, e
	}
	cfg, e := SSHConfigFromTarget(checked, r.timeout)
	if e != nil {
		return nil, e
	}
	return r.factory(cfg), nil
}
func managementInterface(device string) (string, string) {
	sum := sha256.Sum256([]byte(device))
	name := fmt.Sprintf("mmh-%x", sum[:6])
	return name, "mega-magic:" + name
}
func (r *ManagementMigrationRouter) Inspect(ctx context.Context, j *managementmigration.Job) error {
	host, _, e := net.SplitHostPort(r.target.Addr)
	if e != nil || host != j.OldAddress {
		return fmt.Errorf("saved address differs from the Hub allocation")
	}
	j.Fingerprint = r.target.Fingerprint
	if _, e := r.runner(ctx, j, j.NewAddress); e != nil {
		return e
	}
	runner, e := r.runner(ctx, j, j.OldAddress)
	if e != nil {
		return e
	}
	addresses, e := readNetworkObjects(ctx, runner, "addresses")
	if e != nil {
		return e
	}
	iface, marker := managementInterface(j.DeviceID)
	matches := 0
	for _, a := range addresses {
		if a.Values["address"] == j.OldAddress+"/32" {
			if a.Values["interface"] != iface || a.Values["comment"] != marker || networkBool(a.Values["dynamic"]) != "no" || networkBool(a.Values["disabled"]) != "no" {
				return fmt.Errorf("management address is not the expected owned static Hub address")
			}
			j.OldRecord = a.ID
			j.Interface = iface
			matches++
		}
	}
	if matches != 1 {
		return fmt.Errorf("management address is absent or ambiguous")
	}
	return r.verifyIdentity(ctx, runner, j)
}
func (r *ManagementMigrationRouter) verifyIdentity(ctx context.Context, runner CommandRunner, j *managementmigration.Job) error {
	if j.Interface == "" || !gatewayInterfacePattern.MatchString(j.Interface) {
		return fmt.Errorf("invalid management interface")
	}
	out, e := runner.Run(ctx, `:put [/interface wireguard get [find where name=`+routerOSQuoted(j.Interface)+`] public-key]`)
	if e != nil || strings.TrimSpace(out) != j.RouterKey {
		return fmt.Errorf("router WireGuard identity differs from the Hub")
	}
	return nil
}
func (r *ManagementMigrationRouter) VerifyOld(ctx context.Context, j *managementmigration.Job) error {
	runner, e := r.runner(ctx, j, j.OldAddress)
	if e != nil {
		return e
	}
	if e = r.verifyIdentity(ctx, runner, j); e != nil {
		return e
	}
	_, marker := managementInterface(j.DeviceID)
	old, e := networkReadOne(ctx, runner, "addresses", j.OldRecord, map[string]string{"address": "", "interface": "", "comment": "", "disabled": "", "dynamic": ""})
	if e != nil || old["address"] != j.OldAddress+"/32" || old["interface"] != j.Interface || old["comment"] != marker || networkBool(old["disabled"]) != "no" || networkBool(old["dynamic"]) != "no" {
		return fmt.Errorf("original management address changed")
	}
	return nil
}
func migrationAlias(j *managementmigration.Job) networkStep {
	return networkStep{Key: "addresses", SelectorField: "comment", SelectorValue: "mm-migration:" + j.ID, Values: map[string]string{"address": j.NewAddress + "/32", "interface": j.Interface, "comment": "mm-migration:" + j.ID, "disabled": "no"}}
}
func (r *ManagementMigrationRouter) Stage(ctx context.Context, j *managementmigration.Job) error {
	runner, e := r.runner(ctx, j, j.OldAddress)
	if e != nil {
		return e
	}
	if e = r.VerifyOld(ctx, j); e != nil {
		return e
	}
	addresses, e := readNetworkObjects(ctx, runner, "addresses")
	if e != nil {
		return e
	}
	for _, a := range addresses {
		ip, _, err := net.ParseCIDR(a.Values["address"])
		if err != nil {
			return fmt.Errorf("router has an unreadable address")
		}
		if ip.Equal(net.ParseIP(j.NewAddress)) {
			return fmt.Errorf("new address is already configured")
		}
	}
	for _, query := range []string{"/ip address print count-only where address=" + routerOSQuoted(j.NewAddress+"/32"), "/ip address print count-only where comment=" + routerOSQuoted("mm-migration:"+j.ID)} {
		n, e := networkCount(ctx, runner, query)
		if e != nil || n != 0 {
			return fmt.Errorf("new management address or operation marker already exists")
		}
	}
	n, e := networkCount(ctx, runner, "/system script job print count-only")
	if e != nil || n != 1 {
		return fmt.Errorf("router has another operation in progress")
	}
	j.Backup = snapshotName(j.DeviceID+"-management", time.Now().UTC())
	for _, cmd := range []string{"/export file=" + j.Backup, "/system backup save name=" + j.Backup} {
		if _, e = runner.Run(ctx, cmd); e != nil {
			return fmt.Errorf("management backup failed")
		}
	}
	alias := migrationAlias(j)
	// Old is never removed by the timer. The marker is unique per operation, so
	// a late timer cannot remove a later migration's address.
	out, e := runner.Run(ctx, `:put [:execute { :delay 3m; /ip address remove [find where comment=`+routerOSQuoted(alias.SelectorValue)+` and address=`+routerOSQuoted(j.NewAddress+"/32")+` and interface=`+routerOSQuoted(j.Interface)+`] }]`)
	j.Guard = strings.TrimSpace(out)
	if e != nil || !networkIDPattern.MatchString(j.Guard) {
		return fmt.Errorf("router restoration timer unavailable")
	}
	if e = networkWrite(ctx, runner, alias, false); e != nil {
		return e
	}
	return networkVerify(ctx, runner, alias, false)
}
func (r *ManagementMigrationRouter) VerifyNew(ctx context.Context, j *managementmigration.Job) error {
	runner, e := r.runner(ctx, j, j.NewAddress)
	if e != nil {
		return e
	}
	if e = r.verifyIdentity(ctx, runner, j); e != nil {
		return e
	}
	rows, e := readNetworkObjects(ctx, runner, "addresses")
	if e != nil {
		return e
	}
	_, marker := managementInterface(j.DeviceID)
	matches := 0
	for _, a := range rows {
		if a.Values["address"] == j.NewAddress+"/32" {
			if a.Values["interface"] != j.Interface || networkBool(a.Values["disabled"]) != "no" || (a.Values["comment"] != "mm-migration:"+j.ID && a.Values["comment"] != marker) {
				return fmt.Errorf("new address identity changed")
			}
			matches++
		}
	}
	if matches != 1 {
		return fmt.Errorf("new management address is absent or ambiguous")
	}
	return nil
}
func (r *ManagementMigrationRouter) RetainNew(ctx context.Context, j *managementmigration.Job) error {
	if e := r.VerifyNew(ctx, j); e != nil {
		return e
	}
	if !networkIDPattern.MatchString(j.Guard) {
		return fmt.Errorf("restoration timer receipt missing")
	}
	runner, e := r.runner(ctx, j, j.NewAddress)
	if e != nil {
		return e
	}
	if _, e = runner.Run(ctx, "/system script job remove "+j.Guard); e != nil {
		return e
	}
	n, e := networkCount(ctx, runner, "/system script job print count-only where .id="+j.Guard)
	if e != nil || n != 0 {
		return fmt.Errorf("timer cancellation not verified")
	}
	return r.VerifyNew(ctx, j)
}
func (r *ManagementMigrationRouter) RemoveOld(ctx context.Context, j *managementmigration.Job) error {
	if !networkIDPattern.MatchString(j.OldRecord) {
		return fmt.Errorf("invalid original record receipt")
	}
	if e := r.VerifyNew(ctx, j); e != nil {
		return e
	}
	runner, e := r.runner(ctx, j, j.NewAddress)
	if e != nil {
		return e
	}
	_, marker := managementInterface(j.DeviceID)
	// Compare the original record's complete identity before removing it. Never
	// remove a different address if an external administrator reused the ID.
	rows, e := readNetworkObjects(ctx, runner, "addresses")
	if e != nil {
		return e
	}
	newID := ""
	for _, a := range rows {
		if a.ID == j.OldRecord && (a.Values["address"] != j.OldAddress+"/32" || a.Values["interface"] != j.Interface || a.Values["comment"] != marker) {
			return fmt.Errorf("original address record was replaced")
		}
		if a.Values["address"] == j.NewAddress+"/32" {
			newID = a.ID
		}
	}
	if _, e = runner.Run(ctx, `/ip address remove [find where .id=`+j.OldRecord+` and address=`+routerOSQuoted(j.OldAddress+"/32")+` and interface=`+routerOSQuoted(j.Interface)+` and comment=`+routerOSQuoted(marker)+`]`); e != nil {
		return e
	}
	n, e := networkCount(ctx, runner, "/ip address print count-only where address="+routerOSQuoted(j.OldAddress+"/32"))
	if e != nil || n != 0 {
		return fmt.Errorf("old address removal not verified")
	}
	if !networkIDPattern.MatchString(newID) {
		return fmt.Errorf("new address record missing")
	}
	step := networkStep{Key: "addresses", ID: newID, Values: map[string]string{"comment": marker}}
	if e = networkWrite(ctx, runner, step, false); e != nil {
		return e
	}
	return networkVerify(ctx, runner, step, false)
}
func (r *ManagementMigrationRouter) Rollback(ctx context.Context, j *managementmigration.Job) error {
	if e := r.VerifyOld(ctx, j); e != nil {
		return e
	}
	runner, e := r.runner(ctx, j, j.OldAddress)
	if e != nil {
		return e
	}
	alias := migrationAlias(j)
	if _, e = runner.Run(ctx, `/ip address remove [find where comment=`+routerOSQuoted(alias.SelectorValue)+` and address=`+routerOSQuoted(j.NewAddress+"/32")+` and interface=`+routerOSQuoted(j.Interface)+`]`); e != nil {
		return e
	}
	return networkVerify(ctx, runner, alias, true)
}

// LockManagementDevice shares exclusion with existing guarded network writes.
func LockManagementDevice(businessID, deviceID string) func() {
	lock, _ := gatewayLocks.LoadOrStore(businessID+":"+deviceID, &sync.Mutex{})
	mu := lock.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}
