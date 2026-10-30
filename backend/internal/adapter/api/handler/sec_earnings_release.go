package handler

import (
	"context"
	"io"
	"regexp"
	"strings"
	"time"

	"golang.org/x/net/html"
)

var earningsPeriodPattern = regexp.MustCompile(`(?i)(?:quarter|three months|year|twelve months)\s+ended\s+(January|February|March|April|May|June|July|August|September|October|November|December)\s+(\d{1,2}),?\s+(\d{4})`)
var secExhibitNamePattern = regexp.MustCompile(`(?i)^[a-z0-9][a-z0-9._-]*\.html?$`)

// Item 2.02 is the SEC filing that carries an earnings announcement. Other
// 8-K events must not replace a periodic report in the latest-report column.
func latestSECEarningsRelease(submissions secSubmissions, cik string, now time.Time) (SECFinancialReport, bool) {
	recent := submissions.Filings.Recent
	var selected SECFinancialReport
	for i, accession := range recent.Accession {
		if i >= len(recent.Form) || i >= len(recent.Filed) || i >= len(recent.Document) || i >= len(recent.Items) || !strings.HasPrefix(recent.Form[i], "8-K") {
			continue
		}
		items := strings.Split(recent.Items[i], ",")
		isEarnings := false
		for _, item := range items {
			if strings.TrimSpace(item) == "2.02" {
				isEarnings = true
				break
			}
		}
		filed, err := time.Parse("2006-01-02", recent.Filed[i])
		link := secArchiveURL(cik, accession, recent.Document[i])
		if !isEarnings || err != nil || filed.After(now) || link == "" || (selected.FiledDate != "" && recent.Filed[i] <= selected.FiledDate) {
			continue
		}
		selected = SECFinancialReport{Form: "8-K", Kind: "earnings_release", FiledDate: recent.Filed[i], Accession: accession, SourceURL: link}
	}
	return selected, selected.FiledDate != ""
}

// Resolve the actual quarter from the company's SEC filing, not the 8-K's
// reportDate field, which is normally the date of the announcement itself.
func (s *secFundamentalsClient) resolveEarningsRelease(ctx context.Context, cik string, release SECFinancialReport) (SECFinancialReport, bool) {
	primary, ok := s.earningsDocument(ctx, release.SourceURL)
	if !ok {
		return SECFinancialReport{}, false
	}
	period := earningsPeriodEnd(extractFilingExcerpt(primary), release.FiledDate)
	if exhibit := earningsExhibitDocument(primary); exhibit != "" {
		if link := secArchiveURL(cik, release.Accession, exhibit); link != "" {
			release.SourceURL = link
			if period == "" {
				if body, ok := s.earningsDocument(ctx, link); ok {
					period = earningsPeriodEnd(extractFilingExcerpt(body), release.FiledDate)
				}
			}
		}
	}
	if period == "" {
		return SECFinancialReport{}, false
	}
	release.ReportDate = period
	return release, true
}

func (s *secFundamentalsClient) earningsDocument(ctx context.Context, address string) ([]byte, bool) {
	resp, message := s.request(ctx, address, "text/html")
	if message != "" {
		return nil, false
	}
	defer resp.Body.Close()
	if contentType := resp.Header.Get("Content-Type"); contentType != "" && !strings.Contains(contentType, "text/html") {
		return nil, false
	}
	const maxBytes = 2 << 20
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	return body, err == nil && len(body) <= maxBytes
}

func earningsPeriodEnd(text, filedDate string) string {
	filed, err := time.Parse("2006-01-02", filedDate)
	if err != nil {
		return ""
	}
	for _, match := range earningsPeriodPattern.FindAllStringSubmatch(text, 8) {
		month := strings.ToLower(match[1])
		end, err := time.Parse("January 2 2006", strings.ToUpper(month[:1])+month[1:]+" "+match[2]+" "+match[3])
		if err == nil && !end.After(filed) && filed.Sub(end) <= 150*24*time.Hour {
			return end.Format("2006-01-02")
		}
	}
	return ""
}

func earningsExhibitDocument(raw []byte) string {
	root, err := html.Parse(strings.NewReader(string(raw)))
	if err != nil {
		return ""
	}
	var result string
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if result != "" {
			return
		}
		if node.Type == html.ElementNode && node.Data == "a" {
			var href string
			for _, attr := range node.Attr {
				if attr.Key == "href" {
					href = strings.TrimPrefix(attr.Val, "./")
				}
			}
			var label strings.Builder
			for child := node.FirstChild; child != nil; child = child.NextSibling {
				if child.Type == html.TextNode {
					label.WriteString(child.Data)
				}
			}
			name := strings.ToLower(label.String())
			if secExhibitNamePattern.MatchString(href) && (strings.Contains(name, "press release") || strings.Contains(name, "earnings release")) {
				result = href
				return
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)
	return result
}
