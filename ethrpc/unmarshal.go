package ethrpc

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/big"

	"github.com/0xsequence/ethkit/go-ethereum"
	"github.com/0xsequence/ethkit/go-ethereum/common"
	"github.com/0xsequence/ethkit/go-ethereum/common/hexutil"
	"github.com/0xsequence/ethkit/go-ethereum/core/types"
)

type rpcBlock struct {
	Hash         common.Hash       `json:"hash"`
	Transactions []rpcTransaction  `json:"transactions"`
	UncleHashes  []common.Hash     `json:"uncles"`
	Withdrawals  types.Withdrawals `json:"withdrawals"`
}

type rpcTransaction struct {
	tx           *types.Transaction
	txVRSInvalid bool
	txExtraInfo
}

type txExtraInfo struct {
	BlockNumber *string         `json:"blockNumber,omitempty"`
	BlockHash   *common.Hash    `json:"blockHash,omitempty"`
	From        *common.Address `json:"from,omitempty"`
	TxType      string          `json:"type,omitempty"`
}

func (tx *rpcTransaction) UnmarshalJSON(msg []byte) error {
	err := json.Unmarshal(msg, &tx.tx)
	if err != nil {
		// for unsupported txn types, we don't completely fail,
		// ie. some chains like arbitrum nova will return a non-standard type
		//
		// as well, we ignore ErrInvalidSig, but if strictness is enabled,
		// then we check it in the caller.
		if err != types.ErrTxTypeNotSupported && err != types.ErrInvalidSig {
			return err
		}

		// we set internal flag to check if txn has invalid VRS signature
		if err == types.ErrInvalidSig {
			tx.txVRSInvalid = true
		}
	}

	err = json.Unmarshal(msg, &tx.txExtraInfo)
	if err != nil {
		return err
	}

	return nil
}

func IntoJSONRawMessage(raw json.RawMessage, ret *json.RawMessage, strictness StrictnessLevel) error {
	*ret = raw
	return nil
}

func IntoHeader(raw json.RawMessage, ret **types.Header, strictness StrictnessLevel) error {
	var header *types.Header
	if err := json.Unmarshal(raw, &header); err != nil {
		return err
	}
	if strictness == StrictnessLevel_Strict {
		header.SetHash(header.ComputedBlockHash())
	}
	*ret = header
	return nil
}

func IntoBlock(raw json.RawMessage, ret **types.Block, strictness StrictnessLevel) error {
	if len(raw) == 0 {
		return ethereum.NotFound
	}

	// Decode header and transactions
	var (
		head *types.Header
		body rpcBlock
	)
	if err := json.Unmarshal(raw, &head); err != nil {
		return err
	}
	if head == nil {
		return ethereum.NotFound
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return err
	}

	// Fill the sender cache of transactions in the block.
	txs := make([]*types.Transaction, 0, len(body.Transactions))
	for _, tx := range body.Transactions {
		if tx.From != nil {
			setSenderFromServer(tx.tx, *tx.From, body.Hash)
		}

		if strictness >= StrictnessLevel_Semi && tx.txVRSInvalid {
			return fmt.Errorf("invalid transaction v, r, s")
		}

		if tx.txExtraInfo.TxType != "" {
			txType, err := hexutil.DecodeUint64(tx.txExtraInfo.TxType)
			if err != nil {
				return err
			}
			if txType > types.DynamicFeeTxType {
				// skip the txn, its a non-standard type we don't care about
				// NOTE: this is currently skipped blob txn types
				continue
			}
		}

		txs = append(txs, tx.tx)
	}

	/*
		return types.NewBlockWithHeader(head).WithBody(types.Body{
			Transactions: txs,
			Uncles:       uncles,
			Withdrawals:  body.Withdrawals,
		}), nil
	*/
	block := types.NewBlockWithHeader(head).WithBody(types.Body{
		Transactions: txs,
		Withdrawals:  body.Withdrawals,
	})

	// ...
	if strictness == StrictnessLevel_Strict {
		block.SetHash(block.ComputedBlockHash())
	} else {
		block.SetHash(body.Hash)
	}

	*ret = block
	return nil
}

func IntoTransaction(raw json.RawMessage, tx **types.Transaction, strictness StrictnessLevel) error {
	return IntoTransactionWithPending(raw, tx, nil, strictness)
}

func IntoTransactionWithPending(raw json.RawMessage, tx **types.Transaction, pending *bool, strictness StrictnessLevel) error {
	var body *rpcTransaction
	if err := json.Unmarshal(raw, &body); err != nil {
		return err
	}

	if body == nil {
		return ethereum.NotFound
	}

	if strictness >= StrictnessLevel_Semi {
		if body.txVRSInvalid {
			return fmt.Errorf("invalid transaction v, r, s")
		}
		if _, r, _ := body.tx.RawSignatureValues(); r == nil {
			return fmt.Errorf("server returned transaction without signature")
		}
	}

	if body.From != nil && body.BlockHash != nil {
		setSenderFromServer(body.tx, *body.From, *body.BlockHash)
	}

	*tx = body.tx
	if pending != nil {
		*pending = body.BlockNumber == nil
	}
	return nil
}

// senderFromServer is a types.Signer that remembers the sender address returned by the RPC
// server. It is stored in the transaction's sender address cache to avoid an additional
// request in TransactionSender.
type senderFromServer struct {
	addr      common.Address
	blockhash common.Hash
}

var errNotCached = errors.New("ethrpc: sender not cached")

func setSenderFromServer(tx *types.Transaction, addr common.Address, block common.Hash) {
	// Use types.Sender for side-effect to store our signer into the cache.
	if tx == nil {
		panic("tx is nil")
	}
	types.Sender(&senderFromServer{addr, block}, tx)
}

func (s *senderFromServer) Equal(other types.Signer) bool {
	os, ok := other.(*senderFromServer)
	return ok && os.blockhash == s.blockhash
}

func (s *senderFromServer) Sender(tx *types.Transaction) (common.Address, error) {
	if s.blockhash == (common.Hash{}) {
		return common.Address{}, errNotCached
	}
	return s.addr, nil
}

func (s *senderFromServer) ChainID() *big.Int {
	panic("can't sign with senderFromServer")
}
func (s *senderFromServer) Hash(tx *types.Transaction) common.Hash {
	panic("can't sign with senderFromServer")
}
func (s *senderFromServer) SignatureValues(tx *types.Transaction, sig []byte) (R, S, V *big.Int, err error) {
	panic("can't sign with senderFromServer")
}
