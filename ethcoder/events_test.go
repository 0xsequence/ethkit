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

func TestEventTopicHash1(t *testing.T) {
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

func TestEventTopicHash2(t *testing.T) {
	in := []struct {
		event string
	}{
		{"ERC721SellOrderFilled(bytes32,address,address,uint256,address,uint256,(address,uint256)[],address,uint256)"},
		{"ERC721SellOrderFilled(bytes32 indexed ok,address ok,address ok,uint256 ok,address ok,uint256 ok,(address,uint256)[] ok,address ok,uint256 ok)"},
		{"ERC721SellOrderFilled(bytes32 num,   address from,address to,   uint256 num,address rec,uint256 n,   (   address op,   uint256 val   )[] array,   address,   uint256   )  "},
	}

	for _, x := range in {
		topicHash, eventSig, err := ethcoder.EventTopicHash(x.event)
		require.NoError(t, err)
		require.Equal(t, "0x9c248aa1a265aa616f707b979d57f4529bb63a4fc34dc7fc61fdddc18410f74e", topicHash.String())
		require.Equal(t, "ERC721SellOrderFilled(bytes32,address,address,uint256,address,uint256,(address,uint256)[],address,uint256)", eventSig)
	}

	for _, x := range in {
		eventDef, err := ethcoder.ParseEventDef(x.event)
		require.NoError(t, err)
		require.Equal(t, "0x9c248aa1a265aa616f707b979d57f4529bb63a4fc34dc7fc61fdddc18410f74e", eventDef.TopicHash)
		require.Equal(t, "ERC721SellOrderFilled", eventDef.Name)
		require.Equal(t, "ERC721SellOrderFilled(bytes32,address,address,uint256,address,uint256,(address,uint256)[],address,uint256)", eventDef.Sig)
		require.Equal(t, []string{"bytes32", "address", "address", "uint256", "address", "uint256", "(address,uint256)[]", "address", "uint256"}, eventDef.ArgTypes)
	}
}

func TestEventTopicHash3(t *testing.T) {
	in := []struct {
		event string
	}{
		{"NftItemCreated(uint256,uint32,address,bool,uint32,address,address[],uint32[])"},
		{"NftItemCreated(uint256 num,uint32 val,address from,bool flag,     uint32 param,    address from,     address[] friends,     uint32[] nums   )"},
		{"NftItemCreated(uint256,uint32,address op,bool,uint32,address,address[],uint32[]   nums)"},
	}

	for _, x := range in {
		topicHash, eventSig, err := ethcoder.EventTopicHash(x.event)
		require.NoError(t, err)
		require.Equal(t, "0x041b7d65461f6f51e8fd92623a3848b22ce7077c215e4ea064d790e9efa08b8f", topicHash.String())
		require.Equal(t, "NftItemCreated(uint256,uint32,address,bool,uint32,address,address[],uint32[])", eventSig)
	}

	for _, x := range in {
		eventDef, err := ethcoder.ParseEventDef(x.event)
		require.NoError(t, err)
		require.Equal(t, "0x041b7d65461f6f51e8fd92623a3848b22ce7077c215e4ea064d790e9efa08b8f", eventDef.TopicHash)
		require.Equal(t, "NftItemCreated", eventDef.Name)
		require.Equal(t, "NftItemCreated(uint256,uint32,address,bool,uint32,address,address[],uint32[])", eventDef.Sig)
		require.Equal(t, []string{"uint256", "uint32", "address", "bool", "uint32", "address", "address[]", "uint32[]"}, eventDef.ArgTypes)
	}
}

func TestValidateEventSig(t *testing.T) {
	valid, err := ethcoder.ValidateEventSig("Approve(address,address,uint256)")
	require.NoError(t, err)
	require.True(t, valid)

	valid, err = ethcoder.ValidateEventSig("Approve(address,address,uinBREAKt25627fff)")
	require.Error(t, err)
	require.False(t, valid)

	// valid, err = ethcoder.ValidateEventSig("Approve(address,address,uint2ffff)")
	// require.Error(t, err)
	// require.False(t, valid)
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
	require.Equal(t, []string{"address", "address", "uint256"}, eventDef.ArgTypes)
	require.Equal(t, []bool{true, true, false}, eventDef.ArgIndexed)
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

	var eventSig = "Transfer(address indexed,address indexed,uint256)"

	eventDef, eventValues, ok, err := ethcoder.DecodeTransactionLogByEventSig(txnLog, eventSig)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef", eventDef.TopicHash)
	require.Equal(t, "Transfer", eventDef.Name)
	require.Equal(t, "Transfer(address,address,uint256)", eventDef.Sig)
	require.Equal(t, []string{"address", "address", "uint256"}, eventDef.ArgTypes)
	require.Equal(t, []bool{true, true, false}, eventDef.ArgIndexed)
	require.Equal(t, []string{"arg1", "arg2", "arg3"}, eventDef.ArgNames)
	require.Equal(t, common.HexToAddress("0x37af74b8096a6fd85bc4a36653a60b8d673baefc"), eventValues[0])
	require.Equal(t, common.HexToAddress("0xba12222222228d8ba445958a75a0704d566bf2c8"), eventValues[1])
	require.Equal(t, big.NewInt(45377142), eventValues[2])
	// spew.Dump(eventValues)

	eventDef, eventHexValues, ok, err := ethcoder.DecodeTransactionLogByEventSigAsHex(txnLog, eventSig)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef", eventDef.TopicHash)
	require.Equal(t, "Transfer", eventDef.Name)
	require.Equal(t, "Transfer(address,address,uint256)", eventDef.Sig)
	require.Equal(t, []string{"address", "address", "uint256"}, eventDef.ArgTypes)
	require.Equal(t, []bool{true, true, false}, eventDef.ArgIndexed)
	require.Equal(t, []string{"arg1", "arg2", "arg3"}, eventDef.ArgNames)
	require.Equal(t, "0x37af74b8096a6fd85bc4a36653a60b8d673baefc", eventHexValues[0])
	require.Equal(t, "0xba12222222228d8ba445958a75a0704d566bf2c8", eventHexValues[1])
	require.Equal(t, "0x0000000000000000000000000000000000000000000000000000000002b46676", eventHexValues[2])
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

	var eventSig = "Transfer(address indexed from,address indexed to,uint256 indexed amount)"

	eventDef, eventHexValues, ok, err := ethcoder.DecodeTransactionLogByEventSigAsHex(txnLog, eventSig)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef", eventDef.TopicHash)
	require.Equal(t, "Transfer", eventDef.Name)
	require.Equal(t, "Transfer(address,address,uint256)", eventDef.Sig)
	require.Equal(t, []string{"address", "address", "uint256"}, eventDef.ArgTypes)
	require.Equal(t, []bool{true, true, true}, eventDef.ArgIndexed)
	require.Equal(t, []string{"from", "to", "amount"}, eventDef.ArgNames)
	require.Equal(t, "0x0000000000000000000000000000000000000000", eventHexValues[0])
	require.Equal(t, "0x1a05955180488cb07db065d174b44df9aeb0fdb1", eventHexValues[1])
	require.Equal(t, "0x000000000000000000000000000000000000000000000000000000000004d771", eventHexValues[2])
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

	var eventSig = "TransferSingle(address indexed,address indexed,address indexed,uint256,uint256)"

	eventDef, eventHexValues, ok, err := ethcoder.DecodeTransactionLogByEventSigAsHex(txnLog, eventSig)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "0xc3d58168c5ae7397731d063d5bbf3d657854427343f4c083240f7aacaa2d0f62", eventDef.TopicHash)
	require.Equal(t, "TransferSingle", eventDef.Name)
	require.Equal(t, "TransferSingle(address,address,address,uint256,uint256)", eventDef.Sig)
	require.Equal(t, []string{"address", "address", "address", "uint256", "uint256"}, eventDef.ArgTypes)
	require.Equal(t, []bool{true, true, true, false, false}, eventDef.ArgIndexed)
	require.Equal(t, []string{"arg1", "arg2", "arg3", "arg4", "arg5"}, eventDef.ArgNames)
	require.Equal(t, "0xd91e80cf2e7be2e162c6513ced06f1dd0da35296", eventHexValues[0])
	require.Equal(t, "0x4ce73141dbfce41e65db3723e31059a730f0abad", eventHexValues[1])
	require.Equal(t, "0xc5d563a36ae78145c45a50134d48a1215220f80a", eventHexValues[2])
	require.Equal(t, "0xa08c15ba3595b44412ba290036a59015de859621fede8d4f2b9965f9956beca3", eventHexValues[3])
	require.Equal(t, "0x0000000000000000000000000000000000000000000000000000000000501bd0", eventHexValues[4])
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

	var eventSig = "Swap (address indexed sender, address indexed recipient, int256 amount0, int256 amount1, uint160 sqrtPriceX96, uint128 liquidity, int24 tick)"

	eventDef, eventHexValues, ok, err := ethcoder.DecodeTransactionLogByEventSigAsHex(txnLog, eventSig)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "0xc42079f94a6350d7e6235f29174924f928cc2ac818eb64fed8004e115fbcca67", eventDef.TopicHash)
	require.Equal(t, "Swap", eventDef.Name)
	require.Equal(t, "Swap(address,address,int256,int256,uint160,uint128,int24)", eventDef.Sig)
	require.Equal(t, []string{"address", "address", "int256", "int256", "uint160", "uint128", "int24"}, eventDef.ArgTypes)
	require.Equal(t, []bool{true, true, false, false, false, false, false}, eventDef.ArgIndexed)
	require.Equal(t, []string{"sender", "recipient", "amount0", "amount1", "sqrtPriceX96", "liquidity", "tick"}, eventDef.ArgNames)
	require.Equal(t, "0xec7be89e9d109e7e3fec59c222cf297125fefda2", eventHexValues[0])
	require.Equal(t, "0xec7be89e9d109e7e3fec59c222cf297125fefda2", eventHexValues[1])
	require.Equal(t, "0x0000000000000000000000000000000000000000000000100f4b6d6675790000", eventHexValues[2])
	require.Equal(t, "0xfffffffffffffffffffffffffffffffffffffffffffffffffffffffff7271967", eventHexValues[3])
	require.Equal(t, "0x000000000000000000000000000000000000000000000be0c951878517d91842", eventHexValues[4])
	require.Equal(t, "0x000000000000000000000000000000000000000000000000233dca2396037eae", eventHexValues[5])
	require.Equal(t, "0xfffffffffffffffffffffffffffffffffffffffffffffffffffffffffffbada1", eventHexValues[6])
	// spew.Dump(eventValues)

	dataCheck := ""
	for i := 2; i < len(eventHexValues); i++ {
		s := eventHexValues[i]
		dataCheck += s[2:]
	}
	dataCheck = "0x" + dataCheck
	require.Equal(t, logData, dataCheck)
}

