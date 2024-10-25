package ethcoder

import (
	"bytes"
	"errors"
	"sort"

	"github.com/0xsequence/ethkit/go-ethereum/crypto"
)

type Options struct {
	SortLeaves bool
	SortPairs  bool
}

var DefaultMerkleTreeOptions = Options{
	// Default to true
	SortLeaves: true,
	SortPairs:  true,
}

type Proof struct {
	IsLeft bool
	Data   []byte
}

type MerkleTree[TLeaf any] struct {
	sortLeaves bool
	sortPairs  bool
	hashFn     func(TLeaf) ([]byte, error)
	leaves     []TLeaf
	layers     [][][]byte
}

func NewMerkleTree[TLeaf any](leaves []TLeaf, hashFn *func(TLeaf) ([]byte, error), options *Options) *MerkleTree[TLeaf] {
	if hashFn == nil {
		// Assume TLeaf is []byte
		fn := func(leaf TLeaf) ([]byte, error) {
			return any(leaf).([]byte), nil
		}
		hashFn = &fn
	}
	if options == nil {
		options = &DefaultMerkleTreeOptions
	}
	mt := &MerkleTree[TLeaf]{
		hashFn:     *hashFn,
		sortLeaves: options.SortLeaves,
		sortPairs:  options.SortPairs,
	}
	mt.processLeaves(leaves)
	return mt
}

func (mt *MerkleTree[TLeaf]) processLeaves(leaves []TLeaf) error {
	mt.leaves = make([]TLeaf, len(leaves))
	copy(mt.leaves, leaves)
	nodes := make([][]byte, len(leaves))
	if mt.sortLeaves {
		sort.Slice(mt.leaves, func(i, j int) bool {
			// Ignore err during sort
			a, _ := mt.hashFn(mt.leaves[i])
			b, _ := mt.hashFn(mt.leaves[j])
			return bytes.Compare(a, b) < 0
		})
	}
	for i, leaf := range mt.leaves {
		node, err := mt.hashFn(leaf)
		if err != nil {
			return err
		}
		nodes[i] = node
	}
	mt.createHashes(nodes)
	return nil
}

func (mt *MerkleTree[TLeaf]) createHashes(nodes [][]byte) {
	mt.layers = make([][][]byte, 0)
	mt.layers = append(mt.layers, nodes)
	for len(nodes) > 1 {
		var nextLayer [][]byte
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

func (mt *MerkleTree[TLeaf]) GetRoot() []byte {
	if len(mt.layers) == 0 {
		return nil
	}
	return mt.layers[len(mt.layers)-1][0]
}

func (mt *MerkleTree[TLeaf]) GetProof(leaf TLeaf) ([]Proof, error) {
	leafIndex := -1
	targetNode, err := mt.hashFn(leaf)
	if err != nil {
		return nil, err
	}

	for i, l := range mt.leaves {
		// Ignore err. Already checked in processLeaves
		node, _ := mt.hashFn(l)
		if bytes.Equal(node, targetNode) {
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
				IsLeft: isLeft,
				Data:   layer[pairIndex],
			})
		}
		leafIndex /= 2
	}
	return proof, nil
}

func (mt *MerkleTree[TLeaf]) GetHexProof(leaf TLeaf) [][]byte {
	proof, _ := mt.GetProof(leaf)
	hexProof := make([][]byte, 0, len(proof))
	for _, p := range proof {
		hexProof = append(hexProof, []byte(p.Data))
	}
	return hexProof
}

func (mt *MerkleTree[TLeaf]) Verify(proof []Proof, leaf TLeaf, root []byte) (bool, error) {
	hash, err := mt.hashFn(leaf)
	if err != nil {
		return false, err
	}

	if proof == nil || len(hash) == 0 || len(root) == 0 {
		return false, errors.New("invalid proof, leaf or root")
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
