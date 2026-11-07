package server

import (
	"context"
	"mega-magic/internal/database"
	"mega-magic/internal/model"
	"testing"
)

func TestTelegramScopeComesFromActiveDatabaseRole(t *testing.T) {
	db := subscriptionHTTPDatabase(t)
	repo := database.NewBusinessTelegramBotRepository(db)
	makeBinding := func(role string) (string, string) {
		var bid, uid string
		if e := db.QueryRow(`INSERT INTO businesses(company_name,owner_email)VALUES('Telegram scope fixture',$1)RETURNING business_id`, role+"@scope.fixture").Scan(&bid); e != nil {
			t.Fatal(e)
		}
		if e := db.QueryRow(`INSERT INTO users(business_id,email,full_name,password_hash,user_role)VALUES($1,$2,'Fixture','fixture',$3)RETURNING user_id`, bid, bid+"@scope.fixture", role).Scan(&uid); e != nil {
			t.Fatal(e)
		}
		if e := repo.Upsert(&model.BusinessTelegramBot{BusinessID: bid, BotToken: "fixture", TargetChatID: 123, ApproverUserID: 123, Enabled: true}); e != nil {
			t.Fatal(e)
		}
		return bid, uid
	}
	admin, actor := makeBinding("DEVELOPER")
	customer, _ := makeBinding("OWNER")
	source := telegramBindings{db: db, repo: repo}
	b, e := source.Get(context.Background(), admin)
	if e != nil || !b.Platform || b.ActorID != actor {
		t.Fatal("platform scope was not resolved from the active developer")
	}
	b, e = source.Get(context.Background(), customer)
	if e != nil || b.Platform || b.ActorID != "" {
		t.Fatal("customer received platform scope")
	}
	if _, e = db.Exec(`UPDATE users SET user_role='OWNER' WHERE user_id=$1`, actor); e != nil {
		t.Fatal(e)
	}
	b, e = source.Get(context.Background(), admin)
	if e != nil || b.Platform {
		t.Fatal("scope persisted after administrator role removal")
	}
	if _, e = db.Exec(`UPDATE businesses SET is_active=false WHERE business_id=$1`, customer); e != nil {
		t.Fatal(e)
	}
	b, e = source.Get(context.Background(), customer)
	if e != nil || b.Enabled {
		t.Fatal("inactive customer could retain bot access")
	}
}
