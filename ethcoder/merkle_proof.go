package ethcoder

import (
	"bytes"
	"errors"
	"sort"

	"github.com/0xsequence/ethkit/go-ethereum/common"
	"github.com/0xsequence/ethkit/go-ethereum/crypto"
)

type TValue string
type TLeaf []byte
type TLayer []TLeaf

type Options struct {
	SortLeaves bool
	SortPairs  bool
}

type Proof struct {
	IsLeft   bool
	Data     TLeaf
}

type MerkleTree struct {
	leaves     []TLeaf
	layers     []TLayer
	sortLeaves bool
	sortPairs  bool
}

func NewMerkleTree(leaves []TLeaf, options Options) *MerkleTree {
	mt := &MerkleTree{
		sortLeaves: options.SortLeaves,
		sortPairs:  options.SortPairs,
	}
	mt.processLeaves(leaves)
	return mt
}

func (mt *MerkleTree) processLeaves(leaves []TLeaf) {
	mt.leaves = make([]TLeaf, len(leaves))
	copy(mt.leaves, leaves)
	if mt.sortLeaves {
		sort.Slice(mt.leaves, func(i, j int) bool {
			return bytes.Compare(mt.leaves[i], mt.leaves[j]) < 0
		})
	}
	mt.createHashes(mt.leaves)
}

func (mt *MerkleTree) createHashes(nodes []TLeaf) {
	mt.layers = make([]TLayer, 0)
	mt.layers = append(mt.layers, nodes)
	for len(nodes) > 1 {
		var nextLayer []TLeaf
		for i := 0; i < len(nodes); i += 2 {
			if i+1 == len(nodes) {
				nextLayer = append(nextLayer, nodes[i])
			} else {
				left := nodes[i]
				right := nodes[i+1]
				if mt.sortPairs && bytes.Compare(left, right) > 0 {
					left, right = right, left
				}
				hash := crypto.Keccak256(append(left, right...))
				nextLayer = append(nextLayer, hash)
			}
		}
		nodes = nextLayer
		mt.layers = append(mt.layers, nodes)
	}
}

func (mt *MerkleTree) GetRoot() []byte {
	if len(mt.layers) == 0 {
		return TLeaf{}
	}
	return mt.layers[len(mt.layers)-1][0]
}

func (mt *MerkleTree) GetProof(leaf TLeaf) ([]Proof, error) {
	leafIndex := -1
	for i, l := range mt.leaves {
		if bytes.Equal(l, leaf) {
			leafIndex = i
			break
		}
	}
	if leafIndex == -1 {
		return nil, errors.New("leaf not found in tree")
	}

	proof := []Proof{}
	for i := 0; i < len(mt.layers)-1; i++ {
		layer := mt.layers[i]
		pairIndex := leafIndex ^ 1
		if pairIndex < len(layer) {
			isLeft := leafIndex%2 != 0
			proof = append(proof, Proof{
				IsLeft:   isLeft,
				Data:     layer[pairIndex],
			})
		}
		leafIndex /= 2
	}
	return proof, nil
}

func (mt *MerkleTree) GetHexProof(leaf TLeaf) []common.Hash {
	proof, _ := mt.GetProof(leaf)
	hexProof := make([]common.Hash, len(proof))
	for _, p := range proof {
		hexProof = append(hexProof, common.BytesToHash(p.Data))
	}
	return hexProof
}

func (mt *MerkleTree) Verify(proof []Proof, targetNode, root []byte) (bool, error) {
	hash := targetNode

	if proof == nil || len(targetNode) == 0 || len(root) == 0 {
			return false, nil
	}

	for i := 0; i < len(proof); i++ {
			node := proof[i]
			var data []byte
			var isLeftNode bool

			data = node.Data
			isLeftNode = node.IsLeft

			var buffers [][]byte

			if mt.sortPairs {
					if bytes.Compare(hash, data) < 0 {
							buffers = append(buffers, hash, data)
					} else {
							buffers = append(buffers, data, hash)
					}
					hash = crypto.Keccak256(bytes.Join(buffers, []byte{}))
			} else {
					buffers = append(buffers, hash)
					if isLeftNode {
							buffers = append([][]byte{data}, buffers...)
					} else {
							buffers = append(buffers, data)
					}
					hash = crypto.Keccak256(bytes.Join(buffers, []byte{}))
			}
	}

	return bytes.Equal(hash, root), nil
}
