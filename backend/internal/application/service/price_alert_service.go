package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/mail"
	"strings"
	"sync"
	"time"

	"github.com/songhanxu/wiseinvest/internal/domain/model"
)

type PriceAlertStore interface {
	List(context.Context, uint) ([]model.PriceAlert, error)
	Settings(context.Context, uint) (model.PriceAlertSettings, error)
	SaveSettings(context.Context, *model.PriceAlertSettings) error
	Save(context.Context, *model.PriceAlert) error
	Remove(context.Context, uint, string) error
	Armed(context.Context) ([]model.PriceAlert, error)
	Checked(context.Context, model.PriceAlert, *float64, string) error
	Claim(context.Context, model.PriceAlert, float64) (bool, error)
	Delivered(context.Context, model.PriceAlert, error) error
	RecoverInterrupted(context.Context) error
}
type AlertQuote struct {
	HoldingID, AccountID, Name, Code, Currency string
	Price                                      float64
	Fresh                                      bool
}
type AlertQuoteReader func(context.Context, uint, string) ([]AlertQuote, error)
type AlertMailer interface {
	Configured() bool
	Send(context.Context, string, string, string, string) error
}
type PriceAlertService struct {
	Store  PriceAlertStore
	Quotes AlertQuoteReader
	Mailer AlertMailer
	runMu  sync.Mutex
}

func (s *PriceAlertService) MailerReady() bool { return s.Mailer != nil && s.Mailer.Configured() }
func validEmail(email string) bool {
	address, err := mail.ParseAddress(email)
	return err == nil && address.Address == email && len(email) <= 254 && !strings.ContainsAny(email, "\r\n")
}
func (s *PriceAlertService) SaveEmail(ctx context.Context, user uint, email string) error {
	email = strings.TrimSpace(email)
	if email != "" && !validEmail(email) {
		return errors.New("请输入有效的收件邮箱")
	}
	return s.Store.SaveSettings(ctx, &model.PriceAlertSettings{UserID: user, Email: email})
}
func (s *PriceAlertService) Save(ctx context.Context, user uint, account, holding string, target float64, direction string) (model.PriceAlert, error) {
	if user == 0 || len(holding) == 0 || len(holding) > 200 || target <= 0 || target > 1e15 || math.IsNaN(target) || math.IsInf(target, 0) {
		return model.PriceAlert{}, errors.New("请输入有效的期望价格")
	}
	if direction != "auto" && direction != "above" && direction != "below" {
		return model.PriceAlert{}, errors.New("请选择有效的提醒方向")
	}
	quotes, err := s.Quotes(ctx, user, account)
	if err != nil {
		return model.PriceAlert{}, errors.New("无法读取最新标的，请刷新列表并检查连接")
	}
	for _, q := range quotes {
		if q.HoldingID != holding {
			continue
		}
		if !q.Fresh || q.Price <= 0 || math.IsNaN(q.Price) || math.IsInf(q.Price, 0) || q.Currency == "" {
			return model.PriceAlert{}, errors.New("当前标的暂无有效行情，暂时无法设置到价提醒")
		}
		triggerMode := direction
		if direction == "auto" {
			direction = "above"
			if target < q.Price {
				direction = "below"
			}
		}
		row := model.PriceAlert{UserID: user, HoldingID: holding, AccountID: account, Name: q.Name, Code: q.Code, Currency: q.Currency, TargetPrice: target, Direction: direction, TriggerMode: triggerMode, Status: "armed", Revision: 1, LastPrice: &q.Price}
		if err = s.Store.Save(ctx, &row); err != nil {
			return row, errors.New("目标价保存失败，请重试（每个用户最多 100 项）")
		}
		return row, nil
	}
	return model.PriceAlert{}, errors.New("未找到属于当前用户的自选或持仓标的")
}

// Run checks fresh holdings only, and atomically claims each revision before
// sending. Failed or interrupted SMTP submissions require explicit re-arming
// rather than blindly retrying a message that might already have arrived.
func (s *PriceAlertService) Run(ctx context.Context) {
	if !s.runMu.TryLock() {
		return
	}
	defer s.runMu.Unlock()
	_ = s.Store.RecoverInterrupted(ctx)
	if !s.MailerReady() {
		return
	}
	alerts, err := s.Store.Armed(ctx)
	if err != nil {
		return
	}
	type group struct {
		user    uint
		account string
	}
	groups := map[group][]model.PriceAlert{}
	order := []group{}
	for _, row := range alerts {
		key := group{row.UserID, row.AccountID}
		if _, ok := groups[key]; !ok {
			order = append(order, key)
		}
		groups[key] = append(groups[key], row)
	}
	for _, key := range order {
		if ctx.Err() != nil {
			return
		}
		settings, err := s.Store.Settings(ctx, key.user)
		if err != nil || !validEmail(settings.Email) {
			continue
		}
		queryCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		quotes, err := s.Quotes(queryCtx, key.user, key.account)
		cancel()
		byID := map[string]AlertQuote{}
		for _, q := range quotes {
			byID[q.HoldingID] = q
		}
		for _, row := range groups[key] {
			q, ok := byID[row.HoldingID]
			if err != nil || !ok || !q.Fresh || q.Price <= 0 || math.IsNaN(q.Price) || math.IsInf(q.Price, 0) || q.Currency != row.Currency {
				_ = s.Store.Checked(ctx, row, nil, "当前标的或行情暂不可用；本次未触发")
				continue
			}
			hit := row.Direction == "above" && q.Price >= row.TargetPrice || row.Direction == "below" && q.Price <= row.TargetPrice
			if !hit {
				_ = s.Store.Checked(ctx, row, &q.Price, "")
				continue
			}
			claimed, err := s.Store.Claim(ctx, row, q.Price)
			if err != nil || !claimed {
				continue
			}
			direction := "涨至或高于"
			if row.Direction == "below" {
				direction = "跌至或低于"
			}
			source := row.AccountID
			if source == "watchlist" {
				source = "自选列表"
			}
			body := fmt.Sprintf("你在慧投设置的到价提醒已触发。\n\n标的：%s（%s）\n来源：%s\n条件：%s %.12g %s\n检测价格：%.12g %s\n检测时间：%s\n\n每次目标设置触发一次。你可以在对应列表中修改目标价，重新启用提醒。", row.Name, row.Code, source, direction, row.TargetPrice, row.Currency, q.Price, q.Currency, time.Now().In(time.FixedZone("Asia/Shanghai", 8*60*60)).Format("2006-01-02 15:04:05 -07:00"))
			sendCtx, sendCancel := context.WithTimeout(ctx, 25*time.Second)
			sendErr := s.Mailer.Send(sendCtx, settings.Email, "慧投到价提醒："+row.Name, body, fmt.Sprintf("price-alert-%d-%d", row.ID, row.Revision))
			sendCancel()
			// Persist the result even if shutdown cancelled the monitor's parent.
			saveCtx, saveCancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = s.Store.Delivered(saveCtx, row, sendErr)
			saveCancel()
		}
	}
}
func (s *PriceAlertService) Start() func() {
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			runCtx, stop := context.WithTimeout(ctx, 2*time.Minute)
			s.Run(runCtx)
			stop()
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return func() { cancel(); wg.Wait() }
}
