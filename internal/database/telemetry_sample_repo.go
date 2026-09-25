package database

import (
	"database/sql"
	"fmt"
	"time"

	"mega-magic/internal/model"
)

// TelemetrySampleRepository handles telemetry sample database operations
type TelemetrySampleRepository struct {
	db *DB
}

// NewTelemetrySampleRepository creates a new telemetry sample repository
func NewTelemetrySampleRepository(db *DB) *TelemetrySampleRepository {
	return &TelemetrySampleRepository{db: db}
}

// Insert creates a new telemetry sample
func (r *TelemetrySampleRepository) Insert(sample *model.TelemetrySample) (*model.TelemetrySample, error) {
	query := `
		INSERT INTO telemetry_samples (business_id, device_id, subject_kind, subject_id,
		                               cpu_pct, memory_pct, latency_ms, uptime_seconds,
		                               wan_state, connector_state, tunnel_state, observed_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING id, business_id, device_id, subject_kind, subject_id,
		          cpu_pct, memory_pct, latency_ms, uptime_seconds,
		          wan_state, connector_state, tunnel_state, observed_at`

	s := &model.TelemetrySample{}
	var deviceID sql.NullString
	var cpuPct, memPct sql.NullFloat64
	var latency sql.NullInt32
	var uptime sql.NullInt64
	var wanState, connState, tunState sql.NullString

	err := r.db.QueryRow(query,
		sample.BusinessID, sample.DeviceID, sample.SubjectKind, sample.SubjectID,
		sample.CPUPercent, sample.MemoryPercent, sample.LatencyMs, sample.UptimeSeconds,
		sample.WANState, sample.ConnectorState, sample.TunnelState, sample.ObservedAt,
	).Scan(
		&s.ID, &s.BusinessID, &deviceID, &s.SubjectKind, &s.SubjectID,
		&cpuPct, &memPct, &latency, &uptime,
		&wanState, &connState, &tunState, &s.ObservedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert telemetry sample: %w", err)
	}

	if deviceID.Valid {
		s.DeviceID = &deviceID.String
	}
	if cpuPct.Valid {
		s.CPUPercent = &cpuPct.Float64
	}
	if memPct.Valid {
		s.MemoryPercent = &memPct.Float64
	}
	if latency.Valid {
		v := int(latency.Int32)
		s.LatencyMs = &v
	}
	if uptime.Valid {
		s.UptimeSeconds = &uptime.Int64
	}
	if wanState.Valid {
		s.WANState = &wanState.String
	}
	if connState.Valid {
		s.ConnectorState = &connState.String
	}
	if tunState.Valid {
		s.TunnelState = &tunState.String
	}

	return s, nil
}

// ListByDevice returns telemetry samples for a device within an optional time range
func (r *TelemetrySampleRepository) ListByDevice(deviceID string, since, until *time.Time) ([]*model.TelemetrySample, error) {
	query := `
		SELECT id, business_id, device_id, subject_kind, subject_id,
		       cpu_pct, memory_pct, latency_ms, uptime_seconds,
		       wan_state, connector_state, tunnel_state, observed_at
		FROM telemetry_samples
		WHERE device_id = $1`
	args := []interface{}{deviceID}
	argIdx := 2

	if since != nil {
		query += fmt.Sprintf(" AND observed_at >= $%d", argIdx)
		args = append(args, *since)
		argIdx++
	}
	if until != nil {
		query += fmt.Sprintf(" AND observed_at <= $%d", argIdx)
		args = append(args, *until)
		argIdx++
	}
	query += " ORDER BY observed_at DESC"

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list telemetry by device: %w", err)
	}
	defer rows.Close()

	return scanTelemetrySamples(rows)
}

// ListByBusiness returns telemetry samples for a business within an optional time range
func (r *TelemetrySampleRepository) ListByBusiness(businessID string, since, until *time.Time) ([]*model.TelemetrySample, error) {
	query := `
		SELECT id, business_id, device_id, subject_kind, subject_id,
		       cpu_pct, memory_pct, latency_ms, uptime_seconds,
		       wan_state, connector_state, tunnel_state, observed_at
		FROM telemetry_samples
		WHERE business_id = $1`
	args := []interface{}{businessID}
	argIdx := 2

	if since != nil {
		query += fmt.Sprintf(" AND observed_at >= $%d", argIdx)
		args = append(args, *since)
		argIdx++
	}
	if until != nil {
		query += fmt.Sprintf(" AND observed_at <= $%d", argIdx)
		args = append(args, *until)
		argIdx++
	}
	query += " ORDER BY observed_at DESC"

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list telemetry by business: %w", err)
	}
	defer rows.Close()

	return scanTelemetrySamples(rows)
}

