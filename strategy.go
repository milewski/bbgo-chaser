package chaser

import (
	"context"
	"fmt"
	"github.com/c9s/bbgo/pkg/fixedpoint"
	"github.com/sirupsen/logrus"
	"sync"
	"time"

	"github.com/c9s/bbgo/pkg/bbgo"
	"github.com/c9s/bbgo/pkg/types"
)

const ID = "chaser"

var log = logrus.WithField("strategy", ID)

func init() {
	bbgo.RegisterStrategy(ID, &Strategy{})
}

type Strategy struct {
	Notifiability *bbgo.Notifiability

	Symbol            string           `json:"symbol"`
	Quantity          fixedpoint.Value `json:"quantity"`
	Gap               fixedpoint.Value `json:"gap,omitempty"`
	MaxDistance       fixedpoint.Value `json:"maxDistance"`
	MaxNumberOfOrders int              `json:"maxNumberOfOrders"`
	WaitAfter         int              `json:"waitAfter"`
	WaitMinutes       int              `json:"waitMinutes"`
	Profit            fixedpoint.Value `json:"profit,omitempty"`

	Counter      int
	ActiveOrders *bbgo.ActiveOrderBook
	types.Market `json:"-" yaml:"-"`
}

func (s *Strategy) ID() string {
	return ID
}

func (s *Strategy) Initialize() error {
	return nil
}

func (s *Strategy) Validate() error {
	return nil
}

func (s *Strategy) Subscribe(session *bbgo.ExchangeSession) {

	session.Subscribe(types.KLineChannel, s.Symbol, types.SubscribeOptions{Interval: "1m"})

	if !bbgo.IsBackTesting {
		session.Subscribe(types.MarketTradeChannel, s.Symbol, types.SubscribeOptions{})
	}

}

func (s *Strategy) InstanceID() string {
	return fmt.Sprintf("%s-%s-%s-%d-%d-%d", ID, s.Symbol, s.MaxDistance, s.Quantity, s.Gap, s.MaxDistance)
}

func (s *Strategy) Run(ctx context.Context, _ bbgo.OrderExecutor, session *bbgo.ExchangeSession) error {

	s.ActiveOrders = bbgo.NewActiveOrderBook(s.Symbol)
	s.ActiveOrders.BindStream(session.UserDataStream)

	profitStats := types.NewProfitStats(s.Market)
	position := types.NewPositionFromMarket(s.Market)
	tradeStats := &types.TradeStats{}

	orderExecutor := bbgo.NewGeneralOrderExecutor(session, s.Symbol, ID, s.InstanceID(), position)
	orderExecutor.BindProfitStats(profitStats)
	orderExecutor.BindTradeStats(tradeStats)
	orderExecutor.Bind()

	blockNewOrders := false
	maximumAllowedOpenSellOrders := s.WaitAfter
	openSellOrdersCount := 0

	if s.Gap.IsZero() {
		s.Gap = s.Profit.Div(s.Quantity)
	}

	if s.Gap.IsZero() && s.Profit.IsZero() {
		log.Errorf("Please set either Gap or Profit.")
		return nil
	}

	log.Infof("Gap between buy and sell: %v %s", s.Gap, s.QuoteCurrency)

	session.UserDataStream.OnTradeUpdate(func(trade types.Trade) {
		profitStats.AddTrade(trade)
	})

	s.ActiveOrders.OnFilled(func(order types.Order) {

		if order.Side == types.SideTypeSell {

			log.Infof("Profit: %s %s", profitStats.AccumulatedNetProfit.String(), profitStats.QuoteCurrency)

			openSellOrdersCount = 0

			return

		}

		currentPrice, _ := session.LastPrice(s.Symbol)

		sellPrice := order.Price.Add(s.Gap)

		if currentPrice > sellPrice {
			println("Current price is higher than the order price... selling at current price instead.")
			sellPrice = currentPrice
		}

		createdOrders, err := orderExecutor.SubmitOrders(ctx, types.SubmitOrder{
			Symbol:   s.Symbol,
			Side:     types.SideTypeSell,
			Type:     types.OrderTypeLimit,
			Price:    sellPrice,
			Quantity: order.Quantity,
		})

		if err != nil {
			log.WithError(err).Errorf("Can not submit orders")
		}

		s.ActiveOrders.Add(createdOrders...)
		openSellOrdersCount++

	})

	session.MarketDataStream.OnKLine(func(kline types.KLine) {

		if blockNewOrders {
			return
		}

		currentPrice := kline.Close

		for _, order := range s.openBuyOrders() {

			/**
			 * If the current price speedup above the current order price + margin, cancel the order
			 */
			if currentPrice.Sub(order.Price.Add(s.Gap)) >= s.MaxDistance {
				orderExecutor.CancelOrders(context.Background(), order)
			}

		}

		/**
		 * If there are more orders than the max allowed number of do nothing...
		 */
		if s.ActiveOrders.NumOfOrders() >= s.MaxNumberOfOrders {
			return
		}

		/**
		 * If there are active buy orders already ... skip
		 */
		if s.hasAnyOpenBuyOrder() {
			return
		}

		if openSellOrdersCount >= maximumAllowedOpenSellOrders {

			log.Infof("Too many open sell orders... waiting %d minutes before resuming...", s.WaitMinutes)

			if blockNewOrders == false {

				time.AfterFunc(time.Duration(s.WaitMinutes)*time.Minute, func() {
					log.Infof("Lifting transaction ban...")
					blockNewOrders = false
					maximumAllowedOpenSellOrders += s.WaitAfter
				})

				blockNewOrders = true

			}

			return

		}

		createdOrders, error := orderExecutor.SubmitOrders(ctx, types.SubmitOrder{
			Symbol:   s.Symbol,
			Side:     types.SideTypeBuy,
			Type:     types.OrderTypeLimit,
			Price:    currentPrice.Sub(s.Gap),
			Quantity: s.Quantity,
		})

		if error != nil {
			log.WithError(error).Errorf("Can not submit orders...")
		}

		s.ActiveOrders.Add(createdOrders...)

	})

	bbgo.OnShutdown(func(ctx context.Context, wg *sync.WaitGroup) {

		defer wg.Done()
		s.ActiveOrders.Backup()

		log.Infof("Canceling active orders...")

		if err := session.Exchange.CancelOrders(context.Background(), s.openBuyOrders()...); err != nil {
			log.WithError(err).Errorf("Cancel order error")
		}

	})

	return nil
}

func (s *Strategy) hasAnyOpenBuyOrder() bool {
	return len(s.openBuyOrders()) > 0
}

func (s *Strategy) openBuyOrders() []types.Order {

	var orders []types.Order

	for _, order := range s.ActiveOrders.Orders() {
		if order.Side == types.SideTypeBuy {
			orders = append(orders, order)
		}
	}

	return orders

}

func (s *Strategy) openSellOrders() []types.Order {

	var orders []types.Order

	for _, order := range s.ActiveOrders.Orders() {
		if order.Side == types.SideTypeSell {
			orders = append(orders, order)
		}
	}

	return orders

}