func TestDecodeTransactionLogByEventSig5(t *testing.T) {
	logTopics := []string{
		"0x9c248aa1a265aa616f707b979d57f4529bb63a4fc34dc7fc61fdddc18410f74e",
	}
	logData := "0x714e9ffe0a4ab971954fe26f6021c8a9bb92e332a93d63b039f16b58be2eb61c0000000000000000000000004efca6d4d5f355ca7955d0024b5b35ae5aadf372000000000000000000000000cee077ea790a32927c49c6294c392404d0d31c0a0000000000000000000000000000000000000000000000000000000000000005000000000000000000000000eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee00000000000000000000000000000000000000000000000004fefa17b724000000000000000000000000000000000000000000000000000000000000000001200000000000000000000000005d666f215a85b87cb042d59662a7ecd2c8cc44e6000000000000000000000000000000000000000000000000000000000231ce230000000000000000000000000000000000000000000000000000000000000001000000000000000000000000d207842d66b715df6ea08cf52f025b9e2ed287880000000000000000000000000000000000000000000000000019945ca2620000"

	txnLog := types.Log{}
	txnLog.Topics = []common.Hash{}

	for _, topic := range logTopics {
		txnLog.Topics = append(txnLog.Topics, common.HexToHash(topic))
	}
	txnLog.Data, _ = hexutil.Decode(logData)

	var eventSig = "ERC721SellOrderFilled(bytes32,address,address,uint256,address,uint256,(address,uint256)[],address,uint256)"

	eventDef, eventHexValues, ok, err := ethcoder.DecodeTransactionLogByEventSigAsHex(txnLog, eventSig)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "0x9c248aa1a265aa616f707b979d57f4529bb63a4fc34dc7fc61fdddc18410f74e", eventDef.TopicHash)
	require.Equal(t, "ERC721SellOrderFilled", eventDef.Name)
	require.Equal(t, "ERC721SellOrderFilled(bytes32,address,address,uint256,address,uint256,(address,uint256)[],address,uint256)", eventDef.Sig)
	require.Equal(t, []string{"bytes32", "address", "address", "uint256", "address", "uint256", "(address,uint256)[]", "address", "uint256"}, eventDef.ArgTypes)
	require.Equal(t, []bool{false, false, false, false, false, false, false, false, false}, eventDef.ArgIndexed)
	require.Equal(t, []string{"arg1", "arg2", "arg3", "arg4", "arg5", "arg6", "arg7", "arg8", "arg9"}, eventDef.ArgNames)
	require.Equal(t, 9, len(eventHexValues))

	require.Equal(t, "0x714e9ffe0a4ab971954fe26f6021c8a9bb92e332a93d63b039f16b58be2eb61c", eventHexValues[0])
	require.Equal(t, "0x0000000000000000000000004efca6d4d5f355ca7955d0024b5b35ae5aadf372", eventHexValues[1])
	require.Equal(t, "0x000000000000000000000000cee077ea790a32927c49c6294c392404d0d31c0a", eventHexValues[2])
	require.Equal(t, "0x0000000000000000000000000000000000000000000000000000000000000005", eventHexValues[3])
	require.Equal(t, "0x000000000000000000000000eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee", eventHexValues[4])
	require.Equal(t, "0x00000000000000000000000000000000000000000000000004fefa17b7240000", eventHexValues[5])
	require.Equal(t, "0x00000000000000000000000000000000000000000000000000000000000000200000000000000000000000000000000000000000000000000000000000000001000000000000000000000000d207842d66b715df6ea08cf52f025b9e2ed287880000000000000000000000000000000000000000000000000019945ca2620000", eventHexValues[6])
	require.Equal(t, "0x0000000000000000000000005d666f215a85b87cb042d59662a7ecd2c8cc44e6", eventHexValues[7])
	require.Equal(t, "0x000000000000000000000000000000000000000000000000000000000231ce23", eventHexValues[8])

	// spew.Dump(eventValues)

	// is this correct...? 12 values .. looks weird
	// expectedValues := []string{
	// 	"0x714e9ffe0a4ab971954fe26f6021c8a9bb92e332a93d63b039f16b58be2eb61c",
	// 	"0x0000000000000000000000004efca6d4d5f355ca7955d0024b5b35ae5aadf372",
	// 	"0x000000000000000000000000cee077ea790a32927c49c6294c392404d0d31c0a",
	// 	"0x0000000000000000000000000000000000000000000000000000000000000005",
	// 	"0x000000000000000000000000eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
	// 	"0x00000000000000000000000000000000000000000000000004fefa17b7240000",
	// 	"0x0000000000000000000000000000000000000000000000000000000000000120", // ???
	// 	"0x0000000000000000000000005d666f215a85b87cb042d59662a7ecd2c8cc44e6",
	// 	"0x000000000000000000000000000000000000000000000000000000000231ce23",
	// 	"0x0000000000000000000000000000000000000000000000000000000000000001", // ..
	// 	"0x000000000000000000000000d207842d66b715df6ea08cf52f025b9e2ed28788", // ..
	// 	"0x0000000000000000000000000000000000000000000000000019945ca2620000", // ..
	// }

	// NOTE: this does not pass, because the order of the values is not the same
	// dataCheck := ""
	// for i := 0; i < len(eventValues); i++ {
	// 	v := eventValues[i]
	// 	s := v.(string)
	// 	dataCheck += s[2:]
	// }
	// dataCheck = "0x" + dataCheck
	// require.Equal(t, logData, dataCheck)
}

