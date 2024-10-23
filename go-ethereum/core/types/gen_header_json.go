package types

import (
	"encoding/json"
	"errors"
	"math/big"

	"github.com/0xsequence/ethkit/go-ethereum/common"
	"github.com/0xsequence/ethkit/go-ethereum/common/hexutil"
)

var _ = (*headerMarshaling)(nil)

// MarshalJSON marshals as JSON.
func (h Header) MarshalJSON() ([]byte, error) {
	type Header struct {
		ParentHash       common.Hash     `json:"parentHash"       gencodec:"required"`
		UncleHash        common.Hash     `json:"sha3Uncles"       gencodec:"required"`
		Coinbase         common.Address  `json:"miner"`
		Root             common.Hash     `json:"stateRoot"        gencodec:"required"`
		TxHash           common.Hash     `json:"transactionsRoot" gencodec:"required"`
		ReceiptHash      common.Hash     `json:"receiptsRoot"     gencodec:"required"`
		Bloom            Bloom           `json:"logsBloom"        gencodec:"required"`
		Difficulty       *hexutil.Big    `json:"difficulty"       gencodec:"required"`
		Number           *hexutil.Big    `json:"number"           gencodec:"required"`
		GasLimit         hexutil.Uint64  `json:"gasLimit"         gencodec:"required"`
		GasUsed          hexutil.Uint64  `json:"gasUsed"          gencodec:"required"`
		Time             hexutil.Uint64  `json:"timestamp"        gencodec:"required"`
		Extra            hexutil.Bytes   `json:"extraData"        gencodec:"required"`
		MixDigest        common.Hash     `json:"mixHash"`
		Nonce            BlockNonce      `json:"nonce"`
		BaseFee          *hexutil.Big    `json:"baseFeePerGas" rlp:"optional"`
		WithdrawalsHash  *common.Hash    `json:"withdrawalsRoot" rlp:"optional"`
		BlobGasUsed      *hexutil.Uint64 `json:"blobGasUsed" rlp:"optional"`
		ExcessBlobGas    *hexutil.Uint64 `json:"excessBlobGas" rlp:"optional"`
		ParentBeaconRoot *common.Hash    `json:"parentBeaconBlockRoot" rlp:"optional"`
		BlockHash        common.Hash     `json:"hash" gencodec:"required"`
	}
	var enc Header
	enc.ParentHash = h.ParentHash
	enc.UncleHash = h.UncleHash
	enc.Coinbase = h.Coinbase
	enc.Root = h.Root
	enc.TxHash = h.TxHash
	enc.ReceiptHash = h.ReceiptHash
	enc.Bloom = h.Bloom
	enc.Difficulty = (*hexutil.Big)(h.Difficulty)
	enc.Number = (*hexutil.Big)(h.Number)
	enc.GasLimit = hexutil.Uint64(h.GasLimit)
	enc.GasUsed = hexutil.Uint64(h.GasUsed)
	enc.Time = hexutil.Uint64(h.Time)
	enc.Extra = h.Extra
	enc.MixDigest = h.MixDigest
	enc.Nonce = h.Nonce
	enc.BaseFee = (*hexutil.Big)(h.BaseFee)
	enc.WithdrawalsHash = h.WithdrawalsHash
	enc.BlobGasUsed = (*hexutil.Uint64)(h.BlobGasUsed)
	enc.ExcessBlobGas = (*hexutil.Uint64)(h.ExcessBlobGas)
	enc.ParentBeaconRoot = h.ParentBeaconRoot
	enc.BlockHash = h.BlockHash
	return json.Marshal(&enc)
}

