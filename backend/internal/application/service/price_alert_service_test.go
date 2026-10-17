package service

import (
	"context"
	"errors"
	"math"
	"strings"
	"sync"
	"testing"

	"github.com/songhanxu/wiseinvest/internal/domain/model"
)

type alertMemoryStore struct {
	mu       sync.Mutex
	rows     map[uint]model.PriceAlert
	settings map[uint]model.PriceAlertSettings
	next     uint
}

func newAlertStore() *alertMemoryStore {
	return &alertMemoryStore{rows: map[uint]model.PriceAlert{}, settings: map[uint]model.PriceAlertSettings{}}
}
func (m *alertMemoryStore) List(_ context.Context, u uint) ([]model.PriceAlert, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r := []model.PriceAlert{}
	for _, a := range m.rows {
		if a.UserID == u {
			r = append(r, a)
		}
	}
	return r, nil
}
func (m *alertMemoryStore) Settings(_ context.Context, u uint) (model.PriceAlertSettings, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.settings[u], nil
}
func (m *alertMemoryStore) SaveSettings(_ context.Context, s *model.PriceAlertSettings) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.settings[s.UserID] = *s
	return nil
}
func (m *alertMemoryStore) Save(_ context.Context, a *model.PriceAlert) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, row := range m.rows {
		if row.UserID == a.UserID && row.HoldingID == a.HoldingID {
			a.ID = id
			a.Revision = row.Revision + 1
			m.rows[id] = *a
			return nil
		}
	}
	m.next++
	a.ID = m.next
	m.rows[a.ID] = *a
	return nil
}
func (m *alertMemoryStore) Remove(_ context.Context, u uint, h string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, r := range m.rows {
		if r.UserID == u && r.HoldingID == h {
			delete(m.rows, id)
		}
	}
	return nil
}
func (m *alertMemoryStore) Armed(_ context.Context) ([]model.PriceAlert, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r := []model.PriceAlert{}
	for _, a := range m.rows {
		if a.Status == "armed" {
			r = append(r, a)
		}
	}
	return r, nil
}
func (m *alertMemoryStore) Checked(_ context.Context, a model.PriceAlert, p *float64, msg string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	r := m.rows[a.ID]
	if r.Revision == a.Revision && r.Status == "armed" {
		r.LastPrice = p
		r.LastError = msg
		m.rows[a.ID] = r
	}
	return nil
}
func (m *alertMemoryStore) Claim(_ context.Context, a model.PriceAlert, p float64) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r := m.rows[a.ID]
	if r.Revision != a.Revision || r.Status != "armed" {
		return false, nil
	}
	r.Status = "sending"
	r.LastPrice = &p
	m.rows[a.ID] = r
	return true, nil
}
func (m *alertMemoryStore) Delivered(_ context.Context, a model.PriceAlert, e error) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	r := m.rows[a.ID]
	if r.Revision == a.Revision && r.Status == "sending" {
		r.Status = "sent"
		if e != nil {
			r.Status = "error"
		}
		m.rows[a.ID] = r
	}
	return nil
}
func (m *alertMemoryStore) RecoverInterrupted(context.Context) error { return nil }

type alertFakeMailer struct {
	mu    sync.Mutex
	ready bool
	calls int
	fail  bool
	body  string
}

