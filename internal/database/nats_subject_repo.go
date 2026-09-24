package database

import (
	"database/sql"
	"fmt"

	"mega-magic/internal/model"
)

type NATSSubjectRepository struct {
	db *DB
}

func NewNATSSubjectRepository(db *DB) *NATSSubjectRepository {
	return &NATSSubjectRepository{db: db}
}

func (r *NATSSubjectRepository) Register(subject *model.NATSSubject) (*model.NATSSubject, error) {
	query := `
		INSERT INTO nats_subjects (business_id, subject_prefix)
		VALUES ($1, $2)
		ON CONFLICT (business_id, subject_prefix) DO UPDATE
			SET created_at = NOW()
		RETURNING business_id, subject_prefix, created_at`

	s := &model.NATSSubject{}
	err := r.db.QueryRow(query, subject.BusinessID, subject.SubjectPrefix).Scan(
		&s.BusinessID, &s.SubjectPrefix, &s.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("register nats subject: %w", err)
	}
	return s, nil
}

func (r *NATSSubjectRepository) ListByBusiness(businessID string) ([]*model.NATSSubject, error) {
	query := `SELECT business_id, subject_prefix, created_at FROM nats_subjects WHERE business_id = $1 ORDER BY created_at DESC`
	rows, err := r.db.Query(query, businessID)
	if err != nil {
		return nil, fmt.Errorf("list nats subjects by business: %w", err)
	}
	defer rows.Close()

	return scanNATSSubjects(rows)
}

func (r *NATSSubjectRepository) GetBySubject(subjectPrefix string) (*model.NATSSubject, error) {
	query := `SELECT business_id, subject_prefix, created_at FROM nats_subjects WHERE subject_prefix = $1`
	s := &model.NATSSubject{}
	err := r.db.QueryRow(query, subjectPrefix).Scan(&s.BusinessID, &s.SubjectPrefix, &s.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get nats subject: %w", err)
	}
	return s, nil
}

func (r *NATSSubjectRepository) Unregister(businessID, subjectPrefix string) error {
	result, err := r.db.Exec("DELETE FROM nats_subjects WHERE business_id = $1 AND subject_prefix = $2", businessID, subjectPrefix)
	if err != nil {
		return fmt.Errorf("unregister nats subject: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("nats subject not found: %s/%s", businessID, subjectPrefix)
	}
	return nil
}

func scanNATSSubjects(rows *sql.Rows) ([]*model.NATSSubject, error) {
	var subjects []*model.NATSSubject
	for rows.Next() {
		s := &model.NATSSubject{}
		if err := rows.Scan(&s.BusinessID, &s.SubjectPrefix, &s.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan nats subject: %w", err)
		}
		subjects = append(subjects, s)
	}
	return subjects, rows.Err()
}