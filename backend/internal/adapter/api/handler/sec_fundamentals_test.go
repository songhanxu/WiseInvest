package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func secFixture(t *testing.T, serve http.HandlerFunc) *secFundamentalsClient {
	t.Helper()
	server := httptest.NewServer(serve)
	t.Cleanup(server.Close)
	s := newSECFundamentalsClient("WiseInvest test contact@example.com")
	s.baseURL, s.tickerURL = server.URL, server.URL+"/tickers"
	return s
}

func secExampleData(now time.Time) (secCompanyFacts, secSubmissions) {
	end := now.AddDate(0, 0, -30).UTC()
	filed := now.AddDate(0, 0, -10).UTC().Format("2006-01-02")
	quarter := secFact{Start: end.AddDate(0, 0, -89).Format("2006-01-02"), End: end.Format("2006-01-02"), Filed: filed, Form: "10-Q", Accession: "0000000123-26-000001", Value: floatPtr(2.5)}
	cumulative := quarter
	cumulative.Start = end.AddDate(0, 0, -269).Format("2006-01-02")
	cumulative.Value = floatPtr(7)
	instant := quarter
	instant.Start = ""
	instant.Value = floatPtr(100)
	facts := secCompanyFacts{CIK: 123, Facts: map[string]map[string]secConcept{"us-gaap": {
		"EarningsPerShareDiluted":                             {Units: map[string][]secFact{"USD/shares": {cumulative, quarter}}},
		"RevenueFromContractWithCustomerExcludingAssessedTax": {Units: map[string][]secFact{"USD": {cumulative, quarter}}},
		"Assets": {Units: map[string][]secFact{"USD": {instant}}},
		"NetCashProvidedByUsedInOperatingActivities": {Units: map[string][]secFact{"USD": {cumulative}}},
	}}}
	var submissions secSubmissions
	submissions.CIK, submissions.Tickers = "0000000123", []string{"TEST", "TEST-A"}
	submissions.Filings.Recent.Accession = []string{quarter.Accession}
	submissions.Filings.Recent.Form = []string{quarter.Form}
	submissions.Filings.Recent.Filed = []string{filed}
	submissions.Filings.Recent.ReportDate = []string{quarter.End}
	submissions.Filings.Recent.Document = []string{"test-report.htm"}
	return facts, submissions
}

func TestSECReportedEPSKeepsDurationAmendmentsAndUnits(t *testing.T) {
	now := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	facts, submissions := secExampleData(now)
	quarter := facts.Facts["us-gaap"]["EarningsPerShareDiluted"].Units["USD/shares"][1]
	restatement := quarter
	restatement.Value = floatPtr(0)
	restatement.Filed = now.AddDate(0, 0, -1).Format("2006-01-02")
	restatement.Form = "10-Q/A"
	invalid := quarter
	invalid.End = "2099-12-31"
	invalid.Value = floatPtr(99)
	units := facts.Facts["us-gaap"]["EarningsPerShareDiluted"].Units
	units["USD/shares"] = append(units["USD/shares"], restatement, invalid)
	foreignUnit := quarter
	foreignUnit.Value = floatPtr(500)
	units["CNY/shares"] = []secFact{foreignUnit}
	eps, unit, basis, ok := latestSECEPS(facts, submissions, now)
	if !ok || *eps.Value != 0 || eps.Start != quarter.Start || unit != "USD/shares" || basis != "GAAP 稀释 · 单季" {
		t.Fatalf("bad reported EPS: %+v %s %s", eps, unit, basis)
	}
	if strings.Contains(basis, "TTM") {
		t.Fatal("quarter mislabeled TTM")
	}
	// An annual fact is displayed as annual, never relabelled as four-quarter TTM.
	annual := quarter
	annual.Start = now.AddDate(0, 0, -394).Format("2006-01-02")
	annual.Form = "10-K"
	units["USD/shares"] = []secFact{annual}
	_, _, basis, ok = latestSECEPS(facts, submissions, now)
	if !ok || basis != "GAAP 稀释 · 年度" {
		t.Fatal("annual period lost")
	}
	submissions.Filings.Recent.Form = []string{"20-F"}
	if _, _, _, ok = latestSECEPS(facts, submissions, now); ok {
		t.Fatal("unverified ADR EPS accepted as stock EPS")
	}
}