func (m *alertFakeMailer) Configured() bool { return m.ready }
func (m *alertFakeMailer) Send(_ context.Context, to, subject, body, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	m.body = body
	if m.fail {
		return errors.New("private smtp credentials")
	}
	return nil
}
func TestPriceAlertAutoDirectionAndOneShot(t *testing.T) {
	ctx := context.Background()
	store := newAlertStore()
	mailer := &alertFakeMailer{ready: true}
	price := 100.0
	svc := &PriceAlertService{Store: store, Mailer: mailer, Quotes: func(context.Context, uint, string) ([]AlertQuote, error) {
		return []AlertQuote{{HoldingID: "bitget:spot:BTC", Name: "BTC", Code: "BTC", Currency: "USDT", Price: price, Fresh: true}}, nil
	}}
	if err := svc.SaveEmail(ctx, 7, "me@example.invalid"); err != nil {
		t.Fatal(err)
	}
	row, err := svc.Save(ctx, 7, "bitget", "bitget:spot:BTC", 90, "auto")
	if err != nil || row.Direction != "below" || row.TriggerMode != "auto" {
		t.Fatalf("direction: %+v %v", row, err)
	}
	svc.Run(ctx)
	if mailer.calls != 0 {
		t.Fatal("alert triggered early")
	}
	price = 90
	svc.Run(ctx)
	svc.Run(ctx)
	if mailer.calls != 1 || store.rows[row.ID].Status != "sent" || !strings.Contains(mailer.body, "90 USDT") {
		t.Fatal("one-shot threshold delivery failed")
	}
	row, err = svc.Save(ctx, 7, "bitget", "bitget:spot:BTC", 120, "auto")
	if err != nil || row.Revision != 2 || row.Direction != "above" || row.TriggerMode != "auto" {
		t.Fatal("editing did not rearm")
	}
	price = 121
	svc.Run(ctx)
	if mailer.calls != 2 {
		t.Fatal("rearmed alert did not trigger")
	}
}
func TestPriceAlertsSkipStaleWrongCurrencyMissingRecipientAndInvalidInputs(t *testing.T) {
	ctx := context.Background()
	store := newAlertStore()
	mailer := &alertFakeMailer{ready: true}
	quote := AlertQuote{HoldingID: "h", Price: 100, Fresh: true, Currency: "USD"}
	svc := &PriceAlertService{Store: store, Mailer: mailer, Quotes: func(context.Context, uint, string) ([]AlertQuote, error) { return []AlertQuote{quote}, nil }}
	for _, p := range []float64{0, -1, math.Inf(1), math.NaN(), 1e16} {
		if _, err := svc.Save(ctx, 7, "bitget", "h", p, "auto"); err == nil {
			t.Fatal("invalid price accepted")
		}
	}
	if _, err := svc.Save(ctx, 7, "bitget", "not-owned", 90, "auto"); err == nil {
		t.Fatal("foreign holding accepted")
	}
	if svc.SaveEmail(ctx, 7, "a@example.com\r\nBcc: x@example.com") == nil {
		t.Fatal("header injection accepted")
	}
	row, _ := svc.Save(ctx, 7, "bitget", "h", 110, "above")
	quote.Price = 120
	svc.Run(ctx)
	if mailer.calls != 0 {
		t.Fatal("missing recipient was used")
	}
	_ = svc.SaveEmail(ctx, 7, "me@example.invalid")
	quote.Fresh = false
	svc.Run(ctx)
	quote.Fresh = true
	quote.Currency = "USDT"
	svc.Run(ctx)
	if mailer.calls != 0 || store.rows[row.ID].Status != "armed" {
		t.Fatal("stale or wrong-currency quote triggered alert")
	}
	quote.Currency = "USD"
	mailer.ready = false
	svc.Run(ctx)
	if mailer.calls != 0 {
		t.Fatal("unconfigured mail service used")
	}
	mailer.ready = true
	mailer.fail = true
	svc.Run(ctx)
	svc.Run(ctx)
	if mailer.calls != 1 || store.rows[row.ID].Status != "error" {
		t.Fatal("uncertain failed submission was blindly resent")
	}
}
func TestConcurrentMonitorsClaimOnceAndOldRevisionCannotCompleteNewTarget(t *testing.T) {
	ctx := context.Background()
	store := newAlertStore()
	mailer := &alertFakeMailer{ready: true}
	_ = store.SaveSettings(ctx, &model.PriceAlertSettings{UserID: 7, Email: "me@example.invalid"})
	row := model.PriceAlert{UserID: 7, HoldingID: "h", AccountID: "bitget", TargetPrice: 50, Direction: "above", Currency: "USD", Status: "armed", Revision: 1}
	_ = store.Save(ctx, &row)
	read := func(context.Context, uint, string) ([]AlertQuote, error) {
		return []AlertQuote{{HoldingID: "h", Price: 100, Currency: "USD", Fresh: true}}, nil
	}
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); (&PriceAlertService{Store: store, Mailer: mailer, Quotes: read}).Run(ctx) }()
	}
	wg.Wait()
	if mailer.calls != 1 {
		t.Fatalf("duplicate dispatch: %d", mailer.calls)
	}
	old := row
	row.Status = "armed"
	_ = store.Save(ctx, &row)
	_ = store.Delivered(ctx, old, nil)
	if store.rows[row.ID].Status != "armed" {
		t.Fatal("old email result completed edited target")
	}
}
