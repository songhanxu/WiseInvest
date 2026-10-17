package database

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/songhanxu/wiseinvest/internal/adapter/repository"
	"github.com/songhanxu/wiseinvest/internal/domain/model"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestPriceAlertRepositoryClaimsRevisionAndUserIsolation(t *testing.T) {
	dsn := os.Getenv("WISEINVEST_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set WISEINVEST_TEST_POSTGRES_DSN for isolated PostgreSQL integration test")
	}
	admin, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal("test database unavailable")
	}
	schema := fmt.Sprintf("price_alert_test_%d", time.Now().UnixNano())
	if admin.Exec("CREATE SCHEMA "+schema).Error != nil {
		t.Fatal("cannot create isolated test schema")
	}
	sqlAdmin, _ := admin.DB()
	defer func() { admin.Exec("DROP SCHEMA " + schema + " CASCADE"); sqlAdmin.Close() }()
	db, err := gorm.Open(postgres.Open(dsn+" search_path="+schema), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal("isolated test connection failed")
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	if err = db.AutoMigrate(&model.PriceAlert{}, &model.PriceAlertSettings{}); err != nil {
		t.Fatal(err)
	}
	store := repository.NewPriceAlertRepository(db)
	ctx := context.Background()
	a := model.PriceAlert{UserID: 7, AccountID: "bitget", HoldingID: "bitget:spot:BTC", Name: "BTC", Code: "BTC", Currency: "USDT", TargetPrice: 90, Direction: "below", Status: "armed", Revision: 1}
	if err = store.Save(ctx, &a); err != nil {
		t.Fatal(err)
	}
	if err = store.SaveSettings(ctx, &model.PriceAlertSettings{UserID: 7, Email: "me@example.invalid"}); err != nil {
		t.Fatal(err)
	}
	rows, _ := store.List(ctx, 8)
	settings, _ := store.Settings(ctx, 8)
	if len(rows) != 0 || settings.Email != "" {
		t.Fatal("another user's alerts or email exposed")
	}
	if err = store.Remove(ctx, 8, a.HoldingID); err != nil {
		t.Fatal(err)
	}
	rows, _ = store.List(ctx, 7)
	if len(rows) != 1 {
		t.Fatal("foreign deletion affected owner")
	}
	var claims atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, e := store.Claim(ctx, a, 80)
			if e != nil {
				t.Error(e)
			}
			if ok {
				claims.Add(1)
			}
		}()
	}
	wg.Wait()
	if claims.Load() != 1 {
		t.Fatal("duplicate dispatch claim")
	}
	updated := model.PriceAlert{UserID: 7, AccountID: "bitget", HoldingID: a.HoldingID, Name: "BTC", Currency: "USDT", TargetPrice: 120, Direction: "above", Status: "armed", Revision: 1}
	if err = store.Save(ctx, &updated); err != nil {
		t.Fatal(err)
	}
	if updated.ID != a.ID || updated.Revision != 2 || updated.Status != "armed" || updated.TriggeredAt != nil {
		t.Fatalf("rearm failed: %+v", updated)
	}
	if err = store.Delivered(ctx, a, nil); err != nil {
		t.Fatal(err)
	}
	rows, _ = store.List(ctx, 7)
	if len(rows) != 1 || rows[0].Status != "armed" || rows[0].TargetPrice != 120 {
		t.Fatal("old dispatch overwrote new target")
	}
	if ok, _ := store.Claim(ctx, updated, 130); !ok {
		t.Fatal("updated target could not be claimed")
	}
	if err = store.Delivered(ctx, updated, nil); err != nil {
		t.Fatal(err)
	}
	armed, _ := store.Armed(ctx)
	if len(armed) != 0 {
		t.Fatal("sent target still armed")
	}
}
