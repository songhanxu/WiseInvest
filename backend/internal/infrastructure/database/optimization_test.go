package database

import (
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/songhanxu/wiseinvest/internal/adapter/repository"
	"github.com/songhanxu/wiseinvest/internal/domain/model"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestWatchlistMigrationAndConcurrentMembership(t *testing.T) {
	dsn := os.Getenv("WISEINVEST_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set WISEINVEST_TEST_POSTGRES_DSN to enable isolated PostgreSQL integration test")
	}
	admin, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("wiseinvest_test_%d", time.Now().UnixNano())
	if err := admin.Exec("CREATE SCHEMA " + schema).Error; err != nil {
		t.Fatal(err)
	}

	adminSQL, _ := admin.DB()
	defer func() { admin.Exec("DROP SCHEMA " + schema + " CASCADE"); adminSQL.Close() }()
	db, err := gorm.Open(postgres.Open(dsn+" search_path="+schema), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	if err := AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	if err := db.Migrator().DropIndex(&model.WatchlistItem{}, "idx_watchlist_identity"); err != nil {
		t.Fatal(err)
	}
	for _, item := range []model.WatchlistItem{
		{UserID: 7, Market: "crypto", StockCode: "btc_price", Symbol: "BTC/USDT", Name: "Bitcoin", SortOrder: 1},
		{UserID: 7, Market: "crypto", StockCode: "BTC/USDT", Symbol: "BTC/USDT", Name: "Bitcoin", SortOrder: 2},
		{UserID: 7, Market: "crypto", StockCode: "btc", Symbol: "BTC/USDT", Name: "Bitcoin", SortOrder: 3},
		{UserID: 8, Market: "crypto", StockCode: "BTCUSDT", Symbol: "BTC/USDT", Name: "Bitcoin"},
		{UserID: 7, Market: "a_share", StockCode: "000001", Symbol: "SZ000001", Name: "Ping An"},
		{UserID: 7, Market: "hk_stock", StockCode: "700.HK", Symbol: "00700", Name: "Tencent"},
	} {
		if err := db.Create(&item).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	if err := AutoMigrate(db); err != nil {
		t.Fatalf("migration not repeatable: %v", err)
	}
	repo := repository.NewWatchlistRepository(db)
	rows, err := repo.GetByUser(7)
	if err != nil || len(rows) != 3 {
		t.Fatalf("incorrect migration: %+v %v", rows, err)
	}
	codes := map[string]bool{}
	for _, row := range rows {
		codes[row.StockCode] = true
	}
	for _, code := range []string{"BTC", "sz000001", "00700"} {
		if !codes[code] {
			t.Errorf("missing normalized code %s", code)
		}
	}
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := repo.Add(&model.WatchlistItem{UserID: 7, Market: "us_stock", StockCode: "AAPL", Symbol: "AAPL", Name: "Apple"}); err != nil {
				t.Errorf("concurrent add: %v", err)
			}
		}()
	}
	wg.Wait()
	var count int64
	db.Model(&model.WatchlistItem{}).Where("user_id=7 AND stock_code='AAPL'").Count(&count)
	if count != 1 {
		t.Errorf("concurrent duplicates: %d", count)
	}
	for _, alias := range []string{"btc_price", "BTCUSDT", "BTC/USDT"} {
		if err := db.Create(&model.WatchlistItem{UserID: 7, Market: "crypto", StockCode: alias, Symbol: "BTC/USDT", Name: "Bitcoin"}).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.RemoveCrypto(7, "BTC"); err != nil {
		t.Fatal(err)
	}
	db.Model(&model.WatchlistItem{}).Where("user_id=7 AND market='crypto'").Count(&count)
	if count != 0 {
		t.Error("crypto removal failed")
	}
	if exists, _ := repo.Exists(8, "crypto", "BTC"); !exists {
		t.Error("removed another user's row")
	}
}