// UnmarshalJSON unmarshals from JSON.
func (h *Header) UnmarshalJSON(input []byte) error {
	type Header struct {
		ParentHash       *common.Hash    `json:"parentHash"       gencodec:"required"`
		UncleHash        *common.Hash    `json:"sha3Uncles"       gencodec:"required"`
		Coinbase         *common.Address `json:"miner"`
		Root             *common.Hash    `json:"stateRoot"        gencodec:"required"`
		TxHash           *common.Hash    `json:"transactionsRoot" gencodec:"required"`
		ReceiptHash      *common.Hash    `json:"receiptsRoot"     gencodec:"required"`
		Bloom            *Bloom          `json:"logsBloom"        gencodec:"required"`
		Difficulty       *hexutil.Big    `json:"difficulty"       gencodec:"required"`
		Number           *hexutil.Big    `json:"number"           gencodec:"required"`
		GasLimit         *hexutil.Uint64 `json:"gasLimit"         gencodec:"required"`
		GasUsed          *hexutil.Uint64 `json:"gasUsed"          gencodec:"required"`
		Time             *hexutil.Uint64 `json:"timestamp"        gencodec:"required"`
		Extra            *hexutil.Bytes  `json:"extraData"        gencodec:"required"`
		MixDigest        *common.Hash    `json:"mixHash"`
		Nonce            *BlockNonce     `json:"nonce"`
		BaseFee          *hexutil.Big    `json:"baseFeePerGas" rlp:"optional"`
		WithdrawalsHash  *common.Hash    `json:"withdrawalsRoot" rlp:"optional"`
		BlobGasUsed      *hexutil.Uint64 `json:"blobGasUsed" rlp:"optional"`
		ExcessBlobGas    *hexutil.Uint64 `json:"excessBlobGas" rlp:"optional"`
		ParentBeaconRoot *common.Hash    `json:"parentBeaconBlockRoot" rlp:"optional"`
		BlockHash        *common.Hash    `json:"hash" gencodec:"required"`
	}
	var dec Header
	if err := json.Unmarshal(input, &dec); err != nil {
		return err
	}
	if dec.ParentHash == nil {
		return errors.New("missing required field 'parentHash' for Header")
	}
	h.ParentHash = *dec.ParentHash
	if dec.UncleHash == nil {
		return errors.New("missing required field 'sha3Uncles' for Header")
	}
	h.UncleHash = *dec.UncleHash
	if dec.Coinbase != nil {
		h.Coinbase = *dec.Coinbase
	}
	if dec.Root == nil {
		return errors.New("missing required field 'stateRoot' for Header")
	}
	h.Root = *dec.Root
	if dec.TxHash == nil {
		return errors.New("missing required field 'transactionsRoot' for Header")
	}
	h.TxHash = *dec.TxHash
	if dec.ReceiptHash == nil {
		return errors.New("missing required field 'receiptsRoot' for Header")
	}
	h.ReceiptHash = *dec.ReceiptHash
	if dec.Bloom == nil {
		return errors.New("missing required field 'logsBloom' for Header")
	}
	h.Bloom = *dec.Bloom
	if dec.Difficulty == nil {
		return errors.New("missing required field 'difficulty' for Header")
	}
	h.Difficulty = (*big.Int)(dec.Difficulty)
	if dec.Number == nil {
		return errors.New("missing required field 'number' for Header")
	}
	h.Number = (*big.Int)(dec.Number)
	if dec.GasLimit == nil {
		return errors.New("missing required field 'gasLimit' for Header")
	}
	h.GasLimit = uint64(*dec.GasLimit)
	if dec.GasUsed == nil {
		return errors.New("missing required field 'gasUsed' for Header")
	}
	h.GasUsed = uint64(*dec.GasUsed)
	if dec.Time == nil {
		return errors.New("missing required field 'timestamp' for Header")
	}
	h.Time = uint64(*dec.Time)
	if dec.Extra == nil {
		return errors.New("missing required field 'extraData' for Header")
	}
	h.Extra = *dec.Extra
	if dec.MixDigest != nil {
		h.MixDigest = *dec.MixDigest
	}
	if dec.Nonce != nil {
		h.Nonce = *dec.Nonce
	}
	if dec.BaseFee != nil {
		h.BaseFee = (*big.Int)(dec.BaseFee)
	}
	if dec.WithdrawalsHash != nil {
		h.WithdrawalsHash = dec.WithdrawalsHash
	}
	if dec.BlobGasUsed != nil {
		h.BlobGasUsed = (*uint64)(dec.BlobGasUsed)
	}
	if dec.ExcessBlobGas != nil {
		h.ExcessBlobGas = (*uint64)(dec.ExcessBlobGas)
	}
	if dec.ParentBeaconRoot != nil {
		h.ParentBeaconRoot = dec.ParentBeaconRoot
	}
	if dec.BlockHash != nil {
		h.BlockHash = *dec.BlockHash
	}
	return nil
}
