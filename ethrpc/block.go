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

type rpcTransaction struct {
	tx *types.Transaction
	txExtraInfo
}

type txExtraInfo struct {
	BlockNumber *string         `json:"blockNumber,omitempty"`
	BlockHash   *common.Hash    `json:"blockHash,omitempty"`
	From        *common.Address `json:"from,omitempty"`
	TxType      string          `json:"type,omitempty"`
}

type rpcBlock struct {
	Hash         common.Hash       `json:"hash"`
	Transactions []rpcTransaction  `json:"transactions"`
	UncleHashes  []common.Hash     `json:"uncles"`
	Withdrawals  types.Withdrawals `json:"withdrawals"`
}

func (tx *rpcTransaction) UnmarshalJSON(msg []byte) error {
	if err := json.Unmarshal(msg, &tx.tx); err != nil {
		// for unsupported txn types, we don't completely fail,
		// ie. some chains like arbitrum nova will return a non-standard type
		if err != types.ErrTxTypeNotSupported {
			return err
		}
	}
	return json.Unmarshal(msg, &tx.txExtraInfo)
}

func IntoJSONRawMessage(raw json.RawMessage, ret *json.RawMessage) error {
	*ret = raw
	return nil
}

func IntoBlock(raw json.RawMessage, ret **types.Block) error {
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

		if tx.txExtraInfo.TxType != "" {
			txType, err := hexutil.DecodeUint64(tx.txExtraInfo.TxType)
			if err != nil {
				return err
			}
			if txType > types.DynamicFeeTxType {
				// skip the txn, its a non-standard type we don't care about
				continue
			}
		}

		txs = append(txs, tx.tx)
	}

	// return types.NewBlockWithHeader(head).WithBody(txs, uncles), nil
	block := types.NewBlockWithHeader(head).WithBody(txs, nil).WithWithdrawals(body.Withdrawals)

	// TODO: Remove this, we shouldn't need to use the block cache
	// in order for it to contain the correct block hash
	block.SetHash(body.Hash)

	*ret = block
	return nil
}

// unused
/*func intoBlocks(raw json.RawMessage, ret *[]*types.Block) error {
	var list []json.RawMessage

	err := json.Unmarshal(raw, &list)
	if err != nil {
		return err
	}

	blocks := make([]*types.Block, len(list))

	for i := range list {
		err = intoBlock(list[i], &blocks[i])
		if err != nil {
			return err
		}
	}

	*ret = blocks
	return nil
}*/

func IntoTransaction(raw json.RawMessage, tx **types.Transaction) error {
	return IntoTransactionWithPending(raw, tx, nil)
}

func IntoTransactionWithPending(raw json.RawMessage, tx **types.Transaction, pending *bool) error {
	var body *rpcTransaction
	if err := json.Unmarshal(raw, &body); err != nil {
		return err
	} else if body == nil {
		return ethereum.NotFound
	} else if _, r, _ := body.tx.RawSignatureValues(); r == nil {
		return fmt.Errorf("server returned transaction without signature")
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