// GetLatestByDevice returns the most recent telemetry sample for a device
func (r *TelemetrySampleRepository) GetLatestByDevice(deviceID string) (*model.TelemetrySample, error) {
	query := `
		SELECT id, business_id, device_id, subject_kind, subject_id,
		       cpu_pct, memory_pct, latency_ms, uptime_seconds,
		       wan_state, connector_state, tunnel_state, observed_at
		FROM telemetry_samples
		WHERE device_id = $1
		ORDER BY observed_at DESC
		LIMIT 1`

	s := &model.TelemetrySample{}
	var deviceIDNull sql.NullString
	var cpuPct, memPct sql.NullFloat64
	var latency sql.NullInt32
	var uptime sql.NullInt64
	var wanState, connState, tunState sql.NullString

	err := r.db.QueryRow(query, deviceID).Scan(
		&s.ID, &s.BusinessID, &deviceIDNull, &s.SubjectKind, &s.SubjectID,
		&cpuPct, &memPct, &latency, &uptime,
		&wanState, &connState, &tunState, &s.ObservedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get latest telemetry by device: %w", err)
	}

	if deviceIDNull.Valid {
		s.DeviceID = &deviceIDNull.String
	}
	if cpuPct.Valid {
		s.CPUPercent = &cpuPct.Float64
	}
	if memPct.Valid {
		s.MemoryPercent = &memPct.Float64
	}
	if latency.Valid {
		v := int(latency.Int32)
		s.LatencyMs = &v
	}
	if uptime.Valid {
		s.UptimeSeconds = &uptime.Int64
	}
	if wanState.Valid {
		s.WANState = &wanState.String
	}
	if connState.Valid {
		s.ConnectorState = &connState.String
	}
	if tunState.Valid {
		s.TunnelState = &tunState.String
	}

	return s, nil
}

// scanTelemetrySamples scans all rows from a telemetry query
func scanTelemetrySamples(rows *sql.Rows) ([]*model.TelemetrySample, error) {
	var samples []*model.TelemetrySample
	for rows.Next() {
		s := &model.TelemetrySample{}
		var deviceID sql.NullString
		var cpuPct, memPct sql.NullFloat64
		var latency sql.NullInt32
		var uptime sql.NullInt64
		var wanState, connState, tunState sql.NullString

		err := rows.Scan(
			&s.ID, &s.BusinessID, &deviceID, &s.SubjectKind, &s.SubjectID,
			&cpuPct, &memPct, &latency, &uptime,
			&wanState, &connState, &tunState, &s.ObservedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan telemetry sample row: %w", err)
		}

		if deviceID.Valid {
			s.DeviceID = &deviceID.String
		}
		if cpuPct.Valid {
			s.CPUPercent = &cpuPct.Float64
		}
		if memPct.Valid {
			s.MemoryPercent = &memPct.Float64
		}
		if latency.Valid {
			v := int(latency.Int32)
			s.LatencyMs = &v
		}
		if uptime.Valid {
			s.UptimeSeconds = &uptime.Int64
		}
		if wanState.Valid {
			s.WANState = &wanState.String
		}
		if connState.Valid {
			s.ConnectorState = &connState.String
		}
		if tunState.Valid {
			s.TunnelState = &tunState.String
		}

		samples = append(samples, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate telemetry sample rows: %w", err)
	}

	return samples, nil
}

// ---------------------------------------------------------------------------
// Transaction-scoped variants (RLS enforcement rollout: telemetry). Same SQL on
// the caller's *sql.Tx so they can run under WithTenant. Bare versions stay for
// the later full rollout.
// ---------------------------------------------------------------------------

// InsertTXN is the transaction-scoped variant of Insert.
func (r *TelemetrySampleRepository) InsertTXN(tx *sql.Tx, sample *model.TelemetrySample) (*model.TelemetrySample, error) {
	query := `
		INSERT INTO telemetry_samples (business_id, device_id, subject_kind, subject_id,
		                               cpu_pct, memory_pct, latency_ms, uptime_seconds,
		                               wan_state, connector_state, tunnel_state, observed_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING id, business_id, device_id, subject_kind, subject_id,
		          cpu_pct, memory_pct, latency_ms, uptime_seconds,
		          wan_state, connector_state, tunnel_state, observed_at`

	s := &model.TelemetrySample{}
	var deviceID sql.NullString
	var cpuPct, memPct sql.NullFloat64
	var latency sql.NullInt32
	var uptime sql.NullInt64
	var wanState, connState, tunState sql.NullString

	err := tx.QueryRow(query,
		sample.BusinessID, sample.DeviceID, sample.SubjectKind, sample.SubjectID,
		sample.CPUPercent, sample.MemoryPercent, sample.LatencyMs, sample.UptimeSeconds,
		sample.WANState, sample.ConnectorState, sample.TunnelState, sample.ObservedAt,
	).Scan(
		&s.ID, &s.BusinessID, &deviceID, &s.SubjectKind, &s.SubjectID,
		&cpuPct, &memPct, &latency, &uptime,
		&wanState, &connState, &tunState, &s.ObservedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert telemetry sample: %w", err)
	}
	if deviceID.Valid {
		s.DeviceID = &deviceID.String
	}
	if cpuPct.Valid {
		s.CPUPercent = &cpuPct.Float64
	}
	if memPct.Valid {
		s.MemoryPercent = &memPct.Float64
	}
	if latency.Valid {
		v := int(latency.Int32)
		s.LatencyMs = &v
	}
	if uptime.Valid {
		s.UptimeSeconds = &uptime.Int64
	}
	if wanState.Valid {
		s.WANState = &wanState.String
	}
	if connState.Valid {
		s.ConnectorState = &connState.String
	}
	if tunState.Valid {
		s.TunnelState = &tunState.String
	}
	return s, nil
}

// ListByDeviceTXN is the transaction-scoped variant of ListByDevice.
func (r *TelemetrySampleRepository) ListByDeviceTXN(tx *sql.Tx, deviceID string, since, until *time.Time) ([]*model.TelemetrySample, error) {
	query := `
		SELECT id, business_id, device_id, subject_kind, subject_id,
		       cpu_pct, memory_pct, latency_ms, uptime_seconds,
		       wan_state, connector_state, tunnel_state, observed_at
		FROM telemetry_samples
		WHERE device_id = $1`
	args := []interface{}{deviceID}
	argIdx := 2
	if since != nil {
		query += fmt.Sprintf(" AND observed_at >= $%d", argIdx)
		args = append(args, *since)
		argIdx++
	}
	if until != nil {
		query += fmt.Sprintf(" AND observed_at <= $%d", argIdx)
		args = append(args, *until)
		argIdx++
	}
	query += " ORDER BY observed_at DESC"

	rows, err := tx.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list telemetry by device: %w", err)
	}
	defer rows.Close()
	return scanTelemetrySamples(rows)
}