func TestSECTrailingEPSRequiresMatchingFiscalPeriods(t *testing.T) {
	now := time.Date(2026, 10, 23, 0, 0, 0, 0, time.UTC)
	annual := secFact{Start: "2024-09-29", End: "2025-09-27", Filed: "2025-10-31", Form: "10-K", Accession: "0000000123-25-000001", Value: floatPtr(8)}
	prior := secFact{Start: annual.Start, End: "2025-06-28", Filed: "2026-08-01", Form: "10-Q", Accession: "0000000123-26-000001", Value: floatPtr(6)}
	current := secFact{Start: "2025-09-28", End: "2026-06-27", Filed: prior.Filed, Form: "10-Q", Accession: prior.Accession, Value: floatPtr(7)}
	latest := secFact{Start: "2026-03-29", End: current.End, Filed: current.Filed, Form: "10-Q", Accession: current.Accession, Value: floatPtr(2.5)}
	facts := secCompanyFacts{Facts: map[string]map[string]secConcept{"us-gaap": {"EarningsPerShareDiluted": {Units: map[string][]secFact{"USD/shares": {annual, prior, current, latest}}}}}}
	value, start, end, ok := secTrailingEPS(facts, latest, "GAAP 稀释 · 单季", now)
	if !ok || value != 9 || start != "2025-06-29" || end != latest.End {
		t.Fatalf("wrong trailing EPS: %v %s %s %v", value, start, end, ok)
	}
	if value, _, _, ok = secTrailingEPS(facts, annual, "GAAP 稀释 · 年度", now); !ok || value != 8 {
		t.Fatal("annual EPS should be the latest fiscal-year result")
	}
	delete(facts.Facts["us-gaap"]["EarningsPerShareDiluted"].Units, "USD/shares")
	if _, _, _, ok = secTrailingEPS(facts, latest, "GAAP 稀释 · 单季", now); ok {
		t.Fatal("missing comparative periods must not produce estimated EPS")
	}
}

func TestSECPEDemandsFreshUSDStockQuote(t *testing.T) {
	now := time.Date(2026, 10, 23, 0, 0, 0, 0, time.UTC)
	row := FundamentalsResponse{TrailingEPS: floatPtr(5), Metrics: map[string]MetricProvenance{"eps": {SourceURL: "https://www.sec.gov/Archives/example"}}}
	quote := StockResponse{CurrentPrice: 100, Currency: "USD", Source: "tencent", ProductType: "equity", QuoteTime: now.Add(-time.Hour).Format(time.RFC3339)}
	setSECTrailingPE(&row, quote, now)
	if row.PE == nil || *row.PE != 20 || row.PEPrice == nil || *row.PEPrice != 100 || row.Metrics["pe"].SourceURL == "" {
		t.Fatalf("verified quote failed: %+v", row)
	}
	for _, invalid := range []StockResponse{
		{CurrentPrice: 100, Currency: "USDT", Source: "bitget", ProductType: "stock_token", QuoteTime: quote.QuoteTime},
		{CurrentPrice: 100, Currency: "USD", Source: "tencent", ProductType: "equity", QuoteTime: now.Add(-8 * 24 * time.Hour).Format(time.RFC3339)},
	} {
		candidate := FundamentalsResponse{TrailingEPS: floatPtr(5)}
		setSECTrailingPE(&candidate, invalid, now)
		if candidate.PE != nil {
			t.Fatalf("invalid quote produced PE: %+v", invalid)
		}
	}
}

