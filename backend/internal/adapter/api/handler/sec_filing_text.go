package handler

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html"
)

type filingFact struct {
	Tag         string  `json:"tag"`
	Unit        string  `json:"unit"`
	Value       float64 `json:"value"`
	PeriodStart string  `json:"period_start,omitempty"`
	PeriodEnd   string  `json:"period_end"`
}

func (s *secFundamentalsClient) reportFacts(ctx context.Context, cik string, report SECFinancialReport) ([]filingFact, error) {
	number, err := strconv.ParseUint(cik, 10, 32)
	if err != nil || number == 0 {
		return nil, fmt.Errorf("invalid SEC CIK")
	}
	var data secCompanyFacts
	if message := s.get(ctx, fmt.Sprintf("%s/api/xbrl/companyfacts/CIK%010d.json", s.baseURL, number), &data); message != "" {
		return nil, fmt.Errorf("%s", message)
	}
	if data.CIK != int64(number) {
		return nil, fmt.Errorf("SEC CIK mismatch")
	}
	var facts []filingFact
	for _, taxonomy := range []string{"us-gaap", "ifrs-full"} {
		for tag, concept := range data.Facts[taxonomy] {
			if !filingFactRelevant(tag) {
				continue
			}
			for unit, entries := range concept.Units {
				if !secCurrencyPattern.MatchString(unit) && unit != "USD/shares" && unit != "shares" && unit != "pure" {
					continue
				}
				for _, fact := range entries {
					if fact.Accession != report.Accession || fact.End != report.ReportDate || !secFactValid(fact, time.Now()) {
						continue
					}
					if fact.Start != "" && (secDuration(fact) < 60 || secDuration(fact) > 400) {
						continue
					}
					facts = append(facts, filingFact{Tag: taxonomy + ":" + tag, Unit: unit, Value: *fact.Value, PeriodStart: fact.Start, PeriodEnd: fact.End})
				}
			}
		}
	}
	sort.Slice(facts, func(i, j int) bool {
		if facts[i].Tag != facts[j].Tag {
			return facts[i].Tag < facts[j].Tag
		}
		if facts[i].PeriodStart != facts[j].PeriodStart {
			return facts[i].PeriodStart > facts[j].PeriodStart
		}
		return facts[i].Unit < facts[j].Unit
	})
	if len(facts) > 120 {
		facts = facts[:120]
	}
	return facts, nil
}

func filingFactRelevant(tag string) bool {
	for _, term := range []string{"Revenue", "Sales", "NetIncome", "ProfitLoss", "OperatingIncome", "GrossProfit", "CostOf", "Cash", "Assets", "Liabilities", "Equity", "Debt", "Inventory", "Receivable", "Payable", "Research", "Development", "Tax", "Dividend", "EarningsPerShare", "WeightedAverage", "ShareBased", "StockBased", "CapitalExpenditure", "PropertyPlant", "Goodwill", "Impairment", "Segment"} {
		if strings.Contains(tag, term) {
			return true
		}
	}
	return false
}

// The filing URL is built from SEC submission identifiers, never from the
// browser. Keep downloaded text bounded before passing it to the model.
func (s *secFundamentalsClient) filingExcerpt(ctx context.Context, report SECFinancialReport) (string, error) {
	address, err := url.Parse(report.SourceURL)
	if err != nil || address.Scheme != "https" || address.Host != "www.sec.gov" || !strings.HasPrefix(address.Path, "/Archives/edgar/data/") {
		return "", fmt.Errorf("invalid SEC filing address")
	}
	resp, message := s.request(ctx, report.SourceURL, "text/html")
	if message != "" {
		return "", fmt.Errorf("%s", message)
	}
	defer resp.Body.Close()
	if contentType := resp.Header.Get("Content-Type"); contentType != "" && !strings.Contains(contentType, "text/html") && !strings.Contains(contentType, "text/plain") {
		return "", fmt.Errorf("SEC filing is not HTML or text")
	}
	const maxFilingBytes = 8 << 20
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxFilingBytes+1))
	if err != nil || len(raw) > maxFilingBytes {
		return "", fmt.Errorf("SEC filing is too large or incomplete")
	}
	return extractFilingExcerpt(raw), nil
}

func extractFilingExcerpt(raw []byte) string {
	root, err := html.Parse(strings.NewReader(string(raw)))
	if err != nil {
		return ""
	}
	var text strings.Builder
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if text.Len() >= 2<<20 {
			return
		}
		if node.Type == html.ElementNode {
			switch node.Data {
			case "head", "script", "style", "noscript", "svg", "ix:header", "ix:hidden", "ix:resources", "ix:references":
				return
			}
			for _, attr := range node.Attr {
				if attr.Key == "hidden" || (attr.Key == "style" && (strings.Contains(strings.ToLower(attr.Val), "display:none") || strings.Contains(strings.ToLower(attr.Val), "display: none"))) {
					return
				}
			}
		}
		if node.Type == html.TextNode {
			value := strings.TrimSpace(node.Data)
			if value != "" {
				text.WriteString(value)
				text.WriteByte(' ')
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
		if node.Type == html.ElementNode {
			switch node.Data {
			case "div", "p", "tr", "h1", "h2", "h3", "h4", "section":
				text.WriteByte('\n')
			}
		}
	}
	walk(root)
	clean := strings.Join(strings.Fields(text.String()), " ")
	if clean == "" {
		return ""
	}
	var excerpts []string
	if len(clean) > 2500 {
		excerpts = append(excerpts, clean[:2500])
	} else {
		excerpts = append(excerpts, clean)
	}
	lower := strings.ToLower(clean)
	for _, section := range []struct {
		name string
		keys []string
		max  int
	}{
		{"经营讨论", []string{"management's discussion and analysis", "management’s discussion and analysis"}, 16000},
		{"经营结果", []string{"results of operations"}, 7000},
		{"风险因素", []string{"risk factors"}, 5500},
	} {
		position := -1
		for _, key := range section.keys {
			found := strings.Index(lower, key)
			if len(lower) > 5000 {
				if body := strings.Index(lower[5000:], key); body >= 0 {
					found = body + 5000
				}
			}
			if found >= 0 && (position < 0 || found < position) {
				position = found
			}
		}
		if position >= 0 {
			end := position + section.max
			if end > len(clean) {
				end = len(clean)
			}
			excerpts = append(excerpts, section.name+"："+clean[position:end])
		}
	}
	return strings.ToValidUTF8(strings.Join(excerpts, "\n\n"), "")
}
