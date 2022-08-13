# bbgo-chaser

```yaml
exchangeStrategies:
  - on: binance
    chaser:
      symbol: BTCBUSD       # symbol of the market
      quantity: 0.1         # how much BTC to purchase per trade
      #gap: 5                # how much $ to place the order bellow / above current price
      profit: 0.5           # alternatively instead of setting gap you can set the desired profit per trade directly
      maxDistance: 5        # if the current price go above this value, the order is canceled and re-created with new calculated price
      maxNumberOfOrders: 4  # max number of orders to keep open at the same time
      waitAfter: 2          # once there are X orders pending to be sold, wait `waitMinutes` minutes before creating new orders
      waitMinutes: 1        # how long to wait before creating new orders once the `waitAfter` is reached
```