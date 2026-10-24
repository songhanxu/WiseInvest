package handler

import (
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

type secFact struct {
	Start     string   `json:"start"`
	End       string   `json:"end"`
	Value     *float64 `json:"val"`
	Accession string   `json:"accn"`
	Filed     string   `json:"filed"`
	Form      string   `json:"form"`
}
type secConcept struct {
	Units map[string][]secFact `json:"units"`
}
type secCompanyFacts struct {
	CIK   int64                            `json:"cik"`
	Facts map[string]map[string]secConcept `json:"facts"`
}

type ReportedFinancialMetric struct {
	Value       float64 `json:"value"`
	Unit        string  `json:"unit"`
	PeriodStart string  `json:"period_start,omitempty"`
	PeriodEnd   string  `json:"period_end"`
	Tag         string  `json:"tag"`
}
type SECFinancialReport struct {
	Form       string                             `json:"form"`
	FiledDate  string                             `json:"filed_date"`
	ReportDate string                             `json:"report_date"`
	Accession  string                             `json:"accession"`
	SourceURL  string                             `json:"source_url"`
	Metrics    map[string]ReportedFinancialMetric `json:"metrics,omitempty"`
}

var secAccessionPattern = regexp.MustCompile(`^\d{10}-\d{2}-\d{6}$`)
var secCurrencyPattern = regexp.MustCompile(`^[A-Z]{3}$`)

func secArchiveURL(cik, accession, document string) string {
	if !secAccessionPattern.MatchString(accession) {
		return ""
	}
	number, err := strconv.ParseInt(cik, 10, 64)
	if err != nil || number <= 0 {
		return ""
	}
	if document == "" {
		document = accession + "-index.html"
	}
	// Only link to the verified SEC origin; a filing cannot supply its own URL.
	return fmt.Sprintf("https://www.sec.gov/Archives/edgar/data/%d/%s/%s", number, strings.ReplaceAll(accession, "-", ""), url.PathEscape(document))
}

func secReportForm(form string) bool {
	switch strings.TrimSuffix(form, "/A") {
	case "10-K", "10-Q", "20-F", "40-F":
		return true
	}
	return false
}

func secFactValid(f secFact, now time.Time) bool {
	end, err := time.Parse("2006-01-02", f.End)
	filed, filedErr := time.Parse("2006-01-02", f.Filed)
	return f.Value != nil && err == nil && filedErr == nil && !end.After(now) && !filed.After(now) && !filed.Before(end) && secReportForm(f.Form) && secAccessionPattern.MatchString(f.Accession)
}

func secDuration(f secFact) int {
	start, err := time.Parse("2006-01-02", f.Start)
	end, endErr := time.Parse("2006-01-02", f.End)
	if err != nil || endErr != nil || end.Before(start) {
		return 0
	}
	return int(end.Sub(start).Hours()/24) + 1
}

// Select actual reported EPS, never sum quarters or difference cumulative EPS.
// The page labels its period rather than presenting a quarterly number as TTM.
func latestSECEPS(data secCompanyFacts, submissions secSubmissions, now time.Time) (secFact, string, string, bool) {
	for _, form := range submissions.Filings.Recent.Form {
		if !secReportForm(form) {
			continue
		}
		if strings.HasPrefix(form, "20-F") || strings.HasPrefix(form, "40-F") {
			return secFact{}, "", "", false
		}
		break
	}
	var selected secFact
	var selectedUnit, selectedKind string
	for _, tag := range []string{"EarningsPerShareDiluted", "EarningsPerShareBasic"} {
		for unit, facts := range data.Facts["us-gaap"][tag].Units {
			if unit != "USD/shares" {
				continue
			}
			for _, fact := range facts {
				days := secDuration(fact)
				end, _ := time.Parse("2006-01-02", fact.End)
				if !secFactValid(fact, now) || days < 60 || days > 400 || now.Sub(end) > 550*24*time.Hour {
					continue
				}
				if fact.End > selected.End || (fact.End == selected.End && (selectedKind != "EarningsPerShareDiluted" || tag == selectedKind) && (selected.Start < fact.Start || (selected.Start == fact.Start && fact.Filed > selected.Filed))) {
					selected, selectedUnit, selectedKind = fact, unit, tag
				}
			}
		}
	}
	if selected.Value == nil {
		return secFact{}, "", "", false
	}
	kind := "稀释"
	if selectedKind == "EarningsPerShareBasic" {
		kind = "基本"
	}
	period := "累计"
	if secDuration(selected) <= 110 {
		period = "单季"
	} else if secDuration(selected) >= 330 {
		period = "年度"
	}
	return selected, selectedUnit, "GAAP " + kind + " · " + period, true
}

// Build trailing EPS from a reported fiscal year and matching current/prior
// year-to-date figures. The result is an estimate because reported EPS uses
// period-specific weighted average shares; it must be labelled accordingly.
func secTrailingEPS(data secCompanyFacts, latest secFact, basis string, now time.Time) (float64, string, string, bool) {
	if latest.Value == nil || !strings.HasPrefix(basis, "GAAP ") {
		return 0, "", "", false
	}
	tag := "EarningsPerShareDiluted"
	if strings.Contains(basis, "基本") {
		tag = "EarningsPerShareBasic"
	}
	facts := data.Facts["us-gaap"][tag].Units["USD/shares"]
	if secDuration(latest) >= 330 && strings.TrimSuffix(latest.Form, "/A") == "10-K" {
		return *latest.Value, latest.Start, latest.End, true
	}
	if secDuration(latest) > 300 {
		return 0, "", "", false
	}
	var annual secFact
	for _, fact := range facts {
		if !secFactValid(fact, now) || strings.TrimSuffix(fact.Form, "/A") != "10-K" || secDuration(fact) < 330 || secDuration(fact) > 400 || fact.End >= latest.End {
			continue
		}
		if fact.End > annual.End || (fact.End == annual.End && fact.Filed > annual.Filed) {
			annual = fact
		}
	}
	if annual.Value == nil {
		return 0, "", "", false
	}
	annualStart, _ := time.Parse("2006-01-02", annual.Start)
	annualEnd, _ := time.Parse("2006-01-02", annual.End)
	var current, prior secFact
	for _, fact := range facts {
		if !secFactValid(fact, now) || strings.TrimSuffix(fact.Form, "/A") != "10-Q" || secDuration(fact) < 60 || secDuration(fact) > 300 {
			continue
		}
		start, _ := time.Parse("2006-01-02", fact.Start)
		if fact.End == latest.End && absDays(start.Sub(annualEnd.AddDate(0, 0, 1))) <= 7 {
			if current.Value == nil || fact.Filed > current.Filed || (fact.Filed == current.Filed && secDuration(fact) > secDuration(current)) {
				current = fact
			}
		}
	}
	if current.Value == nil {
		return 0, "", "", false
	}
	for _, fact := range facts {
		if !secFactValid(fact, now) || strings.TrimSuffix(fact.Form, "/A") != "10-Q" || fact.End >= annual.End || absDaysDuration(secDuration(fact)-secDuration(current)) > 10 {
			continue
		}
		start, _ := time.Parse("2006-01-02", fact.Start)
		if absDays(start.Sub(annualStart)) > 7 {
			continue
		}
		if prior.Value == nil || (fact.Accession == current.Accession && prior.Accession != current.Accession) || (fact.Accession == prior.Accession && fact.Filed > prior.Filed) {
			prior = fact
		}
	}
	if prior.Value == nil {
		return 0, "", "", false
	}
	priorEnd, _ := time.Parse("2006-01-02", prior.End)
	start := priorEnd.AddDate(0, 0, 1)
	end, _ := time.Parse("2006-01-02", current.End)
	if days := absDays(end.Sub(start)) + 1; days < 330 || days > 400 {
		return 0, "", "", false
	}
	return *annual.Value + *current.Value - *prior.Value, start.Format("2006-01-02"), current.End, true
}

func absDays(d time.Duration) int {
	days := int(d.Hours() / 24)
	return absDaysDuration(days)
}

func absDaysDuration(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func buildSECReports(data secCompanyFacts, submissions secSubmissions, cik string, now time.Time) []SECFinancialReport {
	var reports []SECFinancialReport
	recent := submissions.Filings.Recent
	for i, accession := range recent.Accession {
		if i >= len(recent.Form) || i >= len(recent.Filed) || i >= len(recent.ReportDate) || i >= len(recent.Document) || !secReportForm(recent.Form[i]) {
			continue
		}
		filed, err := time.Parse("2006-01-02", recent.Filed[i])
		end, endErr := time.Parse("2006-01-02", recent.ReportDate[i])
		link := secArchiveURL(cik, accession, recent.Document[i])
		if err != nil || endErr != nil || filed.After(now) || end.After(now) || link == "" {
			continue
		}
		reports = append(reports, SECFinancialReport{Form: recent.Form[i], FiledDate: recent.Filed[i], ReportDate: recent.ReportDate[i], Accession: accession, SourceURL: link})
	}
	sort.SliceStable(reports, func(i, j int) bool {
		if reports[i].ReportDate != reports[j].ReportDate {
			return reports[i].ReportDate > reports[j].ReportDate
		}
		return reports[i].FiledDate > reports[j].FiledDate
	})
	if len(reports) > 8 {
		reports = reports[:8]
	}
	// Keep taxonomy, units and each metric's actual duration. A cash flow value
	// reported year-to-date must not be relabelled as a standalone quarter.
	metrics := []struct {
		key     string
		tags    []string
		instant bool
	}{
		{"revenue", []string{"RevenueFromContractWithCustomerExcludingAssessedTax", "Revenues", "SalesRevenueNet", "Revenue"}, false},
		{"net_income", []string{"NetIncomeLoss", "ProfitLoss"}, false},
		{"assets", []string{"Assets"}, true},
		{"liabilities", []string{"Liabilities"}, true},
		{"equity", []string{"StockholdersEquity", "Equity"}, true},
		{"operating_cash_flow", []string{"NetCashProvidedByUsedInOperatingActivities", "CashFlowsFromUsedInOperatingActivities"}, false},
	}
	for i := range reports {
		report := &reports[i]
		report.Metrics = make(map[string]ReportedFinancialMetric)
		for _, metric := range metrics {
			for _, taxonomy := range []string{"us-gaap", "ifrs-full"} {
				for _, tag := range metric.tags {
					units := data.Facts[taxonomy][tag].Units
					keys := make([]string, 0, len(units))
					for unit := range units {
						keys = append(keys, unit)
					}
					sort.Strings(keys)
					for _, unit := range keys {
						facts := units[unit]
						// Only monetary ISO-style units are appropriate for these metrics.
						if !secCurrencyPattern.MatchString(unit) {
							continue
						}
						for _, fact := range facts {
							if fact.Accession != report.Accession || fact.End != report.ReportDate || !secFactValid(fact, now) {
								continue
							}
							if metric.instant && fact.Start != "" {
								continue
							}
							if !metric.instant && (secDuration(fact) < 60 || secDuration(fact) > 400) {
								continue
							}
							old, exists := report.Metrics[metric.key]
							if exists {
								// Do not combine units or taxonomy tags within one metric.
								if old.Unit != unit || old.Tag != taxonomy+":"+tag {
									continue
								}
								if fact.Start <= old.PeriodStart {
									continue
								}
							}
							report.Metrics[metric.key] = ReportedFinancialMetric{Value: *fact.Value, Unit: unit, PeriodStart: fact.Start, PeriodEnd: fact.End, Tag: taxonomy + ":" + tag}
						}
					}
				}
			}
		}
	}
	return reports
}
