package database

import (
	"os"
	"testing"
)

// The disposable cluster uses trust authentication. These fixture passwords
// exercise keyword parsing, not authentication; none are real credentials.
func TestDB_ConnectionValuesCannotChangeSelectedDatabase(t *testing.T) {
	if os.Getenv("MM_TEST_TRUST_AUTH") != "true" {
		t.Skip("requires an explicitly selected disposable trust-authenticated PostgreSQL cluster")
	}
	requireHarness(t)
	for _, password := range []string{"", "fixture with spaces", `fixture'\ dbname=unintended_database`} {
		cfg := harnessCfg
		cfg.Password = password
		db, err := New(cfg)
		if err != nil {
			t.Fatal("fixture connection value was not preserved")
		}
		var selected string
		err = db.QueryRow("SELECT current_database()").Scan(&selected)
		db.Close()
		if err != nil || selected != cfg.Name {
			t.Fatal("connection value changed database selection")
		}
	}
}
