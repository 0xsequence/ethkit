// Copyright 2021 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package types_test

import (
	"bytes"
	"fmt"
	"io"
	"math/big"
	mrand "math/rand"
	"testing"

	"github.com/0xsequence/ethkit/go-ethereum/common"
	"github.com/0xsequence/ethkit/go-ethereum/common/hexutil"
	"github.com/0xsequence/ethkit/go-ethereum/core/types"
	"github.com/0xsequence/ethkit/go-ethereum/crypto"
	"github.com/0xsequence/ethkit/go-ethereum/rlp"
)

// TestEIP2718DeriveSha tests that the input to the DeriveSha function is correct.
func TestEIP2718DeriveSha(t *testing.T) {
	for _, tc := range []struct {
		rlpData string
		exp     string
	}{
		{
			rlpData: "0xb8a701f8a486796f6c6f763380843b9aca008262d4948a8eafb1cf62bfbeb1741769dae1a9dd479961928080f838f7940000000000000000000000000000000000001337e1a0000000000000000000000000000000000000000000000000000000000000000080a0775101f92dcca278a56bfe4d613428624a1ebfc3cd9e0bcc1de80c41455b9021a06c9deac205afe7b124907d4ba54a9f46161498bd3990b90d175aac12c9a40ee9",
			exp:     "01 01f8a486796f6c6f763380843b9aca008262d4948a8eafb1cf62bfbeb1741769dae1a9dd479961928080f838f7940000000000000000000000000000000000001337e1a0000000000000000000000000000000000000000000000000000000000000000080a0775101f92dcca278a56bfe4d613428624a1ebfc3cd9e0bcc1de80c41455b9021a06c9deac205afe7b124907d4ba54a9f46161498bd3990b90d175aac12c9a40ee9\n80 01f8a486796f6c6f763380843b9aca008262d4948a8eafb1cf62bfbeb1741769dae1a9dd479961928080f838f7940000000000000000000000000000000000001337e1a0000000000000000000000000000000000000000000000000000000000000000080a0775101f92dcca278a56bfe4d613428624a1ebfc3cd9e0bcc1de80c41455b9021a06c9deac205afe7b124907d4ba54a9f46161498bd3990b90d175aac12c9a40ee9\n",
		},
	} {
		d := &hashToHumanReadable{}
		var t1, t2 types.Transaction
		rlp.DecodeBytes(common.FromHex(tc.rlpData), &t1)
		rlp.DecodeBytes(common.FromHex(tc.rlpData), &t2)
		txs := types.Transactions{&t1, &t2}
		types.DeriveSha(txs, d)
		if tc.exp != string(d.data) {
			t.Fatalf("Want\n%v\nhave:\n%v", tc.exp, string(d.data))
		}
	}
}

func genTxs(num uint64) (types.Transactions, error) {
	key, err := crypto.HexToECDSA("deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef")
	if err != nil {
		return nil, err
	}
	var addr = crypto.PubkeyToAddress(key.PublicKey)
	newTx := func(i uint64) (*types.Transaction, error) {
		signer := types.NewEIP155Signer(big.NewInt(18))
		utx := types.NewTransaction(i, addr, new(big.Int), 0, new(big.Int).SetUint64(10000000), nil)
		tx, err := types.SignTx(utx, signer, key)
		return tx, err
	}
	var txs types.Transactions
	for i := uint64(0); i < num; i++ {
		tx, err := newTx(i)
		if err != nil {
			return nil, err
		}
		txs = append(txs, tx)
	}
	return txs, nil
}

type dummyDerivableList struct {
	len  int
	seed int
}

func newDummy(seed int) *dummyDerivableList {
	d := &dummyDerivableList{}
	src := mrand.NewSource(int64(seed))
	// don't use lists longer than 4K items
	d.len = int(src.Int63() & 0x0FFF)
	d.seed = seed
	return d
}

func (d *dummyDerivableList) Len() int {
	return d.len
}

func (d *dummyDerivableList) EncodeIndex(i int, w *bytes.Buffer) {
	src := mrand.NewSource(int64(d.seed + i))
	// max item size 256, at least 1 byte per item
	size := 1 + src.Int63()&0x00FF
	io.CopyN(w, mrand.New(src), size)
}

func printList(l types.DerivableList) {
	fmt.Printf("list length: %d\n", l.Len())
	fmt.Printf("{\n")
	for i := 0; i < l.Len(); i++ {
		var buf bytes.Buffer
		l.EncodeIndex(i, &buf)
		fmt.Printf("\"0x%x\",\n", buf.Bytes())
	}
	fmt.Printf("},\n")
}

type flatList []string

func (f flatList) Len() int {
	return len(f)
}
func (f flatList) EncodeIndex(i int, w *bytes.Buffer) {
	w.Write(hexutil.MustDecode(f[i]))
}

type hashToHumanReadable struct {
	data []byte
}

func (d *hashToHumanReadable) Reset() {
	d.data = make([]byte, 0)
}

func (d *hashToHumanReadable) Update(i []byte, i2 []byte) {
	l := fmt.Sprintf("%x %x\n", i, i2)
	d.data = append(d.data, []byte(l)...)
}

func (d *hashToHumanReadable) Hash() common.Hash {
	return common.Hash{}
}
