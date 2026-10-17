package handler

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/songhanxu/wiseinvest/internal/adapter/repository"
	"github.com/songhanxu/wiseinvest/internal/domain/model"
)

type alertWatchlistRepo struct {
	*repository.WatchlistRepository
	items map[uint][]model.WatchlistItem
	err   error
}

func (r *alertWatchlistRepo) GetByUserContext(ctx context.Context, user uint) ([]model.WatchlistItem, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return r.items[user], r.err
}

func TestWatchlistAlertsUseOwnedMembershipAndPublicQuotes(t *testing.T) {
	repo := &alertWatchlistRepo{items: map[uint][]model.WatchlistItem{
		7: {{Market: "a_share", StockCode: "600519", Symbol: "SH600519", Name: "Owned"}},
		8: {{Market: "a_share", StockCode: "000001", Symbol: "SZ000001", Name: "Other user"}},
	}}
	calls := 0
	fail := false
	h := &StockHandler{watchlistRepo: repo, httpClient: &http.Client{Transport: quoteTransport(func(req *http.Request) (*http.Response, error) {
		calls++
		if req.URL.Path != "/q=sh600519" {
			t.Errorf("unexpected symbol request %s", req.URL.Path)
		}
		if fail {
			return nil, errors.New("provider unavailable")
		}
		fields := make([]string, 50)
		fields[1], fields[2], fields[3], fields[4] = "Owned", "600519", "1500", "1490"
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`v_sh600519="` + strings.Join(fields, "~") + `";`))}, nil
	})}}
	ctx := context.Background()
	quotes, err := h.WatchlistPriceAlertQuotes(ctx, 7)
	if err != nil || len(quotes) != 1 {
		t.Fatalf("watchlist quotes: %+v %v", quotes, err)
	}
	q := quotes[0]
	if q.HoldingID != "watchlist:a_share:sh600519" || q.AccountID != "watchlist" || q.Currency != "CNY" || q.Price != 1500 || !q.Fresh {
		t.Fatalf("wrong alert identity/price: %+v", q)
	}
	if rows, err := h.WatchlistPriceAlertQuotes(ctx, 9); err != nil || len(rows) != 0 || calls != 1 {
		t.Fatal("another user's watchlist leaked or triggered quote requests")
	}
	if _, err := h.WatchlistPriceAlertQuotes(ctx, 0); err == nil || calls != 1 {
		t.Fatal("anonymous lookup allowed")
	}
	fail = true
	quotes, err = h.WatchlistPriceAlertQuotes(ctx, 7)
	if err != nil || len(quotes) != 1 || quotes[0].Fresh || quotes[0].Price != 0 {
		t.Fatal("failed provider quote can trigger alert")
	}
	fail = false
	repo.items[7] = nil
	quotes, err = h.WatchlistPriceAlertQuotes(ctx, 7)
	if err != nil || len(quotes) != 0 || calls != 2 {
		t.Fatal("removed symbol still monitored")
	}
	repo.err = errors.New("database unavailable")
	if _, err = h.WatchlistPriceAlertQuotes(ctx, 7); err == nil {
		t.Fatal("membership failure ignored")
	}
}
