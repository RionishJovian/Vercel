package server

import (
	"context"
	"database/sql"
	"mega-magic/internal/database"
	"mega-magic/internal/telegram"
)

type telegramBindings struct {
	db            *database.DB
	repo          *database.BusinessTelegramBotRepository
	reservedToken string
}

func (s telegramBindings) Get(ctx context.Context, id string) (telegram.Binding, error) {
	bot, err := s.repo.GetByBusinessContext(ctx, id)
	if err != nil {
		return telegram.Binding{}, err
	}
	if bot == nil {
		return telegram.Binding{}, sql.ErrNoRows
	}
	b := telegram.Binding{BusinessID: bot.BusinessID, Token: bot.BotToken, ChatID: bot.TargetChatID, ApproverID: bot.ApproverUserID, Enabled: bot.Enabled}
	b.Reserved = s.reservedToken != "" && b.Token == s.reservedToken
	var active bool
	err = s.db.QueryRowContext(ctx, `SELECT is_active FROM businesses WHERE business_id=$1`, id).Scan(&active)
	if err != nil {
		return b, err
	}
	b.Enabled = b.Enabled && active
	var count int
	err = s.db.QueryRowContext(ctx, `SELECT count(*),COALESCE(min(user_id::text),'') FROM users WHERE business_id=$1 AND user_role='DEVELOPER' AND is_active`, id).Scan(&count, &b.ActorID)
	b.Platform = count > 0
	if count != 1 {
		b.ActorID = ""
	}
	return b, err
}
func (s telegramBindings) List(ctx context.Context) ([]telegram.Binding, error) {
	// Include disabled bindings so their status is explicit in the existing page.
	rows, err := s.db.QueryContext(ctx, `SELECT business_id FROM business_telegram_bots`)
	if err != nil {
		return nil, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	out := make([]telegram.Binding, 0, len(ids))
	for _, id := range ids {
		b, e := s.Get(ctx, id)
		if e != nil {
			return nil, e
		}
		out = append(out, b)
	}
	return out, nil
}
