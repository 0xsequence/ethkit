package ethcoder_test

import (
	"math/big"
	"testing"

	"github.com/0xsequence/ethkit/ethcoder"
	"github.com/0xsequence/ethkit/go-ethereum/common"
	"github.com/0xsequence/ethkit/go-ethereum/common/hexutil"
	"github.com/0xsequence/ethkit/go-ethereum/core/types"
	"github.com/stretchr/testify/require"
)

func TestEventTopicHash(t *testing.T) {
	in := []struct {
		event string
	}{
		{"Transfer(address indexed from, address indexed to, uint256 value)"},
		{"Transfer(address from, address indexed to, uint256 value)"},
		{"Transfer(address, address , uint256 )"},
		{"Transfer   (address from, address , uint256 value)"},
	}

	for _, x := range in {
		topicHash, eventSig, err := ethcoder.EventTopicHash(x.event)
		require.NoError(t, err)
		require.Equal(t, "0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef", topicHash.String())
		require.Equal(t, "Transfer(address,address,uint256)", eventSig)
	}

	for _, x := range in {
		eventDef, err := ethcoder.ParseEventDef(x.event)
		require.NoError(t, err)
		require.Equal(t, "0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef", eventDef.TopicHash)
		require.Equal(t, "Transfer", eventDef.Name)
		require.Equal(t, "Transfer(address,address,uint256)", eventDef.Sig)
		require.Equal(t, []string{"address", "address", "uint256"}, eventDef.ArgTypes)
		// require.Equal(t, []string{"from", "to", "value"}, eventDef.ArgNames)
	}
}

func TestDecodeTransactionLogByContractABIJSON(t *testing.T) {
	logTopics := []string{
		"0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef",
		"0x00000000000000000000000037af74b8096a6fd85bc4a36653a60b8d673baefc",
		"0x000000000000000000000000ba12222222228d8ba445958a75a0704d566bf2c8",
	}
	logData := "0x0000000000000000000000000000000000000000000000000000000002b46676"

	txnLog := types.Log{}
	txnLog.Topics = []common.Hash{}

	for _, topic := range logTopics {
		txnLog.Topics = append(txnLog.Topics, common.HexToHash(topic))
	}
	txnLog.Data, _ = hexutil.Decode(logData)

	var erc20ABI = "[{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"address\",\"name\":\"owner\",\"type\":\"address\"},{\"indexed\":true,\"internalType\":\"address\",\"name\":\"spender\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"value\",\"type\":\"uint256\"}],\"name\":\"Approval\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"address\",\"name\":\"from\",\"type\":\"address\"},{\"indexed\":true,\"internalType\":\"address\",\"name\":\"to\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"value\",\"type\":\"uint256\"}],\"name\":\"Transfer\",\"type\":\"event\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"owner\",\"type\":\"address\"},{\"internalType\":\"address\",\"name\":\"spender\",\"type\":\"address\"}],\"name\":\"allowance\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"spender\",\"type\":\"address\"},{\"internalType\":\"uint256\",\"name\":\"amount\",\"type\":\"uint256\"}],\"name\":\"approve\",\"outputs\":[{\"internalType\":\"bool\",\"name\":\"\",\"type\":\"bool\"}],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"account\",\"type\":\"address\"}],\"name\":\"balanceOf\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"totalSupply\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"recipient\",\"type\":\"address\"},{\"internalType\":\"uint256\",\"name\":\"amount\",\"type\":\"uint256\"}],\"name\":\"transfer\",\"outputs\":[{\"internalType\":\"bool\",\"name\":\"\",\"type\":\"bool\"}],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"sender\",\"type\":\"address\"},{\"internalType\":\"address\",\"name\":\"recipient\",\"type\":\"address\"},{\"internalType\":\"uint256\",\"name\":\"amount\",\"type\":\"uint256\"}],\"name\":\"transferFrom\",\"outputs\":[{\"internalType\":\"bool\",\"name\":\"\",\"type\":\"bool\"}],\"stateMutability\":\"nonpayable\",\"type\":\"function\"}]"

	eventDef, eventValues, ok, err := ethcoder.DecodeTransactionLogByContractABIJSON(txnLog, erc20ABI)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef", eventDef.TopicHash)
	require.Equal(t, "Transfer", eventDef.Name)
	require.Equal(t, "Transfer(address,address,uint256)", eventDef.Sig)
	require.Equal(t, []string{"from", "to", "value"}, eventDef.ArgNames)

	require.Equal(t, common.HexToAddress("0x37af74b8096a6fd85bc4a36653a60b8d673baefc"), eventValues[0])
	require.Equal(t, common.HexToAddress("0xba12222222228d8ba445958a75a0704d566bf2c8"), eventValues[1])
	require.Equal(t, big.NewInt(45377142), eventValues[2])
	// spew.Dump(eventValues)
}

