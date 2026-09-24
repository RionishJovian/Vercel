package database

import (
	"database/sql"
	"fmt"
	"time"

	"mega-magic/internal/model"
)

// PeerHeartbeatRepository handles peer heartbeat database operations
type PeerHeartbeatRepository struct {
	db *DB
}

// NewPeerHeartbeatRepository creates a new peer heartbeat repository
func NewPeerHeartbeatRepository(db *DB) *PeerHeartbeatRepository {
	return &PeerHeartbeatRepository{db: db}
}

// Insert saves a new peer heartbeat record
func (r *PeerHeartbeatRepository) Insert(h *model.PeerHeartbeat) (*model.PeerHeartbeat, error) {
	query := `
		INSERT INTO peer_heartbeats (id, business_id, peer_id, device_id, handshake_at, state, rx_bytes, tx_bytes, observed_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, business_id, peer_id, device_id, handshake_at, state, rx_bytes, tx_bytes, observed_at`

	out := &model.PeerHeartbeat{}
	var deviceID, handshakeAt sql.NullString

	err := r.db.QueryRow(query,
		h.ID, h.BusinessID, h.PeerID, h.DeviceID, h.HandshakeAt,
		h.State, h.RXBytes, h.TXBytes, h.ObservedAt,
	).Scan(
		&out.ID, &out.BusinessID, &out.PeerID, &deviceID, &handshakeAt,
		&out.State, &out.RXBytes, &out.TXBytes, &out.ObservedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert peer heartbeat: %w", err)
	}

	if deviceID.Valid {
		out.DeviceID = &deviceID.String
	}
	if handshakeAt.Valid {
		t, err := time.Parse(time.RFC3339Nano, handshakeAt.String)
		if err == nil {
			out.HandshakeAt = &t
		}
	}

	return out, nil
}

// ListByPeer returns heartbeats for a specific peer within a time range
func (r *PeerHeartbeatRepository) ListByPeer(businessID, peerID string, since, until time.Time) ([]model.PeerHeartbeat, error) {
	query := `
		SELECT id, business_id, peer_id, device_id, handshake_at, state, rx_bytes, tx_bytes, observed_at
		FROM peer_heartbeats
		WHERE business_id = $1 AND peer_id = $2 AND observed_at BETWEEN $3 AND $4
		ORDER BY observed_at DESC`

	rows, err := r.db.Query(query, businessID, peerID, since, until)
	if err != nil {
		return nil, fmt.Errorf("list peer heartbeats by peer: %w", err)
	}
	defer rows.Close()

	var heartbeats []model.PeerHeartbeat
	for rows.Next() {
		var h model.PeerHeartbeat
		var deviceID, handshakeAt sql.NullString
		err := rows.Scan(
			&h.ID, &h.BusinessID, &h.PeerID, &deviceID, &handshakeAt,
			&h.State, &h.RXBytes, &h.TXBytes, &h.ObservedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan peer heartbeat: %w", err)
		}
		if deviceID.Valid {
			h.DeviceID = &deviceID.String
		}
		if handshakeAt.Valid {
			t, err := time.Parse(time.RFC3339Nano, handshakeAt.String)
			if err == nil {
				h.HandshakeAt = &t
			}
		}
		heartbeats = append(heartbeats, h)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration: %w", err)
	}

	return heartbeats, nil
}

// ListByBusiness returns heartbeats for a business within a time range
func (r *PeerHeartbeatRepository) ListByBusiness(businessID string, since, until time.Time) ([]model.PeerHeartbeat, error) {
	query := `
		SELECT id, business_id, peer_id, device_id, handshake_at, state, rx_bytes, tx_bytes, observed_at
		FROM peer_heartbeats
		WHERE business_id = $1 AND observed_at BETWEEN $2 AND $3
		ORDER BY observed_at DESC`

	rows, err := r.db.Query(query, businessID, since, until)
	if err != nil {
		return nil, fmt.Errorf("list peer heartbeats by business: %w", err)
	}
	defer rows.Close()

	var heartbeats []model.PeerHeartbeat
	for rows.Next() {
		var h model.PeerHeartbeat
		var deviceID, handshakeAt sql.NullString
		err := rows.Scan(
			&h.ID, &h.BusinessID, &h.PeerID, &deviceID, &handshakeAt,
			&h.State, &h.RXBytes, &h.TXBytes, &h.ObservedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan peer heartbeat: %w", err)
		}
		if deviceID.Valid {
			h.DeviceID = &deviceID.String
		}
		if handshakeAt.Valid {
			t, err := time.Parse(time.RFC3339Nano, handshakeAt.String)
			if err == nil {
				h.HandshakeAt = &t
			}
		}
		heartbeats = append(heartbeats, h)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration: %w", err)
	}

	return heartbeats, nil
}

// GetLatestByPeer returns the most recent heartbeat for a peer
func (r *PeerHeartbeatRepository) GetLatestByPeer(businessID, peerID string) (*model.PeerHeartbeat, error) {
	query := `
		SELECT id, business_id, peer_id, device_id, handshake_at, state, rx_bytes, tx_bytes, observed_at
		FROM peer_heartbeats
		WHERE business_id = $1 AND peer_id = $2
		ORDER BY observed_at DESC
		LIMIT 1`

	h := &model.PeerHeartbeat{}
	var deviceID, handshakeAt sql.NullString

	err := r.db.QueryRow(query, businessID, peerID).Scan(
		&h.ID, &h.BusinessID, &h.PeerID, &deviceID, &handshakeAt,
		&h.State, &h.RXBytes, &h.TXBytes, &h.ObservedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get latest peer heartbeat: %w", err)
	}

	if deviceID.Valid {
		h.DeviceID = &deviceID.String
	}
	if handshakeAt.Valid {
		t, err := time.Parse(time.RFC3339Nano, handshakeAt.String)
		if err == nil {
			h.HandshakeAt = &t
		}
	}

	return h, nil
}