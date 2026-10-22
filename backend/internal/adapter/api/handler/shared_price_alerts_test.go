package handler

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/songhanxu/wiseinvest/internal/adapter/repository"
	"github.com/songhanxu/wiseinvest/internal/application/service"
	"github.com/songhanxu/wiseinvest/internal/domain/model"
)

type sharedAlertTestMailer struct{ calls int }

func (m *sharedAlertTestMailer) Configured() bool { return true }
func (m *sharedAlertTestMailer) Send(context.Context, string, string, string, string) error {
	m.calls++
	return nil
}

func TestSharedTargetsMigrateWithoutRearmingAndSendOnce(t *testing.T) {
	db := passwordTestDB(t)
	if err := db.AutoMigrate(&model.PriceAlert{}, &model.PriceAlertSettings{}); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	repo := repository.NewPriceAlertRepository(db)
	old := model.PriceAlert{UserID: 7, HoldingID: "watchlist:us_stock:MU", AccountID: "watchlist", Code: "MU", Currency: "USD", TargetPrice: 850, Direction: "below", Status: "armed", Revision: 2, UpdatedAt: time.Now().Add(-time.Hour)}
	latest := model.PriceAlert{UserID: 7, HoldingID: "bitget:spot:rMU", AccountID: "bitget", Code: "MU", Currency: "USDT", TargetPrice: 900, Direction: "below", Status: "sent", Revision: 3, UpdatedAt: time.Now()}
	foreign := old
	foreign.UserID = 8
	for _, row := range []*model.PriceAlert{&old, &latest, &foreign} {
		if err := db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	price := 950.0
	mailer := &sharedAlertTestMailer{}
	svc := &service.PriceAlertService{Store: repo, Mailer: mailer, Quotes: func(_ context.Context, user uint, source string) ([]service.AlertQuote, error) {
		if user != 7 {
			t.Fatal("queried foreign account")
		}
		holding, currency := "bitget:spot:rMU", "USDT"
		if source == "watchlist" {
			holding, currency = "watchlist:us_stock:MU", "USD"
		}
		return []service.AlertQuote{{HoldingID: holding, AssetKey: "us_stock:MU", Currency: currency, Price: price, Fresh: true}}, nil
	}}
	rows, err := svc.List(ctx, 7)
	if err != nil || len(rows) != 1 || rows[0].ID != latest.ID || rows[0].Status != "sent" || rows[0].Revision != 3 || rows[0].TargetPrice != 900 || rows[0].Currency != "USDT" {
		t.Fatalf("legacy target/state lost: %+v %v", rows, err)
	}
	other, _ := repo.List(ctx, 8)
	if len(other) != 1 || other[0].ID != foreign.ID || other[0].AssetKey != "" {
		t.Fatal("another user's settings were changed")
	}
	if err := svc.SaveEmail(ctx, 7, "test@example.invalid"); err != nil {
		t.Fatal(err)
	}
	row, err := svc.Save(ctx, 7, "watchlist", "watchlist:us_stock:MU", 1000, "auto")
	if err != nil || row.ID != latest.ID || row.Revision != 4 || row.Currency != "USD" || row.Direction != "above" {
		t.Fatalf("cross-page edit created a second target: %+v %v", row, err)
	}
	svc.Run(ctx)
	if mailer.calls != 0 {
		t.Fatal("early email")
	}
	price = 1001
	svc.Run(ctx)
	svc.Run(ctx)
	if mailer.calls != 1 {
		t.Fatalf("expected one email, got %d", mailer.calls)
	}
	rows, _ = svc.List(ctx, 7)
	if len(rows) != 1 || rows[0].Status != "sent" {
		t.Fatal("sent state not shared")
	}
	price = 1050
	row, err = svc.Save(ctx, 7, "bitget", "bitget:spot:rMU", 1100, "auto")
	if err != nil || row.ID != latest.ID || row.Currency != "USDT" || row.Revision != 5 {
		t.Fatal("account edit did not replace the shared target")
	}
	price = 1200
	svc.Run(ctx)
	svc.Run(ctx)
	if mailer.calls != 2 {
		t.Fatal("account target did not remain one-shot")
	}
}

func TestConcurrentSharedTargetSavesKeepOneRow(t *testing.T) {
	db := passwordTestDB(t)
	if err := db.AutoMigrate(&model.PriceAlert{}); err != nil {
		t.Fatal(err)
	}
	repo := repository.NewPriceAlertRepository(db)
	ctx := context.Background()
	var wg sync.WaitGroup
	errors := make(chan error, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			row := model.PriceAlert{UserID: 7, HoldingID: "watchlist:us_stock:MU", AccountID: "watchlist", AssetKey: "us_stock:MU", Currency: "USD", TargetPrice: 900 + float64(i), Direction: "above", Status: "armed", Revision: 1}
			if i%2 == 0 {
				row.HoldingID, row.AccountID = "bitget:spot:rMU", "bitget"
			}
			errors <- repo.Save(ctx, &row)
		}(i)
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	rows, err := repo.List(ctx, 7)
	if err != nil || len(rows) != 1 || rows[0].Revision != 10 {
		t.Fatalf("duplicate target after concurrent edits: %+v %v", rows, err)
	}
}