func TestDecodeTransactionLogByEventSig6(t *testing.T) {
	logTopics := []string{
		"0xefed6d3500546b29533b128a29e3a94d70788727f0507505ac12eaf2e578fd9c",
		"0x278944315b79f813d47c1b9acdebbe0faa10420831741faf06a41ffb78379089",
		"0x0000000000000000000000001493e7b8d4dfade0a178dad9335470337a3a219a",
	}
	logData := "0x000000000000000000000000000000000000000000000000000000000000759e00000000000000000000000000000000000000000000000000000000003fab53"

	txnLog := types.Log{}
	txnLog.Topics = []common.Hash{}

	for _, topic := range logTopics {
		txnLog.Topics = append(txnLog.Topics, common.HexToHash(topic))
	}
	txnLog.Data, _ = hexutil.Decode(logData)

	var eventSig = "OFTReceived(bytes32 indexed guid,uint32 srcEid,address indexed toAddress,uint256 amountReceivedLD)"

	eventDef, eventValues, ok, err := ethcoder.DecodeTransactionLogByEventSig(txnLog, eventSig)
	require.NoError(t, err)
	require.True(t, ok)

	require.Equal(t, "0xefed6d3500546b29533b128a29e3a94d70788727f0507505ac12eaf2e578fd9c", eventDef.TopicHash)
	require.Equal(t, "OFTReceived", eventDef.Name)
	require.Equal(t, "OFTReceived(bytes32,uint32,address,uint256)", eventDef.Sig)
	require.Equal(t, []string{"bytes32", "uint32", "address", "uint256"}, eventDef.ArgTypes)
	require.Equal(t, []bool{true, false, true, false}, eventDef.ArgIndexed)
	require.Equal(t, []string{"guid", "srcEid", "toAddress", "amountReceivedLD"}, eventDef.ArgNames)
	require.Equal(t, 4, len(eventValues))

	src := eventValues[0].([32]uint8)
	dst := make([]byte, 32)
	copy(dst[:], src[:])
	require.Equal(t, "0x278944315b79f813d47c1b9acdebbe0faa10420831741faf06a41ffb78379089", hexutil.Encode(dst))
	require.Equal(t, uint32(30110), eventValues[1])
	require.Equal(t, common.HexToAddress("0x1493e7b8d4dfade0a178dad9335470337a3a219a"), eventValues[2])
	require.Equal(t, big.NewInt(4172627), eventValues[3])

	// .. in hex
	eventDef, eventHexValues, ok, err := ethcoder.DecodeTransactionLogByEventSigAsHex(txnLog, eventSig)
	require.NoError(t, err)
	require.True(t, ok)

	require.Equal(t, "0xefed6d3500546b29533b128a29e3a94d70788727f0507505ac12eaf2e578fd9c", eventDef.TopicHash)
	require.Equal(t, "OFTReceived", eventDef.Name)
	require.Equal(t, "OFTReceived(bytes32,uint32,address,uint256)", eventDef.Sig)
	require.Equal(t, []string{"bytes32", "uint32", "address", "uint256"}, eventDef.ArgTypes)
	require.Equal(t, []bool{true, false, true, false}, eventDef.ArgIndexed)
	require.Equal(t, []string{"guid", "srcEid", "toAddress", "amountReceivedLD"}, eventDef.ArgNames)
	require.Equal(t, 4, len(eventValues))

	// spew.Dump(eventValues)
	require.Equal(t, "0x278944315b79f813d47c1b9acdebbe0faa10420831741faf06a41ffb78379089", eventHexValues[0])
	require.Equal(t, "0x000000000000000000000000000000000000000000000000000000000000759e", eventHexValues[1])
	require.Equal(t, "0x1493e7b8d4dfade0a178dad9335470337a3a219a", eventHexValues[2])
	require.Equal(t, "0x00000000000000000000000000000000000000000000000000000000003fab53", eventHexValues[3])
}

