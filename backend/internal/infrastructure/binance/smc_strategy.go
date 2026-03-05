package binance

import (
	"context"
	"fmt"
	"math"
	"strconv"
)

// SMCStrategy implements Smart Money Concept trading strategy
type SMCStrategy struct {
	client *Client
}

// NewSMCStrategy creates a new SMC strategy
func NewSMCStrategy(client *Client) *SMCStrategy {
	return &SMCStrategy{
		client: client,
	}
}

// MarketStructure represents market structure analysis
type MarketStructure struct {
	Trend           string  // "bullish", "bearish", "ranging"
	LastBOS         *BOS    // Last Break of Structure
	LastCHoCH       *CHoCH  // Last Change of Character
	HigherHighs     []float64
	HigherLows      []float64
	LowerHighs      []float64
	LowerLows       []float64
	CurrentHigh     float64
	CurrentLow      float64
}

// BOS represents Break of Structure
type BOS struct {
	Price     float64
	Timestamp int64
	Direction string // "bullish" or "bearish"
}

// CHoCH represents Change of Character
type CHoCH struct {
	Price     float64
	Timestamp int64
	FromTrend string
	ToTrend   string
}

// OrderBlock represents an order block
type OrderBlock struct {
	Type      string  // "bullish" or "bearish"
	High      float64
	Low       float64
	Open      float64
	Close     float64
	Volume    float64
	Timestamp int64
	Strength  float64 // 0-1, based on volume and price action
}

// FairValueGap represents a Fair Value Gap
type FVG struct {
	Type      string  // "bullish" or "bearish"
	Top       float64
	Bottom    float64
	Timestamp int64
	Filled    bool
}

// LiquidityZone represents a liquidity zone
type LiquidityZone struct {
	Type      string  // "buy_side" or "sell_side"
	Price     float64
	Strength  float64 // Based on volume and number of touches
	Timestamp int64
}

// SMCAnalysis represents complete SMC analysis
type SMCAnalysis struct {
	Symbol           string
	MarketStructure  *MarketStructure
	OrderBlocks      []OrderBlock
	FairValueGaps    []FVG
	LiquidityZones   []LiquidityZone
	PremiumZone      *PriceZone
	DiscountZone     *PriceZone
	Equilibrium      float64
	Recommendation   string
	Confidence       float64
}

// PriceZone represents a price zone
type PriceZone struct {
	High float64
	Low  float64
}

// TradeSignal represents a trading signal
type TradeSignal struct {
	Action      string  // "BUY", "SELL", "HOLD"
	Symbol      string
	EntryPrice  float64
	StopLoss    float64
	TakeProfit1 float64
	TakeProfit2 float64
	TakeProfit3 float64
	Quantity    float64
	Reasoning   string
	Confidence  float64
	RiskReward  float64
}

// AnalyzeMarket performs SMC analysis on a symbol
func (s *SMCStrategy) AnalyzeMarket(ctx context.Context, symbol string) (*SMCAnalysis, error) {
	// Get kline data (4H timeframe for structure, 15m for entry)
	klines4h, err := s.client.GetKlines(ctx, symbol, "4h", 100)
	if err != nil {
		return nil, fmt.Errorf("failed to get 4h klines: %w", err)
	}

	klines15m, err := s.client.GetKlines(ctx, symbol, "15m", 200)
	if err != nil {
		return nil, fmt.Errorf("failed to get 15m klines: %w", err)
	}

	// Analyze market structure
	marketStructure := s.analyzeMarketStructure(klines4h)

	// Identify order blocks
	orderBlocks := s.identifyOrderBlocks(klines15m, marketStructure)

	// Identify Fair Value Gaps
	fvgs := s.identifyFVGs(klines15m)

	// Identify liquidity zones
	liquidityZones := s.identifyLiquidityZones(klines4h)

	// Calculate premium/discount zones
	premiumZone, discountZone, equilibrium := s.calculatePremiumDiscount(klines4h)

	// Generate recommendation
	recommendation, confidence := s.generateRecommendation(
		marketStructure,
		orderBlocks,
		fvgs,
		liquidityZones,
		premiumZone,
		discountZone,
	)

	return &SMCAnalysis{
		Symbol:          symbol,
		MarketStructure: marketStructure,
		OrderBlocks:     orderBlocks,
		FairValueGaps:   fvgs,
		LiquidityZones:  liquidityZones,
		PremiumZone:     premiumZone,
		DiscountZone:    discountZone,
		Equilibrium:     equilibrium,
		Recommendation:  recommendation,
		Confidence:      confidence,
	}, nil
}

