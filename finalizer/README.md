# Finalizer

A wallet adapter for guaranteeing transaction inclusion on a specific chain.

This fixes "nonce too low" issues that can happen if reorgs occur or if you trust your node's reported nonces.

## Usage

### Create a mempool

For demonstration:

```go
mempool := finalizer.NewMemoryMempool[struct{}]()
```

Here `struct{}` can be any type for transaction metadata, data that gets persisted with the transaction, but not sent on chain.

For production, implement the `Mempool` interface and persist to a database instead.

### Create a chain provider using ethkit

For EIP-1559 chains:

```go
chain, err := finalizer.NewEthkitChain(finalizer.EthkitChainOptions{
    ChainID:     big.NewInt(1),
    IsEIP1559:   true,
    Provider:    provider,
    Monitor:     monitor,                // must be running
    GasGauge:    nil,                    // not used for EIP-1559 chains
    PriorityFee: big.NewInt(1000000000), // required for EIP-1559 chains
})
```

For non-EIP-1559 chains:

```go
chain, err := finalizer.NewEthkitChain(finalizer.EthkitChainOptions{
    ChainID:       big.NewInt(56),
    IsEIP1559:     false,
    Provider:      provider,
    Monitor:       monitor,                       // must be running
    GasGauge:      gasGauge,                      // required for non-EIP-1559 chains
    GasGaugeSpeed: finalizer.GasGaugeSpeedDefault // default = fast
    PriorityFee:   nil,                           // not used for non-EIP-1559 chains
})
```

### Create a finalizer for a specific wallet on a specific chain

```go
f, err := finalizer.NewFinalizer(finalizer.FinalizerOptions[struct{}]{
    Wallet:       wallet,
    Chain:        chain,
    Mempool:      mempool,
    Logger:       nil,
    PollInterval: 5 * time.Second,  // period between chain state checks
    PollTimeout:  4 * time.Second,  // time limit for operations while checking chain state
    RetryDelay:   24 * time.Second, // minimum time to wait before retrying a transaction
    FeeMargin:    25,               // percentage added on top of the estimated gas price
    PriceBump:    15,               // go-ethereum requires at least 10% by default
})
```

The finalizer has a blocking run loop that must be called for it to work:

```go
err := f.Run(ctx)
```

### Subscribe to mining and reorg events

```go
for event := range f.Subscribe(ctx) {
    if event.Added != nil {
        if event.Removed == nil {
            fmt.Println(
                "mined",
                event.Added.Hash(),
                event.Added.Metadata,
            )
        } else {
            fmt.Println(
                "reorged",
                event.Removed.Hash(),
                event.Removed.Metadata,
                "->",
                event.Added.Hash(),
                event.Added.Metadata,
            )
        }
    } else if event.Removed != nil {
        fmt.Println(
            "reorged",
            event.Removed.Hash(),
            event.Removed.Metadata,
        )
    }
}
```

### Send a transaction

```go
signed, err := f.Send(ctx, unsigned, struct{}{})
```

The `struct{}{}` argument here is the transaction's metadata.
The returned signed transaction may or may not be the final transaction included on chain.