func TestSECReportsPreserveCashFlowYTDAndFilingIdentity(t *testing.T) {
	now := time.Now()
	facts, submissions := secExampleData(now)
	wrong := facts.Facts["us-gaap"]["Assets"].Units["USD"][0]
	wrong.Accession = "0000000123-26-999999"
	wrong.Value = floatPtr(999)
	units := facts.Facts["us-gaap"]["Assets"].Units
	units["USD"] = append(units["USD"], wrong)
	reports := buildSECReports(facts, submissions, "0000000123", now)
	if len(reports) != 1 {
		t.Fatal("report missing")
	}
	report := reports[0]
	if report.Metrics["assets"].Value != 100 || report.Metrics["assets"].PeriodStart != "" || report.Metrics["revenue"].PeriodStart == report.Metrics["operating_cash_flow"].PeriodStart {
		t.Fatalf("mixed reporting durations: %+v", report)
	}
	if !strings.HasPrefix(report.SourceURL, "https://www.sec.gov/Archives/edgar/data/123/") || report.FiledDate == report.ReportDate {
		t.Fatal("filing provenance lost")
	}
	if _, ok := report.Metrics["liabilities"]; ok {
		t.Fatal("missing balance sheet value fabricated")
	}
	if link := secArchiveURL("123", "not-an-accession", "https://evil.example/"); link != "" {
		t.Fatal("invalid filing URL accepted")
	}
	// A newly filed amendment to an older period is not the latest report.
	recent := &submissions.Filings.Recent
	recent.Accession = append(recent.Accession, "0000000123-26-000002")
	recent.Form = append(recent.Form, "10-Q/A")
	recent.Filed = append(recent.Filed, now.AddDate(0, 0, -1).Format("2006-01-02"))
	recent.ReportDate = append(recent.ReportDate, now.AddDate(0, -6, 0).Format("2006-01-02"))
	recent.Document = append(recent.Document, "older-amendment.htm")
	if reports = buildSECReports(facts, submissions, "0000000123", now); reports[0].ReportDate != report.ReportDate {
		t.Fatal("older amended period shown as latest report")
	}
	// Broken column arrays must be ignored rather than panic.
	submissions.Filings.Recent.ReportDate = nil
	if reports = buildSECReports(facts, submissions, "0000000123", now); len(reports) != 0 {
		t.Fatal("malformed filing accepted")
	}
}

func TestSECIsPrimaryAndYahooKeepsItsOwnValuationProvenance(t *testing.T) {
	facts, submissions := secExampleData(time.Now())
	sec := secFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("User-Agent"), "contact@example.com") {
			t.Error("SEC contact missing")
		}
		switch r.URL.Path {
		case "/tickers":
			fmt.Fprint(w, `{"0":{"cik_str":123,"ticker":"TEST"}}`)
		case "/api/xbrl/companyfacts/CIK0000000123.json":
			json.NewEncoder(w).Encode(facts)
		case "/submissions/CIK0000000123.json":
			json.NewEncoder(w).Encode(submissions)
		default:
			t.Errorf("unexpected SEC path %s", r.URL.Path)
			w.WriteHeader(404)
		}
	})
	yahoo := yahooFixture(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cookie":
			w.WriteHeader(404)
		case "/crumb":
			fmt.Fprint(w, "crumb")
		default:
			fmt.Fprint(w, `{"quoteSummary":{"result":[{"financialData":{"financialCurrency":"USD"},"defaultKeyStatistics":{"trailingEps":999,"forwardEps":10,"forwardPE":20},"summaryDetail":{"trailingPE":30}}]}}`)
		}
	})
	s := NewFundamentalsService("")
	s.SEC, s.Yahoo, s.FinnhubAPIKey = sec, yahoo, ""
	row := s.Fetch(context.Background(), "us_stock", "TEST")
	if row.EPS == nil || *row.EPS != 2.5 || !strings.HasPrefix(row.Metrics["eps"].Source, "SEC") || row.PE == nil || *row.PE != 30 || !strings.HasPrefix(row.Metrics["pe"].Source, "Yahoo") || row.EPSBasis != "GAAP 稀释 · 单季" || len(row.FinancialReports) != 1 || row.NextEarningsDate != "" || row.GuidanceEPS != nil {
		t.Fatalf("SEC did not remain primary: %+v", row)
	}
	if row.Metrics["eps"].SourceURL == "" {
		t.Fatal("EPS filing link missing")
	}
}

func TestSECMappingSharesRefreshAndNormalizesClassTicker(t *testing.T) {
	var calls atomic.Int32
	sec := secFixture(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		fmt.Fprint(w, `{"0":{"cik_str":123,"ticker":"TEST-A"}}`)
	})
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cik, message := sec.cik(context.Background(), "test.a")
			if cik != "0000000123" || message != "" {
				t.Errorf("bad map %s %s", cik, message)
			}
		}()
	}
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatal("mapping refresh not shared")
	}
}

