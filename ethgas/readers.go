package ethgas

import (
	"math/big"

	"github.com/0xsequence/ethkit/ethmonitor"
	"github.com/0xsequence/ethkit/go-ethereum/core/types"
)

type GasPriceReader func(block *ethmonitor.Block) []*big.Int

var CustomGasPriceBidReaders = map[uint64]GasPriceReader{}

var CustomPaidGasPriceReaders = map[uint64]GasPriceReader{
	42161:  arbitrumPaidGasPriceReader, // arbitrum one
	42170:  arbitrumPaidGasPriceReader, // arbitrum one testnet
	421611: arbitrumPaidGasPriceReader, // arbitrum nova
}

func DefaultGasPriceBidReader(block *ethmonitor.Block) []*big.Int {
	transactions := block.Transactions()
	prices := make([]*big.Int, 0, len(transactions))

	for _, transaction := range transactions {
		prices = append(prices, transaction.GasFeeCap())
	}

	return prices
}

func DefaultPaidGasPriceReader(block *ethmonitor.Block) []*big.Int {
	transactions := block.Transactions()
	prices := make([]*big.Int, 0, len(transactions))

	for _, transaction := range transactions {
		var price *big.Int

		switch transaction.Type() {
		case types.LegacyTxType:
			price = transaction.GasPrice()
		case types.AccessListTxType:
			price = transaction.GasPrice()
		case types.DynamicFeeTxType:
			price = new(big.Int).Add(block.BaseFee(), transaction.GasTipCap())
		}

		prices = append(prices, price)
	}

	return prices
}

func arbitrumPaidGasPriceReader(block *ethmonitor.Block) []*big.Int {
	transactions := block.Transactions()
	prices := make([]*big.Int, 0, len(transactions))

	for range transactions {
		prices = append(prices, block.BaseFee())
	}

	return prices
}
