// Copyright 2017 Weald Technology Trading
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package ethrpc

import (
	"context"
	"fmt"
	"strings"

	"github.com/0xsequence/ethkit/go-ethereum/common"
	"golang.org/x/crypto/sha3"
	"golang.org/x/net/idna"
)

// TODO: Add a cachestore to cache the results of the ENS lookups

const ENSContractAddress = "0x00000000000C2E074eC69A0dFb2997BA6C7d2e1e"

var p = idna.New(idna.MapForLookup(), idna.StrictDomainName(false), idna.Transitional(false))

func ResolveEnsAddress(ctx context.Context, ens string, provider *Provider) (common.Address, bool, error) {
	// check if it's an address
	ensAddress := common.HexToAddress(ens)
	if ensAddress.Hex() == ens {
		return ensAddress, true, nil
	}

	chainId, err := provider.ChainID(ctx)
	if err != nil {
		return common.Address{}, false, fmt.Errorf("ethrpc: failed to get chainId of the passed provider")
	}
	if chainId.Int64() != 1 {
		return common.Address{}, false, fmt.Errorf("ethrpc: only ENS on mainnet is supported")
	}

	namehash, err := NameHash(ens)
	if err != nil {
		return common.Address{}, false, fmt.Errorf("ethrpc: failed to generate namehash: %w", err)
	}

	resolverAddress, err := provider.contractQuery(ctx, ENSContractAddress, "resolver(bytes32)", "address", []interface{}{namehash})
	if err != nil {
		return common.Address{}, false, fmt.Errorf("ethrpc: failed to query resolver address: %w", err)
	}

	if len(resolverAddress) < 1 || (resolverAddress[0] == common.Address{}.Hex()) {
		return common.Address{}, false, nil
	}

	contractAddress, err := provider.contractQuery(ctx, resolverAddress[0], "addr(bytes32)", "address", []interface{}{namehash})
	if err != nil {
		return common.Address{}, false, fmt.Errorf("ethrpc: failed to query resolver address: %w", err)
	}

	if len(contractAddress) < 1 {
		return common.Address{}, false, nil
	}

	return common.HexToAddress(contractAddress[0]), true, nil
}

// NameHash generates a hash from a name that can be used to
// look up the name in ENS
func NameHash(name string) (hash [32]byte, err error) {
	if name == "" {
		return
	}
	normalizedName, err := Normalize(name)
	if err != nil {
		return
	}
	parts := strings.Split(normalizedName, ".")
	for i := len(parts) - 1; i >= 0; i-- {
		if hash, err = nameHashPart(hash, parts[i]); err != nil {
			return
		}
	}
	return
}

// Normalize normalizes a name according to the ENS rules
func Normalize(input string) (output string, err error) {
	output, err = p.ToUnicode(input)
	if err != nil {
		return
	}
	// If the name started with a period then ToUnicode() removes it, but we want to keep it
	if strings.HasPrefix(input, ".") && !strings.HasPrefix(output, ".") {
		output = "." + output
	}
	return
}

func nameHashPart(currentHash [32]byte, name string) (hash [32]byte, err error) {
	sha := sha3.NewLegacyKeccak256()
	if _, err = sha.Write(currentHash[:]); err != nil {
		return
	}
	nameSha := sha3.NewLegacyKeccak256()
	if _, err = nameSha.Write([]byte(name)); err != nil {
		return
	}
	nameHash := nameSha.Sum(nil)
	if _, err = sha.Write(nameHash); err != nil {
		return
	}
	sha.Sum(hash[:0])
	return
}
