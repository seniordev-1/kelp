package kraken

import (
	"fmt"

	"github.com/lightyeario/kelp/support/exchange/assets"
	"github.com/lightyeario/kelp/support/exchange/dates"
	"github.com/lightyeario/kelp/support/exchange/number"
	"github.com/lightyeario/kelp/support/exchange/orderbook"
)

// GetOpenOrders impl.
func (k krakenExchange) GetOpenOrders() (map[assets.TradingPair][]orderbook.OpenOrder, error) {
	openOrdersResponse, e := k.api.OpenOrders(map[string]string{})
	if e != nil {
		return nil, e
	}

	m := map[assets.TradingPair][]orderbook.OpenOrder{}
	for ID, o := range openOrdersResponse.Open {
		// for some reason the open orders API returns the normal codes for assets
		pair, e := assets.TradingPairFromString(3, assets.Display, o.Description.AssetPair)
		if e != nil {
			return nil, e
		}
		if _, ok := m[*pair]; !ok {
			m[*pair] = []orderbook.OpenOrder{}
		}
		if _, ok := m[assets.TradingPair{Base: pair.Quote, Quote: pair.Base}]; ok {
			return nil, fmt.Errorf("open orders are listed with repeated base/quote pairs for %s", *pair)
		}

		m[*pair] = append(m[*pair], orderbook.OpenOrder{
			Order: orderbook.Order{
				Pair:        pair,
				OrderAction: orderbook.OrderActionFromString(o.Description.Type),
				OrderType:   orderbook.OrderTypeFromString(o.Description.OrderType),
				Price:       number.MustFromString(o.Description.PrimaryPrice, k.precision),
				Volume:      number.MustFromString(o.Volume, k.precision),
				Timestamp:   dates.MakeTimestamp(int64(o.OpenTime)),
			},
			ID:             ID,
			StartTime:      dates.MakeTimestamp(int64(o.StartTime)),
			ExpireTime:     dates.MakeTimestamp(int64(o.ExpireTime)),
			VolumeExecuted: number.FromFloat(o.VolumeExecuted, k.precision),
		})
	}
	return m, nil
}