// GenerateTradeSignal generates a trade signal based on SMC analysis
func (s *SMCStrategy) GenerateTradeSignal(ctx context.Context, symbol string, analysis *SMCAnalysis) (*TradeSignal, error) {
	// Get current price
	ticker, err := s.client.Get24hrTicker(ctx, symbol)
	if err != nil {
		return nil, fmt.Errorf("failed to get ticker: %w", err)
	}

	currentPrice, _ := strconv.ParseFloat(ticker.LastPrice, 64)

	signal := &TradeSignal{
		Symbol:     symbol,
		Action:     "HOLD",
		Confidence: 0,
	}

	// Bullish scenario: Price in discount zone + bullish order block + bullish structure
	if analysis.MarketStructure.Trend == "bullish" {
		// Check if price is in discount zone
		if currentPrice <= analysis.Equilibrium {
			// Find nearest bullish order block
			var nearestOB *OrderBlock
			minDistance := math.MaxFloat64
			
			for i := range analysis.OrderBlocks {
				ob := &analysis.OrderBlocks[i]
				if ob.Type == "bullish" && ob.High >= currentPrice {
					distance := ob.Low - currentPrice
					if distance >= 0 && distance < minDistance {
						minDistance = distance
						nearestOB = ob
					}
				}
			}

			if nearestOB != nil {
				signal.Action = "BUY"
				signal.EntryPrice = nearestOB.Low
				signal.StopLoss = nearestOB.Low - (nearestOB.High-nearestOB.Low)*0.5
				
				// Multiple take profits
				riskAmount := signal.EntryPrice - signal.StopLoss
				signal.TakeProfit1 = signal.EntryPrice + riskAmount*1.5
				signal.TakeProfit2 = signal.EntryPrice + riskAmount*2.5
				signal.TakeProfit3 = signal.EntryPrice + riskAmount*4.0
				
				signal.RiskReward = (signal.TakeProfit2 - signal.EntryPrice) / riskAmount
				signal.Confidence = nearestOB.Strength * 0.7
				signal.Reasoning = fmt.Sprintf(
					"Bullish structure detected. Entry at order block %.2f, targeting premium zone with R:R %.2f",
					signal.EntryPrice,
					signal.RiskReward,
				)
			}
		}
	}

	// Bearish scenario: Price in premium zone + bearish order block + bearish structure
	if analysis.MarketStructure.Trend == "bearish" {
		if currentPrice >= analysis.Equilibrium {
			var nearestOB *OrderBlock
			minDistance := math.MaxFloat64
			
			for i := range analysis.OrderBlocks {
				ob := &analysis.OrderBlocks[i]
				if ob.Type == "bearish" && ob.Low <= currentPrice {
					distance := currentPrice - ob.High
					if distance >= 0 && distance < minDistance {
						minDistance = distance
						nearestOB = ob
					}
				}
			}

			if nearestOB != nil {
				signal.Action = "SELL"
				signal.EntryPrice = nearestOB.High
				signal.StopLoss = nearestOB.High + (nearestOB.High-nearestOB.Low)*0.5
				
				riskAmount := signal.StopLoss - signal.EntryPrice
				signal.TakeProfit1 = signal.EntryPrice - riskAmount*1.5
				signal.TakeProfit2 = signal.EntryPrice - riskAmount*2.5
				signal.TakeProfit3 = signal.EntryPrice - riskAmount*4.0
				
				signal.RiskReward = (signal.EntryPrice - signal.TakeProfit2) / riskAmount
				signal.Confidence = nearestOB.Strength * 0.7
				signal.Reasoning = fmt.Sprintf(
					"Bearish structure detected. Entry at order block %.2f, targeting discount zone with R:R %.2f",
					signal.EntryPrice,
					signal.RiskReward,
				)
			}
		}
	}

	return signal, nil
}

