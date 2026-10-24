package handler

import (
	"database/sql"
	"mega-magic/internal/database"
	"os"
	"regexp"
	"testing"
	"time"
)

// Run the real deployment SQL in rollback-only disposable DB transactions.
func TestIndividualRolloutClockAndPreflight(t *testing.T) {
	db := harness_testdb(t)
	raw, e := os.ReadFile("../../deploy/activate-individual-passes.sql")
	if e != nil {
		t.Fatal(e)
	}
	script := regexp.MustCompile(`(?m)^(BEGIN|COMMIT);$`).ReplaceAllString(string(raw), "")
	begin := func() *sql.Tx {
		tx, e := db.Begin()
		if e != nil {
			t.Fatal(e)
		}
		if _, e = tx.Exec(`UPDATE businesses SET is_active=false; DELETE FROM app_account_pass_transitions; DELETE FROM app_account_pass_rollouts`); e != nil {
			t.Fatal(e)
		}
		return tx
	}
	makeClient := func(tx *sql.Tx, canonical bool) (string, string) {
		email := randHex(6) + "@example.test"
		var biz, user string
		if e := tx.QueryRow(`INSERT INTO businesses(company_name,owner_email,access_tier)VALUES('Rollout fixture',$1,'SMALL_WISP')RETURNING business_id`, email).Scan(&biz); e != nil {
			t.Fatal(e)
		}
		if !canonical {
			email = "other-" + email
		}
		if e := tx.QueryRow(`INSERT INTO users(business_id,email,full_name,password_hash,user_role)VALUES($1,$2,'Fixture','fixture','OWNER')RETURNING user_id`, biz, email).Scan(&user); e != nil {
			t.Fatal(e)
		}
		return biz, user
	}
	t.Run("ambiguous owner fails before arming", func(t *testing.T) {
		tx := begin()
		defer tx.Rollback()
		makeClient(tx, false)
		if _, e := tx.Exec(script); e == nil {
			t.Fatal("unreviewed client was enforced")
		}
	})
	t.Run("restart preserves deadline and new clients inherit no grace", func(t *testing.T) {
		tx := begin()
		defer tx.Rollback()
		biz, user := makeClient(tx, true)
		if _, e := tx.Exec(script); e != nil {
			t.Fatal(e)
		}
		var start, expiry time.Time
		if e := tx.QueryRow(`SELECT r.starts_at,t.expires_at FROM app_account_pass_rollouts r JOIN app_account_pass_transitions t ON t.rollout_key=r.rollout_key WHERE t.user_id=$1`, user).Scan(&start, &expiry); e != nil {
			t.Fatal(e)
		}
		if expiry.Sub(start) != 7*24*time.Hour {
			t.Fatal("not seven days", start, expiry)
		}
		if _, e := tx.Exec(`UPDATE app_account_pass_rollouts SET enforcement_enabled=false`); e != nil {
			t.Fatal(e)
		}
		if _, e := tx.Exec(`INSERT INTO users(business_id,email,full_name,password_hash,user_role,created_at)VALUES($1,$2,'New client','fixture','ADMIN',clock_timestamp()+interval '1 second')`, biz, randHex(6)+"@example.test"); e != nil {
			t.Fatal(e)
		}
		if _, e := tx.Exec(script); e != nil {
			t.Fatal(e)
		}
		var again time.Time
		var count int
		if e := tx.QueryRow(`SELECT expires_at FROM app_account_pass_transitions WHERE user_id=$1`, user).Scan(&again); e != nil || !again.Equal(expiry) {
			t.Fatal("restart extended deadline", again, e)
		}
		if e := tx.QueryRow(`SELECT count(*) FROM app_account_pass_transitions WHERE business_id=$1`, biz).Scan(&count); e != nil || count != 1 {
			t.Fatal("new login inherited transition", count, e)
		}
		var access bool
		if e := database.WithTenant(tx, biz, func() error {
			s, e := database.AccountPassStatusTXN(tx, biz, user, true, false, start.Add(time.Minute))
			access = s.Access && s.State == "transition"
			return e
		}); e != nil || !access {
			t.Fatal("owner lost transition", e)
		}
	})
}
