package ethreceipts

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/0xsequence/ethkit/ethmonitor"
	"github.com/0xsequence/ethkit/ethrpc"
	"github.com/0xsequence/ethkit/go-ethereum"
	"github.com/0xsequence/ethkit/go-ethereum/common"
	"github.com/0xsequence/ethkit/go-ethereum/core/types"
	"github.com/0xsequence/ethkit/go-ethereum/crypto"
	"github.com/goware/breaker"
)

// Only the RPC methods used by the listener are provided. No external node is needed.
type hardeningProvider struct {
	ethrpc.RawInterface
	chainID func(context.Context) (*big.Int, error)
	receipt func(context.Context, common.Hash) (*types.Receipt, error)
}

func (p *hardeningProvider) ChainID(ctx context.Context) (*big.Int, error) {
	if p.chainID != nil {
		return p.chainID(ctx)
	}
	return big.NewInt(1), nil
}

func (p *hardeningProvider) TransactionReceipt(ctx context.Context, h common.Hash) (*types.Receipt, error) {
	if p.receipt != nil {
		return p.receipt(ctx, h)
	}
	return nil, ethereum.NotFound
}

func hardeningBlock(num int64, txns ...*types.Transaction) *ethmonitor.Block {
	b := types.NewBlockWithHeader(&types.Header{Number: big.NewInt(num), BlockHash: common.BigToHash(big.NewInt(num)), ParentHash: common.BigToHash(big.NewInt(num - 1)), GasLimit: 30_000_000, Time: uint64(num)})
	return &ethmonitor.Block{Block: b.WithBody(types.Body{Transactions: txns}), Event: ethmonitor.Added, OK: true}
}

func hardeningTxn(t *testing.T, nonce uint64) (*types.Transaction, common.Address, common.Address) {
	t.Helper()
	key, err := crypto.HexToECDSA("1111111111111111111111111111111111111111111111111111111111111111")
	if err != nil {
		t.Fatal(err)
	}
	from := crypto.PubkeyToAddress(key.PublicKey)
	to := common.HexToAddress("0x2222222222222222222222222222222222222222")
	tx, err := types.SignTx(types.NewTransaction(nonce, to, big.NewInt(1), 21000, big.NewInt(1), nil), types.NewEIP155Signer(big.NewInt(1)), key)
	if err != nil {
		t.Fatal(err)
	}
	return tx, from, to
}

func hardeningReceipt(b *ethmonitor.Block, tx *types.Transaction) *types.Receipt {
	return &types.Receipt{TxHash: tx.Hash(), BlockHash: b.Hash(), BlockNumber: b.Number(), Status: 1}
}

func hardeningListener(t *testing.T, p *hardeningProvider, opts Options, blocks ...*ethmonitor.Block) *ReceiptsListener {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	mo := ethmonitor.DefaultOptions
	mo.Logger = log
	mo.WithLogs = true
	mo.BlockRetentionLimit = 100
	mo.Bootstrap = true
	m, err := ethmonitor.NewMonitor(p, mo)
	if err != nil {
		t.Fatal(err)
	}
	if len(blocks) == 0 {
		blocks = []*ethmonitor.Block{hardeningBlock(100)}
	}
	if err = m.Chain().BootstrapFromBlocks(blocks); err != nil {
		t.Fatal(err)
	}
	l, err := NewReceiptsListener(log, p, m, opts)
	if err != nil {
		t.Fatal(err)
	}
	l.br = breaker.New(log, 0, 1, 0)
	return l
}

func hardeningStart(t *testing.T, l *ReceiptsListener) {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- l.Run(context.Background()) }()
	deadline := time.After(time.Second)
	for l.monitor.NumSubscribers() != 1 {
		select {
		case err := <-done:
			t.Fatalf("Run exited before subscription: %v", err)
		case <-deadline:
			t.Fatal("Run did not subscribe to monitor")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	t.Cleanup(func() {
		l.Stop()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("Run: %v", err)
			}
		case <-time.After(time.Second):
			t.Error("Run did not stop")
		}
	})
}

func hardeningRead(t *testing.T, s Subscription) Receipt {
	t.Helper()
	select {
	case r := <-s.TransactionReceipt():
		return r
	case <-time.After(time.Second):
		t.Fatal("missing receipt")
		return Receipt{}
	}
}

func hardeningNoReceipt(t *testing.T, s Subscription) {
	t.Helper()
	select {
	case r := <-s.TransactionReceipt():
		t.Errorf("unexpected receipt: txn=%s block=%s final=%v reorged=%v owner=%p", r.TransactionHash(), r.BlockHash(), r.Final, r.Reorged, r.Filter)
	case <-time.After(30 * time.Millisecond):
	}
}

func hardeningProcess(t *testing.T, l *ReceiptsListener, s *subscriber, blocks ...*ethmonitor.Block) {
	t.Helper()
	if _, err := l.processCachedBlocks(context.Background(), blocks, []*subscriber{s}, [][]Filterer{s.Filters()}); err != nil {
		t.Fatal(err)
	}
}

func hardeningOptions() Options { opts := DefaultOptions; opts.NumBlocksToFinality = 2; return opts }

// The real monitor polls this provider and builds/broadcasts reorg events. Tests
// advance its canonical RPC responses without changing monitor production APIs.
type ownershipLiveProvider struct {
	*hardeningProvider
	mu        sync.Mutex
	canonical map[uint64]*types.Block
	blocks    map[common.Hash]*types.Block
}

func (p *ownershipLiveProvider) advance(blocks ...*types.Block) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, b := range blocks {
		p.canonical[b.NumberU64()] = b
		p.blocks[b.Hash()] = b
	}
}

func ownershipBlockPayload(b *types.Block) (json.RawMessage, error) {
	if b == nil {
		return nil, ethereum.NotFound
	}
	header := b.Header()
	header.Difficulty = big.NewInt(0)
	payload, err := json.Marshal(header)
	if err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	if err = json.Unmarshal(payload, &fields); err != nil {
		return nil, err
	}
	fields["transactions"], err = json.Marshal(b.Transactions())
	if err != nil {
		return nil, err
	}
	fields["uncles"] = json.RawMessage(`[]`)
	return json.Marshal(fields)
}

func (p *ownershipLiveProvider) RawBlockByNumber(_ context.Context, num *big.Int) (json.RawMessage, error) {
	p.mu.Lock()
	b := p.canonical[num.Uint64()]
	p.mu.Unlock()
	return ownershipBlockPayload(b)
}

func (p *ownershipLiveProvider) RawBlockByHash(_ context.Context, hash common.Hash) (json.RawMessage, error) {
	p.mu.Lock()
	b := p.blocks[hash]
	p.mu.Unlock()
	return ownershipBlockPayload(b)
}

func (p *ownershipLiveProvider) RawFilterLogs(context.Context, ethereum.FilterQuery) (json.RawMessage, error) {
	return json.RawMessage(`[]`), nil
}

func receiptsFixCollect(t *testing.T, s Subscription) []Receipt {
	t.Helper()
	var receipts []Receipt
	timer := time.NewTimer(40 * time.Millisecond)
	defer timer.Stop()
	for {
		select {
		case r, ok := <-s.TransactionReceipt():
			if !ok {
				t.Fatal("receipt channel closed unexpectedly")
			}
			receipts = append(receipts, r)
		case <-timer.C:
			return receipts
		}
	}
}