func TestSECFailureFallsBackAndBacksOffWithoutLeakingResponse(t *testing.T) {
	var calls atomic.Int32
	sec := secFixture(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(429)
		fmt.Fprint(w, "upstream-secret")
	})
	sec.tickers = map[string]string{"TEST": "0000000123"}
	sec.tickerExpiry = time.Now().Add(time.Hour)
	first := sec.fetch(context.Background(), "TEST")
	second := sec.fetch(context.Background(), "TEST")
	if first.EPS != nil || second.EPS != nil || strings.Contains(first.Message, "upstream-secret") || strings.Contains(second.Message, "upstream-secret") || calls.Load() > 2 {
		t.Fatal("SEC rate-limit backoff or sanitization failed")
	}
	yahoo := yahooFixture(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cookie":
			w.WriteHeader(404)
		case "/crumb":
			fmt.Fprint(w, "crumb")
		default:
			fmt.Fprint(w, `{"quoteSummary":{"result":[{"financialData":{"financialCurrency":"USD"},"defaultKeyStatistics":{"trailingEps":3}}]}}`)
		}
	})
	s := NewFundamentalsService("")
	s.SEC, s.Yahoo, s.FinnhubAPIKey = sec, yahoo, ""
	row := s.Fetch(context.Background(), "us_stock", "TEST")
	if row.EPS == nil || *row.EPS != 3 || row.EPSBasis != "TTM（供应商）" || !strings.HasPrefix(row.Metrics["eps"].Source, "Yahoo") {
		t.Fatalf("SEC failure blocked fallback: %+v", row)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	sec.blockedUntil = time.Time{}
	sec.nextRequest = time.Now().Add(time.Hour)
	if message := sec.get(ctx, sec.baseURL, nil); message != "SEC 查询超时" {
		t.Fatalf("cancel ignored: %s", message)
	}
}

func TestSECRejectsMismatchedCIKOrTicker(t *testing.T) {
	facts, submissions := secExampleData(time.Now())
	facts.CIK = 999
	submissions.Tickers = []string{"OTHER"}
	sec := secFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "companyfacts") {
			json.NewEncoder(w).Encode(facts)
		} else {
			json.NewEncoder(w).Encode(submissions)
		}
	})
	sec.tickers = map[string]string{"TEST": "0000000123"}
	sec.tickerExpiry = time.Now().Add(time.Hour)
	row := sec.fetch(context.Background(), "TEST")
	if row.Status != "unavailable" || row.EPS != nil || len(row.FinancialReports) > 0 || row.Source != "" {
		t.Fatalf("wrong company data accepted: %+v", row)
	}
}

// Opt-in check against responses downloaded from the official APIs. Ordinary
// unit tests never depend on internet access or embed real company values.
func TestSECDownloadedOfficialResponses(t *testing.T) {
	dir := os.Getenv("SEC_LIVE_FIXTURE_DIR")
	if dir == "" {
		t.Skip("set SEC_LIVE_FIXTURE_DIR to verify downloaded official responses")
	}
	var facts secCompanyFacts
	var submissions secSubmissions
	for name, target := range map[string]any{"facts.json": &facts, "submissions.json": &submissions} {
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if err = json.Unmarshal(raw, target); err != nil {
			t.Fatal(err)
		}
	}
	eps, _, basis, ok := latestSECEPS(facts, submissions, time.Now())
	if !ok || eps.Value == nil {
		t.Fatal("no reported EPS parsed")
	}
	reports := buildSECReports(facts, submissions, submissions.CIK, time.Now())
	if len(reports) == 0 || len(reports[0].Metrics) == 0 {
		t.Fatal("no financial report parsed")
	}
	t.Logf("official response verified: EPS period %s to %s (%s), %d reports, %d latest-report metrics", eps.Start, eps.End, basis, len(reports), len(reports[0].Metrics))
}

func TestSECLiveClient(t *testing.T) {
	if os.Getenv("SEC_LIVE_CHECK") != "1" {
		t.Skip("set SEC_LIVE_CHECK=1 for official network verification")
	}
	client := newSECFundamentalsClient(os.Getenv("SEC_USER_AGENT"))
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	row := client.fetch(ctx, "AAPL")
	if row.Status != "available" || row.EPS == nil || row.TrailingEPS == nil || row.Metrics["eps"].SourceURL == "" || len(row.FinancialReports) == 0 {
		t.Fatalf("official SEC client failed: status=%s diagnostic=%s", row.Status, row.Message)
	}
	t.Logf("live SEC client verified: CIK=%s, EPS period=%s to %s, basis=%s, trailing EPS=%v for %s to %s, reports=%d", row.SECCIK, row.EPSPeriodStart, row.EPSPeriodEnd, row.EPSBasis, *row.TrailingEPS, row.TrailingEPSStart, row.TrailingEPSEnd, len(row.FinancialReports))
}
