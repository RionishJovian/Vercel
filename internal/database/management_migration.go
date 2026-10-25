package database

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"mega-magic/internal/managementmigration"
	"time"
)

type ManagementMigrationStore struct{ DB *DB }

const migrationColumns = `id::text,business_id::text,device_id::text,requested_by::text,host(old_address),host(new_address),interface_name,old_record,fingerprint,router_key,guard_job,backup_name,phase,revision,to_char(expires_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"')`

func scanManagementJob(row interface{ Scan(...any) error }) (*managementmigration.Job, error) {
	j := &managementmigration.Job{}
	e := row.Scan(&j.ID, &j.BusinessID, &j.DeviceID, &j.RequestedBy, &j.OldAddress, &j.NewAddress, &j.Interface, &j.OldRecord, &j.Fingerprint, &j.RouterKey, &j.Guard, &j.Backup, &j.Phase, &j.Revision, &j.ExpiresAt)
	return j, e
}
func (s ManagementMigrationStore) transaction(ctx context.Context, biz string, fn func(*sql.Tx) error) error {
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if e = WithTenant(tx, biz, func() error { return fn(tx) }); e != nil {
		return e
	}
	return tx.Commit()
}
func (s ManagementMigrationStore) Load(ctx context.Context, biz, device, id string) (*managementmigration.Job, error) {
	var j *managementmigration.Job
	e := s.transaction(ctx, biz, func(tx *sql.Tx) error {
		var e error
		j, e = scanManagementJob(tx.QueryRowContext(ctx, `SELECT `+migrationColumns+` FROM management_address_migrations WHERE business_id=$1 AND device_id=$2 AND id=$3`, biz, device, id))
		return e
	})
	return j, e
}
func (s ManagementMigrationStore) Current(ctx context.Context, biz, device string) (*managementmigration.Job, error) {
	var j *managementmigration.Job
	e := s.transaction(ctx, biz, func(tx *sql.Tx) error {
		var e error
		j, e = scanManagementJob(tx.QueryRowContext(ctx, `SELECT `+migrationColumns+` FROM management_address_migrations WHERE business_id=$1 AND device_id=$2 ORDER BY created_at DESC,id DESC LIMIT 1`, biz, device))
		return e
	})
	return j, e
}
func (s ManagementMigrationStore) Create(ctx context.Context, j *managementmigration.Job) error {
	return s.transaction(ctx, j.BusinessID, func(tx *sql.Tx) error {
		res, e := tx.ExecContext(ctx, `INSERT INTO management_address_migrations(id,business_id,device_id,requested_by,old_address,new_address,interface_name,old_record,fingerprint,router_key,phase,expires_at) SELECT $1,$2,$3,$4,$5::inet,$6::inet,$7,$8,$9,$10,'planned',$11::timestamptz FROM devices WHERE device_id=$3 AND business_id=$2 AND management_ip=$5::inet AND ssh_host_key_fingerprint=$9`, j.ID, j.BusinessID, j.DeviceID, j.RequestedBy, j.OldAddress, j.NewAddress, j.Interface, j.OldRecord, j.Fingerprint, j.RouterKey, j.ExpiresAt)
		if e != nil {
			return e
		}
		n, e := res.RowsAffected()
		if e != nil || n != 1 {
			return fmt.Errorf("saved connection changed")
		}
		return nil
	})
}
func (s ManagementMigrationStore) Save(ctx context.Context, j *managementmigration.Job, previous string) error {
	e := s.transaction(ctx, j.BusinessID, func(tx *sql.Tx) error {
		res, e := tx.ExecContext(ctx, `UPDATE management_address_migrations SET phase=$1,revision=revision+1,guard_job=$2,backup_name=$3,updated_at=now() WHERE id=$4 AND business_id=$5 AND device_id=$6 AND phase=$7 AND revision=$8`, j.Phase, j.Guard, j.Backup, j.ID, j.BusinessID, j.DeviceID, previous, j.Revision)
		if e != nil {
			return e
		}
		n, e := res.RowsAffected()
		if e != nil || n != 1 {
			return fmt.Errorf("migration changed; reload its durable state")
		}
		return nil
	})
	if e == nil {
		j.Revision++
	}
	return e
}
func (s ManagementMigrationStore) SwitchConnection(ctx context.Context, j *managementmigration.Job) error {
	e := s.transaction(ctx, j.BusinessID, func(tx *sql.Tx) error {
		// Lock the journal before target CAS. Both updates commit atomically.
		var phase string
		var rev int
		if e := tx.QueryRowContext(ctx, `SELECT phase,revision FROM management_address_migrations WHERE id=$1 AND business_id=$2 AND device_id=$3 FOR UPDATE`, j.ID, j.BusinessID, j.DeviceID).Scan(&phase, &rev); e != nil {
			return e
		}
		if phase != "router_retained" || rev != j.Revision {
			return fmt.Errorf("migration checkpoint changed")
		}
		if _, e := tx.ExecContext(ctx, `SELECT set_config('app.management_migration_id',$1,true)`, j.ID); e != nil {
			return e
		}
		res, e := tx.ExecContext(ctx, `UPDATE devices SET management_ip=$1::inet,updated_at=now() WHERE device_id=$2 AND business_id=$3 AND host(management_ip)=$4 AND ssh_host_key_fingerprint=$5`, j.NewAddress, j.DeviceID, j.BusinessID, j.OldAddress, j.Fingerprint)
		if e != nil {
			return e
		}
		n, e := res.RowsAffected()
		if e != nil || n != 1 {
			return fmt.Errorf("saved connection changed")
		}
		_, e = tx.ExecContext(ctx, `UPDATE management_address_migrations SET phase='connection_saved',revision=revision+1,updated_at=now() WHERE id=$1 AND business_id=$2`, j.ID, j.BusinessID)
		if e != nil {
			return e
		}
		var role string
		if e = tx.QueryRowContext(ctx, `SELECT user_role FROM users WHERE user_id=$1 AND business_id=$2`, j.RequestedBy, j.BusinessID).Scan(&role); e != nil {
			return e
		}
		return NewDeviceRepository(s.DB).RecordChangeTXN(tx, j.BusinessID, j.DeviceID, role, "router.management_address.migrated", "Pinned management connection verified; migration "+j.ID)
	})
	if e == nil {
		j.Phase = "connection_saved"
		j.Revision++
	}
	return e
}

