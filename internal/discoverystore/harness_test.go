package discoverystore

// E2EHarness exposes the integration database to the external end-to-end test. ok is false without MM_TEST_DB_URL.
func E2EHarness() (s *Store, business string, ok bool) {
	if !setupOK {
		return nil, "", false
	}
	_, _ = pool.Exec(`DELETE FROM discovery_sessions`)
	return New(pool), mine, true
}