// ListByBusinessTXN is the transaction-scoped variant of ListByBusiness.
func (r *TelemetrySampleRepository) ListByBusinessTXN(tx *sql.Tx, businessID string, since, until *time.Time) ([]*model.TelemetrySample, error) {
	query := `
		SELECT id, business_id, device_id, subject_kind, subject_id,
		       cpu_pct, memory_pct, latency_ms, uptime_seconds,
		       wan_state, connector_state, tunnel_state, observed_at
		FROM telemetry_samples
		WHERE business_id = $1`
	args := []interface{}{businessID}
	argIdx := 2
	if since != nil {
		query += fmt.Sprintf(" AND observed_at >= $%d", argIdx)
		args = append(args, *since)
		argIdx++
	}
	if until != nil {
		query += fmt.Sprintf(" AND observed_at <= $%d", argIdx)
		args = append(args, *until)
		argIdx++
	}
	query += " ORDER BY observed_at DESC"

	rows, err := tx.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list telemetry by business: %w", err)
	}
	defer rows.Close()
	return scanTelemetrySamples(rows)
}

// GetLatestByDeviceTXN is the transaction-scoped variant of GetLatestByDevice.
func (r *TelemetrySampleRepository) GetLatestByDeviceTXN(tx *sql.Tx, deviceID string) (*model.TelemetrySample, error) {
	query := `
		SELECT id, business_id, device_id, subject_kind, subject_id,
		       cpu_pct, memory_pct, latency_ms, uptime_seconds,
		       wan_state, connector_state, tunnel_state, observed_at
		FROM telemetry_samples
		WHERE device_id = $1
		ORDER BY observed_at DESC
		LIMIT 1`

	s := &model.TelemetrySample{}
	var deviceIDNull sql.NullString
	var cpuPct, memPct sql.NullFloat64
	var latency sql.NullInt32
	var uptime sql.NullInt64
	var wanState, connState, tunState sql.NullString

	err := tx.QueryRow(query, deviceID).Scan(
		&s.ID, &s.BusinessID, &deviceIDNull, &s.SubjectKind, &s.SubjectID,
		&cpuPct, &memPct, &latency, &uptime,
		&wanState, &connState, &tunState, &s.ObservedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get latest telemetry by device: %w", err)
	}
	if deviceIDNull.Valid {
		s.DeviceID = &deviceIDNull.String
	}
	if cpuPct.Valid {
		s.CPUPercent = &cpuPct.Float64
	}
	if memPct.Valid {
		s.MemoryPercent = &memPct.Float64
	}
	if latency.Valid {
		v := int(latency.Int32)
		s.LatencyMs = &v
	}
	if uptime.Valid {
		s.UptimeSeconds = &uptime.Int64
	}
	if wanState.Valid {
		s.WANState = &wanState.String
	}
	if connState.Valid {
		s.ConnectorState = &connState.String
	}
	if tunState.Valid {
		s.TunnelState = &tunState.String
	}
	return s, nil
}