// Lease is session-scoped, so process death releases it. Never hold a tenant
// transaction open across router traffic; every journal checkpoint is durable.
func (s ManagementMigrationStore) Lease(ctx context.Context, biz, device string) (func(), error) {
	conn, e := s.DB.Conn(ctx)
	if e != nil {
		return nil, e
	}
	var locked bool
	e = conn.QueryRowContext(ctx, `SELECT pg_try_advisory_lock(hashtextextended($1,0))`, "management-migration:"+biz+":"+device).Scan(&locked)
	if e != nil || !locked {
		conn.Close()
		return nil, fmt.Errorf("another migration is running")
	}
	return func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		var unlocked bool
		err := conn.QueryRowContext(cleanup, `SELECT pg_advisory_unlock(hashtextextended($1,0))`, "management-migration:"+biz+":"+device).Scan(&unlocked)
		if err != nil || !unlocked {
			_ = conn.Raw(func(any) error { return driver.ErrBadConn })
		}
		conn.Close()
	}, nil
}

// A partially applied release must fail before enabling migration operations.
func CheckManagementMigrationSchema(db *DB) error {
	var ready bool
	err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM pg_class WHERE oid=to_regclass('public.management_address_migrations') AND relrowsecurity AND relforcerowsecurity) AND EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid='devices'::regclass AND tgname='guard_hub_management_address_move' AND tgenabled='O') AND EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid='devices'::regclass AND tgname='guard_pending_management_device_delete' AND tgenabled='O') AND EXISTS(SELECT 1 FROM pg_indexes WHERE schemaname='public' AND indexname='management_migration_one_active') AND NOT EXISTS(SELECT * FROM (VALUES ('id','uuid'),('business_id','uuid'),('device_id','uuid'),('requested_by','uuid'),('old_address','inet'),('new_address','inet'),('interface_name','text'),('old_record','text'),('fingerprint','text'),('router_key','text'),('guard_job','text'),('backup_name','text'),('phase','text'),('revision','int4'),('expires_at','timestamptz'),('created_at','timestamptz'),('updated_at','timestamptz')) expected(name,kind) EXCEPT SELECT column_name,udt_name FROM information_schema.columns WHERE table_schema='public' AND table_name='management_address_migrations')`).Scan(&ready)
	if err != nil || !ready {
		return fmt.Errorf("management migration schema is not ready; apply its additive migration before enabling the feature")
	}
	return nil
}
