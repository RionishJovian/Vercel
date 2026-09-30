package database

import (
	"database/sql"
	"fmt"
	"time"

	"mega-magic/internal/model"
)

// GetUserProfileSettingsTXN returns an empty default when the user has not yet
// customized their profile. All calls are made inside the user's tenant tx.
func (r *UserRepository) GetUserProfileSettingsTXN(tx *sql.Tx, businessID, userID string) (*model.UserProfileSettings, error) {
	p := &model.UserProfileSettings{UserID: userID, BusinessID: businessID, Language: "en"}
	err := tx.QueryRow(`
		SELECT user_id, business_id, language, avatar_data IS NOT NULL,
		       cover_data IS NOT NULL, updated_at
		FROM user_profile_settings
		WHERE user_id=$1 AND business_id=$2`, userID, businessID).Scan(
		&p.UserID, &p.BusinessID, &p.Language, &p.HasAvatar, &p.HasCover, &p.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		p.UpdatedAt = time.Time{}
		return p, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get user profile settings: %w", err)
	}
	return p, nil
}

func (r *UserRepository) UpdateOwnProfileTXN(tx *sql.Tx, businessID, userID, fullName, language string) error {
	res, err := tx.Exec(`UPDATE users SET full_name=$3, updated_at=now()
		WHERE user_id=$1 AND business_id=$2 AND is_active=true`, userID, businessID, fullName)
	if err != nil {
		return fmt.Errorf("update own profile name: %w", err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return sql.ErrNoRows
	}
	_, err = tx.Exec(`INSERT INTO user_profile_settings (user_id, business_id, language)
		VALUES ($1,$2,$3)
		ON CONFLICT (user_id) DO UPDATE SET language=EXCLUDED.language, updated_at=now()
		WHERE user_profile_settings.business_id=EXCLUDED.business_id`, userID, businessID, language)
	if err != nil {
		return fmt.Errorf("update own profile settings: %w", err)
	}
	return nil
}

func (r *UserRepository) UpdateOwnPasswordTXN(tx *sql.Tx, businessID, userID, passwordHash string) error {
	res, err := tx.Exec(`UPDATE users SET password_hash=$3, updated_at=now()
		WHERE user_id=$1 AND business_id=$2 AND is_active=true`, userID, businessID, passwordHash)
	if err != nil {
		return fmt.Errorf("update own password: %w", err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *UserRepository) PutOwnProfileAssetTXN(tx *sql.Tx, businessID, userID, kind, mime string, data []byte) error {
	if _, err := tx.Exec(`INSERT INTO user_profile_settings (user_id, business_id)
		VALUES ($1,$2) ON CONFLICT (user_id) DO NOTHING`, userID, businessID); err != nil {
		return err
	}
	var query string
	switch kind {
	case "avatar":
		query = `UPDATE user_profile_settings SET avatar_data=$3, avatar_mime=$4, updated_at=now()
			WHERE user_id=$1 AND business_id=$2`
	case "cover":
		query = `UPDATE user_profile_settings SET cover_data=$3, cover_mime=$4, updated_at=now()
			WHERE user_id=$1 AND business_id=$2`
	default:
		return fmt.Errorf("unsupported profile asset")
	}
	_, err := tx.Exec(query, userID, businessID, data, mime)
	return err
}

func (r *UserRepository) GetOwnProfileAssetTXN(tx *sql.Tx, businessID, userID, kind string) ([]byte, string, error) {
	var data []byte
	var mime sql.NullString
	var query string
	switch kind {
	case "avatar":
		query = `SELECT avatar_data, avatar_mime FROM user_profile_settings WHERE user_id=$1 AND business_id=$2`
	case "cover":
		query = `SELECT cover_data, cover_mime FROM user_profile_settings WHERE user_id=$1 AND business_id=$2`
	default:
		return nil, "", fmt.Errorf("unsupported profile asset")
	}
	err := tx.QueryRow(query, userID, businessID).Scan(&data, &mime)
	if err == sql.ErrNoRows || data == nil {
		return nil, "", nil
	}
	if err != nil {
		return nil, "", err
	}
	return data, mime.String, nil
}

func (r *UserRepository) DeleteOwnProfileAssetTXN(tx *sql.Tx, businessID, userID, kind string) error {
	var query string
	switch kind {
	case "avatar":
		query = `UPDATE user_profile_settings SET avatar_data=NULL, avatar_mime=NULL, updated_at=now()
			WHERE user_id=$1 AND business_id=$2`
	case "cover":
		query = `UPDATE user_profile_settings SET cover_data=NULL, cover_mime=NULL, updated_at=now()
			WHERE user_id=$1 AND business_id=$2`
	default:
		return fmt.Errorf("unsupported profile asset")
	}
	_, err := tx.Exec(query, userID, businessID)
	return err
}