func TestEventDecoder(t *testing.T) {
	ed := ethcoder.NewEventDecoder()

	err := ed.RegisterEventSig(
		// even though, these look the same and derive to the same topic hash, they
		// are in fact different because of the indexed flag on the arguments
		// which will effect the decoder
		"Transfer(address indexed from,address indexed to,uint256 indexed amount)",
		"Transfer(address from,address to,uint256 amount)",
		"Other(address,uint256)", // just a random thing to add
	)
	require.NoError(t, err)

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

	eventDef, eventValues, ok, err := ed.DecodeLog(txnLog)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef", eventDef.TopicHash)
	require.Equal(t, "Transfer", eventDef.Name)
	require.Equal(t, "Transfer(address,address,uint256)", eventDef.Sig)
	require.Equal(t, []string{"address", "address", "uint256"}, eventDef.ArgTypes)
	require.Equal(t, []bool{true, true, true}, eventDef.ArgIndexed)
	require.Equal(t, []string{"from", "to", "amount"}, eventDef.ArgNames)
	require.Equal(t, common.HexToAddress("0x0000000000000000000000000000000000000000"), eventValues[0])
	require.Equal(t, common.HexToAddress("0x1a05955180488cb07db065d174b44df9aeb0fdb1"), eventValues[1])
	require.Equal(t, big.NewInt(317297), eventValues[2])

	eventDef, eventHexValues, ok, err := ed.DecodeLogAsHex(txnLog)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef", eventDef.TopicHash)
	require.Equal(t, "Transfer", eventDef.Name)
	require.Equal(t, "Transfer(address,address,uint256)", eventDef.Sig)
	require.Equal(t, []string{"address", "address", "uint256"}, eventDef.ArgTypes)
	require.Equal(t, []bool{true, true, true}, eventDef.ArgIndexed)
	require.Equal(t, []string{"from", "to", "amount"}, eventDef.ArgNames)
	require.Equal(t, "0x0000000000000000000000000000000000000000", eventHexValues[0])
	require.Equal(t, "0x1a05955180488cb07db065d174b44df9aeb0fdb1", eventHexValues[1])
	require.Equal(t, "0x000000000000000000000000000000000000000000000000000000000004d771", eventHexValues[2])
}
