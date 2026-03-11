package scheduler

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/songhanxu/wiseinvest/internal/adapter/repository"
	"github.com/songhanxu/wiseinvest/internal/domain/agent"
	infraapns "github.com/songhanxu/wiseinvest/internal/infrastructure/apns"
	"github.com/songhanxu/wiseinvest/internal/infrastructure/logger"
	"github.com/songhanxu/wiseinvest/internal/infrastructure/wxwork"
)

const dailyReportPrompt = `请生成今日（%s）A股市场日报，内容包括：

1. **大盘走势总结**：今日沪深300、上证50、创业板指的涨跌幅及主要驱动因素
2. **热门板块分析**：今日涨幅前三的行业板块及背后逻辑
3. **个股推荐**：结合基本面与技术面，推荐3只值得关注的个股（附理由）
   - 限制条件：**只推荐主板股票**，且当前股价**不超过50元**
4. **风险提示**：当前市场需关注的主要风险点

请用简洁的 Markdown 格式输出，适合在企业微信中直接阅读。
**全文总字数控制在2500字以内，语言精炼，不要废话。**`

// DailyReportTask generates and dispatches the daily A-share market report.
type DailyReportTask struct {
	agentFactory    *agent.Factory
	wxClient        *wxwork.Client
	apnsClient      *infraapns.Client
	deviceTokenRepo *repository.DeviceTokenRepository
	log             *logger.Logger
}

// NewDailyReportTask creates a new DailyReportTask.
func NewDailyReportTask(
	agentFactory *agent.Factory,
	wxClient *wxwork.Client,
	apnsClient *infraapns.Client,
	deviceTokenRepo *repository.DeviceTokenRepository,
	log *logger.Logger,
) *DailyReportTask {
	return &DailyReportTask{
		agentFactory:    agentFactory,
		wxClient:        wxClient,
		apnsClient:      apnsClient,
		deviceTokenRepo: deviceTokenRepo,
		log:             log,
	}
}

// Run is called by the scheduler every day at 15:00 CST.
func (t *DailyReportTask) Run() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	today := time.Now().In(time.FixedZone("CST", 8*60*60)).Format("2006年01月02日")
	t.log.Infof("DailyReportTask: generating report for %s", today)

	// 1. Generate report via AShareAgent
	report, err := t.generateReport(ctx, today)
	if err != nil {
		t.log.Errorf("DailyReportTask: failed to generate report: %v", err)
		return
	}
	t.log.Infof("DailyReportTask: report generated (%d chars)", len(report))

	// 2. Push to WeChat Work robot
	t.sendWxWork(report, today)

	// 3. Push APNs alert to all registered iOS devices
	t.sendAPNs(today)
}

func (t *DailyReportTask) generateReport(ctx context.Context, today string) (string, error) {
	a, err := t.agentFactory.CreateAgent(agent.TypeAShare)
	if err != nil {
		return "", fmt.Errorf("create agent: %w", err)
	}

	prompt := fmt.Sprintf(dailyReportPrompt, today)
	req := agent.ProcessRequest{UserMessage: prompt}

	resp, err := a.Process(ctx, req)
	if err != nil {
		return "", fmt.Errorf("agent process: %w", err)
	}

	return resp.Content, nil
}

func (t *DailyReportTask) sendWxWork(report, today string) {
	if t.wxClient == nil || !t.wxClient.IsConfigured() {
		t.log.Warn("DailyReportTask: WeChat Work webhook not configured, skipping")
		return
	}

	// Prepend a header so the date is always visible at the top of the card.
	header := fmt.Sprintf("## 慧投 · A股日报 %s\n\n", today)
	content := header + report

	// Enforce 3000-character limit (WeChat Work markdown max is 4096 bytes).
	// We use rune count to correctly handle Chinese characters.
	const maxRunes = 3000
	runes := []rune(content)
	if len(runes) > maxRunes {
		content = string(runes[:maxRunes]) + "\n\n*（内容过长，已截断）*"
	}

	if err := t.wxClient.SendMarkdown(content); err != nil {
		t.log.Errorf("DailyReportTask: WeChat Work push failed: %v", err)
		return
	}
	t.log.Info("DailyReportTask: WeChat Work push succeeded")
}

func (t *DailyReportTask) sendAPNs(today string) {
	if t.apnsClient == nil || !t.apnsClient.IsConfigured() {
		t.log.Warn("DailyReportTask: APNs not configured, skipping")
		return
	}

	tokens, err := t.deviceTokenRepo.FindAll()
	if err != nil {
		t.log.Errorf("DailyReportTask: failed to fetch device tokens: %v", err)
		return
	}

	if len(tokens) == 0 {
		t.log.Info("DailyReportTask: no registered devices, skipping APNs")
		return
	}

	title := fmt.Sprintf("慧投 A股日报 · %s", today)
	body := "今日大盘走势、热门板块分析及个股推荐已出炉，点击查看！"

	var failures []string
	for _, dt := range tokens {
		if err := t.apnsClient.SendAlert(dt.Token, title, body); err != nil {
			t.log.Errorf("DailyReportTask: APNs push failed for token %s: %v", dt.Token[:min(8, len(dt.Token))], err)
			failures = append(failures, dt.Token)
		}
	}

	sent := len(tokens) - len(failures)
	t.log.Infof("DailyReportTask: APNs push complete — %d sent, %d failed", sent, len(failures))
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// formatWxMarkdown converts standard Markdown headings (### → **bold**) for
// WeChat Work, which only supports a limited subset of Markdown.
func formatWxMarkdown(content string) string {
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, "#### ") {
			lines[i] = "**" + strings.TrimPrefix(line, "#### ") + "**"
		} else if strings.HasPrefix(line, "### ") {
			lines[i] = "**" + strings.TrimPrefix(line, "### ") + "**"
		} else if strings.HasPrefix(line, "## ") {
			lines[i] = "# " + strings.TrimPrefix(line, "## ")
		}
	}
	return strings.Join(lines, "\n")
}

// init ensures formatWxMarkdown is referenced to satisfy the compiler.
var _ = formatWxMarkdown