func TestDecodeTransactionLogByEventSig1(t *testing.T) {
	logTopics := []string{
		"0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef",
		"0x00000000000000000000000037af74b8096a6fd85bc4a36653a60b8d673baefc",
		"0x000000000000000000000000ba12222222228d8ba445958a75a0704d566bf2c8",
	}
	logData := "0x0000000000000000000000000000000000000000000000000000000002b46676"

	txnLog := types.Log{}
	txnLog.Topics = []common.Hash{}

	for _, topic := range logTopics {
		txnLog.Topics = append(txnLog.Topics, common.HexToHash(topic))
	}
	txnLog.Data, _ = hexutil.Decode(logData)

	var eventSig = "Transfer(address,address,uint256)"

	eventDef, eventValues, ok, err := ethcoder.DecodeTransactionLogByEventSig(txnLog, eventSig, false)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef", eventDef.TopicHash)
	require.Equal(t, "Transfer", eventDef.Name)
	require.Equal(t, "Transfer(address,address,uint256)", eventDef.Sig)
	require.Equal(t, []string{"", "", ""}, eventDef.ArgNames)
	require.Equal(t, common.HexToAddress("0x37af74b8096a6fd85bc4a36653a60b8d673baefc"), eventValues[0])
	require.Equal(t, common.HexToAddress("0xba12222222228d8ba445958a75a0704d566bf2c8"), eventValues[1])
	require.Equal(t, big.NewInt(45377142), eventValues[2])
	// spew.Dump(eventValues)

	eventDef, eventValues, ok, err = ethcoder.DecodeTransactionLogByEventSig(txnLog, eventSig, true)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef", eventDef.TopicHash)
	require.Equal(t, "Transfer", eventDef.Name)
	require.Equal(t, "Transfer(address,address,uint256)", eventDef.Sig)
	require.Equal(t, []string{"", "", ""}, eventDef.ArgNames)
	require.Equal(t, "0x37af74b8096a6fd85bc4a36653a60b8d673baefc", eventValues[0])
	require.Equal(t, "0xba12222222228d8ba445958a75a0704d566bf2c8", eventValues[1])
	require.Equal(t, "0x0000000000000000000000000000000000000000000000000000000002b46676", eventValues[2])
	// spew.Dump(eventValues)
}

func TestDecodeTransactionLogByEventSig2(t *testing.T) {
	logTopics := []string{
		"0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef",
		"0x0000000000000000000000000000000000000000000000000000000000000000",
		"0x0000000000000000000000001a05955180488cb07db065d174b44df9aeb0fdb1",
		"0x000000000000000000000000000000000000000000000000000000000004d771",
	}
	logData := "0x"

	txnLog := types.Log{}
	txnLog.Topics = []common.Hash{}

	for _, topic := range logTopics {
		txnLog.Topics = append(txnLog.Topics, common.HexToHash(topic))
	}
	txnLog.Data, _ = hexutil.Decode(logData)

	var eventSig = "Transfer(address,address,uint256)"

	eventDef, eventValues, ok, err := ethcoder.DecodeTransactionLogByEventSig(txnLog, eventSig, true)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef", eventDef.TopicHash)
	require.Equal(t, "Transfer", eventDef.Name)
	require.Equal(t, "Transfer(address,address,uint256)", eventDef.Sig)
	require.Equal(t, []string{"", "", ""}, eventDef.ArgNames)
	require.Equal(t, "0x0000000000000000000000000000000000000000", eventValues[0])
	require.Equal(t, "0x1a05955180488cb07db065d174b44df9aeb0fdb1", eventValues[1])
	require.Equal(t, "0x000000000000000000000000000000000000000000000000000000000004d771", eventValues[2])
	// spew.Dump(eventValues)
}