// analyzeMarketStructure analyzes market structure from klines
func (s *SMCStrategy) analyzeMarketStructure(klines []Kline) *MarketStructure {
	ms := &MarketStructure{
		Trend:       "ranging",
		HigherHighs: []float64{},
		HigherLows:  []float64{},
		LowerHighs:  []float64{},
		LowerLows:   []float64{},
	}

	if len(klines) < 3 {
		return ms
	}

	// Find swing highs and lows
	highs := []float64{}
	lows := []float64{}

	for i := 1; i < len(klines)-1; i++ {
		high, _ := strconv.ParseFloat(klines[i].High, 64)
		low, _ := strconv.ParseFloat(klines[i].Low, 64)
		
		prevHigh, _ := strconv.ParseFloat(klines[i-1].High, 64)
		prevLow, _ := strconv.ParseFloat(klines[i-1].Low, 64)
		nextHigh, _ := strconv.ParseFloat(klines[i+1].High, 64)
		nextLow, _ := strconv.ParseFloat(klines[i+1].Low, 64)

		// Swing high
		if high > prevHigh && high > nextHigh {
			highs = append(highs, high)
		}

		// Swing low
		if low < prevLow && low < nextLow {
			lows = append(lows, low)
		}
	}

	// Determine trend based on swing points
	if len(highs) >= 2 && len(lows) >= 2 {
		recentHighs := highs[max(0, len(highs)-3):]
		recentLows := lows[max(0, len(lows)-3):]

		// Check for higher highs and higher lows (bullish)
		isHigherHighs := true
		isHigherLows := true

		for i := 1; i < len(recentHighs); i++ {
			if recentHighs[i] <= recentHighs[i-1] {
				isHigherHighs = false
				break
			}
		}

		for i := 1; i < len(recentLows); i++ {
			if recentLows[i] <= recentLows[i-1] {
				isHigherLows = false
				break
			}
		}

		if isHigherHighs && isHigherLows {
			ms.Trend = "bullish"
			ms.HigherHighs = recentHighs
			ms.HigherLows = recentLows
		} else {
			// Check for lower highs and lower lows (bearish)
			isLowerHighs := true
			isLowerLows := true

			for i := 1; i < len(recentHighs); i++ {
				if recentHighs[i] >= recentHighs[i-1] {
					isLowerHighs = false
					break
				}
			}

			for i := 1; i < len(recentLows); i++ {
				if recentLows[i] >= recentLows[i-1] {
					isLowerLows = false
					break
				}
			}

			if isLowerHighs && isLowerLows {
				ms.Trend = "bearish"
				ms.LowerHighs = recentHighs
				ms.LowerLows = recentLows
			}
		}
	}

	// Set current high and low
	if len(klines) > 0 {
		ms.CurrentHigh, _ = strconv.ParseFloat(klines[len(klines)-1].High, 64)
		ms.CurrentLow, _ = strconv.ParseFloat(klines[len(klines)-1].Low, 64)
	}

	return ms
}

// identifyOrderBlocks identifies order blocks from klines
func (s *SMCStrategy) identifyOrderBlocks(klines []Kline, ms *MarketStructure) []OrderBlock {
	orderBlocks := []OrderBlock{}

	for i := 1; i < len(klines)-1; i++ {
		open, _ := strconv.ParseFloat(klines[i].Open, 64)
		close, _ := strconv.ParseFloat(klines[i].Close, 64)
		high, _ := strconv.ParseFloat(klines[i].High, 64)
		low, _ := strconv.ParseFloat(klines[i].Low, 64)
		volume, _ := strconv.ParseFloat(klines[i].Volume, 64)

		nextOpen, _ := strconv.ParseFloat(klines[i+1].Open, 64)
		nextClose, _ := strconv.ParseFloat(klines[i+1].Close, 64)

		// Bullish order block: bearish candle followed by strong bullish move
		if close < open && nextClose > nextOpen && (nextClose-nextOpen) > (open-close)*1.5 {
			strength := math.Min(volume/10000, 1.0) // Normalize volume
			orderBlocks = append(orderBlocks, OrderBlock{
				Type:      "bullish",
				High:      high,
				Low:       low,
				Open:      open,
				Close:     close,
				Volume:    volume,
				Timestamp: klines[i].OpenTime,
				Strength:  strength,
			})
		}

		// Bearish order block: bullish candle followed by strong bearish move
		if close > open && nextClose < nextOpen && (nextOpen-nextClose) > (close-open)*1.5 {
			strength := math.Min(volume/10000, 1.0)
			orderBlocks = append(orderBlocks, OrderBlock{
				Type:      "bearish",
				High:      high,
				Low:       low,
				Open:      open,
				Close:     close,
				Volume:    volume,
				Timestamp: klines[i].OpenTime,
				Strength:  strength,
			})
		}
	}

	// Keep only the most recent and strongest order blocks
	if len(orderBlocks) > 10 {
		orderBlocks = orderBlocks[len(orderBlocks)-10:]
	}

	return orderBlocks
}