func TestDecodeTransactionLogByEventSig3(t *testing.T) {
	logTopics := []string{
		"0xc3d58168c5ae7397731d063d5bbf3d657854427343f4c083240f7aacaa2d0f62",
		"0x000000000000000000000000d91e80cf2e7be2e162c6513ced06f1dd0da35296",
		"0x0000000000000000000000004ce73141dbfce41e65db3723e31059a730f0abad",
		"0x000000000000000000000000c5d563a36ae78145c45a50134d48a1215220f80a",
	}
	logData := "0xa08c15ba3595b44412ba290036a59015de859621fede8d4f2b9965f9956beca30000000000000000000000000000000000000000000000000000000000501bd0"

	txnLog := types.Log{}
	txnLog.Topics = []common.Hash{}

	for _, topic := range logTopics {
		txnLog.Topics = append(txnLog.Topics, common.HexToHash(topic))
	}
	txnLog.Data, _ = hexutil.Decode(logData)

	var eventSig = "TransferSingle(address,address,address,uint256,uint256)"

	eventDef, eventValues, ok, err := ethcoder.DecodeTransactionLogByEventSig(txnLog, eventSig, true)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "0xc3d58168c5ae7397731d063d5bbf3d657854427343f4c083240f7aacaa2d0f62", eventDef.TopicHash)
	require.Equal(t, "TransferSingle", eventDef.Name)
	require.Equal(t, "TransferSingle(address,address,address,uint256,uint256)", eventDef.Sig)
	require.Equal(t, []string{"", "", "", "", ""}, eventDef.ArgNames)
	require.Equal(t, "0xd91e80cf2e7be2e162c6513ced06f1dd0da35296", eventValues[0])
	require.Equal(t, "0x4ce73141dbfce41e65db3723e31059a730f0abad", eventValues[1])
	require.Equal(t, "0xc5d563a36ae78145c45a50134d48a1215220f80a", eventValues[2])
	require.Equal(t, "0xa08c15ba3595b44412ba290036a59015de859621fede8d4f2b9965f9956beca3", eventValues[3])
	require.Equal(t, "0x0000000000000000000000000000000000000000000000000000000000501bd0", eventValues[4])
	// spew.Dump(eventValues)
}

func TestDecodeTransactionLogByEventSig4(t *testing.T) {
	logTopics := []string{
		"0xc42079f94a6350d7e6235f29174924f928cc2ac818eb64fed8004e115fbcca67",
		"0x000000000000000000000000ec7be89e9d109e7e3fec59c222cf297125fefda2",
		"0x000000000000000000000000ec7be89e9d109e7e3fec59c222cf297125fefda2",
	}
	logData := "0x0000000000000000000000000000000000000000000000100f4b6d6675790000fffffffffffffffffffffffffffffffffffffffffffffffffffffffff7271967000000000000000000000000000000000000000000000be0c951878517d91842000000000000000000000000000000000000000000000000233dca2396037eaefffffffffffffffffffffffffffffffffffffffffffffffffffffffffffbada1"

	txnLog := types.Log{}
	txnLog.Topics = []common.Hash{}

	for _, topic := range logTopics {
		txnLog.Topics = append(txnLog.Topics, common.HexToHash(topic))
	}
	txnLog.Data, _ = hexutil.Decode(logData)

	var eventSig = "Swap (address sender, address recipient, int256 amount0, int256 amount1, uint160 sqrtPriceX96, uint128 liquidity, int24 tick)"

	eventDef, eventValues, ok, err := ethcoder.DecodeTransactionLogByEventSig(txnLog, eventSig, true) // use generics...?
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "0xc42079f94a6350d7e6235f29174924f928cc2ac818eb64fed8004e115fbcca67", eventDef.TopicHash)
	require.Equal(t, "Swap", eventDef.Name)
	require.Equal(t, "Swap(address,address,int256,int256,uint160,uint128,int24)", eventDef.Sig)
	require.Equal(t, []string{"sender", "recipient", "amount0", "amount1", "sqrtPriceX96", "liquidity", "tick"}, eventDef.ArgNames)
	require.Equal(t, "0xec7be89e9d109e7e3fec59c222cf297125fefda2", eventValues[0])
	require.Equal(t, "0xec7be89e9d109e7e3fec59c222cf297125fefda2", eventValues[1])
	require.Equal(t, "0x0000000000000000000000000000000000000000000000100f4b6d6675790000", eventValues[2])
	require.Equal(t, "0xfffffffffffffffffffffffffffffffffffffffffffffffffffffffff7271967", eventValues[3])
	require.Equal(t, "0x000000000000000000000000000000000000000000000be0c951878517d91842", eventValues[4])
	require.Equal(t, "0x000000000000000000000000000000000000000000000000233dca2396037eae", eventValues[5])
	require.Equal(t, "0xfffffffffffffffffffffffffffffffffffffffffffffffffffffffffffbada1", eventValues[6])
	// spew.Dump(eventValues)

	dataCheck := ""
	for i := 2; i < len(eventValues); i++ {
		v := eventValues[i]
		s := v.(string)
		dataCheck += s[2:]
	}
	dataCheck = "0x" + dataCheck
	require.Equal(t, logData, dataCheck)
}