// identifyFVGs identifies Fair Value Gaps
func (s *SMCStrategy) identifyFVGs(klines []Kline) []FVG {
	fvgs := []FVG{}

	for i := 1; i < len(klines)-1; i++ {
		prevHigh, _ := strconv.ParseFloat(klines[i-1].High, 64)
		prevLow, _ := strconv.ParseFloat(klines[i-1].Low, 64)
		currHigh, _ := strconv.ParseFloat(klines[i].High, 64)
		currLow, _ := strconv.ParseFloat(klines[i].Low, 64)
		nextHigh, _ := strconv.ParseFloat(klines[i+1].High, 64)
		nextLow, _ := strconv.ParseFloat(klines[i+1].Low, 64)

		// Bullish FVG: gap between prev high and next low
		if nextLow > prevHigh {
			fvgs = append(fvgs, FVG{
				Type:      "bullish",
				Top:       nextLow,
				Bottom:    prevHigh,
				Timestamp: klines[i].OpenTime,
				Filled:    false,
			})
		}

		// Bearish FVG: gap between prev low and next high
		if nextHigh < prevLow {
			fvgs = append(fvgs, FVG{
				Type:      "bearish",
				Top:       prevLow,
				Bottom:    nextHigh,
				Timestamp: klines[i].OpenTime,
				Filled:    false,
			})
		}

		// Check if current price filled any FVG
		for j := range fvgs {
			if !fvgs[j].Filled {
				if currLow <= fvgs[j].Top && currHigh >= fvgs[j].Bottom {
					fvgs[j].Filled = true
				}
			}
		}
	}

	// Keep only unfilled FVGs
	unfilledFVGs := []FVG{}
	for _, fvg := range fvgs {
		if !fvg.Filled {
			unfilledFVGs = append(unfilledFVGs, fvg)
		}
	}

	return unfilledFVGs
}

// identifyLiquidityZones identifies liquidity zones
func (s *SMCStrategy) identifyLiquidityZones(klines []Kline) []LiquidityZone {
	zones := []LiquidityZone{}

	// Find equal highs and lows (liquidity pools)
	tolerance := 0.001 // 0.1% tolerance

	highs := make(map[float64]int)
	lows := make(map[float64]int)

	for _, k := range klines {
		high, _ := strconv.ParseFloat(k.High, 64)
		low, _ := strconv.ParseFloat(k.Low, 64)

		// Group similar highs
		for h := range highs {
			if math.Abs(high-h)/h < tolerance {
				highs[h]++
				high = h
				break
			}
		}
		highs[high]++

		// Group similar lows
		for l := range lows {
			if math.Abs(low-l)/l < tolerance {
				lows[l]++
				low = l
				break
			}
		}
		lows[low]++
	}

	// Create liquidity zones from frequently touched levels
	for price, count := range highs {
		if count >= 2 {
			zones = append(zones, LiquidityZone{
				Type:     "buy_side",
				Price:    price,
				Strength: float64(count) / 10.0,
			})
		}
	}

	for price, count := range lows {
		if count >= 2 {
			zones = append(zones, LiquidityZone{
				Type:     "sell_side",
				Price:    price,
				Strength: float64(count) / 10.0,
			})
		}
	}

	return zones
}

// calculatePremiumDiscount calculates premium and discount zones
func (s *SMCStrategy) calculatePremiumDiscount(klines []Kline) (*PriceZone, *PriceZone, float64) {
	if len(klines) == 0 {
		return nil, nil, 0
	}

	// Find range high and low
	var rangeHigh, rangeLow float64
	for i, k := range klines {
		high, _ := strconv.ParseFloat(k.High, 64)
		low, _ := strconv.ParseFloat(k.Low, 64)

		if i == 0 {
			rangeHigh = high
			rangeLow = low
		} else {
			if high > rangeHigh {
				rangeHigh = high
			}
			if low < rangeLow {
				rangeLow = low
			}
		}
	}

	equilibrium := (rangeHigh + rangeLow) / 2
	range_ := rangeHigh - rangeLow

	// Premium zone: top 30% of range
	premiumZone := &PriceZone{
		High: rangeHigh,
		Low:  rangeHigh - range_*0.3,
	}

	// Discount zone: bottom 30% of range
	discountZone := &PriceZone{
		High: rangeLow + range_*0.3,
		Low:  rangeLow,
	}

	return premiumZone, discountZone, equilibrium
}

// generateRecommendation generates trading recommendation
func (s *SMCStrategy) generateRecommendation(
	ms *MarketStructure,
	obs []OrderBlock,
	fvgs []FVG,
	lzs []LiquidityZone,
	premium *PriceZone,
	discount *PriceZone,
) (string, float64) {
	confidence := 0.5

	if ms.Trend == "bullish" {
		confidence += 0.2
		if len(obs) > 0 {
			for _, ob := range obs {
				if ob.Type == "bullish" {
					confidence += 0.1
					break
				}
			}
		}
		return "Look for BUY opportunities in discount zone near bullish order blocks", confidence
	}

	if ms.Trend == "bearish" {
		confidence += 0.2
		if len(obs) > 0 {
			for _, ob := range obs {
				if ob.Type == "bearish" {
					confidence += 0.1
					break
				}
			}
		}
		return "Look for SELL opportunities in premium zone near bearish order blocks", confidence
	}

	return "Market is ranging, wait for clear structure", confidence
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
