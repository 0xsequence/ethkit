package ethcoder

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"sort"
)

type TValue interface{}
type TLeaf []byte
type TLayer []TLeaf
type THashFn func(value TValue) TLeaf

type Options struct {
	SortLeaves bool
	SortPairs  bool
}

type Proof struct {
	Position string
	Data     TLeaf
}

type MerkleTree struct {
	HashFn     THashFn
	Leaves     []TLeaf
	Layers     []TLayer
	SortLeaves bool
	SortPairs  bool
}

func NewMerkleTree(leaves []TLeaf, options Options) *MerkleTree {
	mt := &MerkleTree{
		HashFn:     bufferifyFn(sha256Hash),
		SortLeaves: options.SortLeaves,
		SortPairs:  options.SortPairs,
	}
	mt.processLeaves(leaves)
	return mt
}

func bufferifyFn(hashFn func(data []byte) []byte) THashFn {
	return func(value TValue) TLeaf {
		switch v := value.(type) {
		case string:
			return hashFn([]byte(v))
		case []byte:
			return hashFn(v)
		default:
			return nil
		}
	}
}

func sha256Hash(data []byte) []byte {
	hash := sha256.Sum256(data)
	return hash[:]
}

func (mt *MerkleTree) processLeaves(leaves []TLeaf) {
	mt.Leaves = make([]TLeaf, len(leaves))
	copy(mt.Leaves, leaves)
	if mt.SortLeaves {
		sort.Slice(mt.Leaves, func(i, j int) bool {
			return bytes.Compare(mt.Leaves[i], mt.Leaves[j]) < 0
		})
	}
	mt.createHashes(mt.Leaves)
}

func (mt *MerkleTree) createHashes(nodes []TLeaf) {
	mt.Layers = append(mt.Layers, nodes)
	for len(nodes) > 1 {
		var nextLayer []TLeaf
		for i := 0; i < len(nodes); i += 2 {
			if i+1 == len(nodes) {
				nextLayer = append(nextLayer, nodes[i])
			} else {
				left := nodes[i]
				right := nodes[i+1]
				if mt.SortPairs && bytes.Compare(left, right) > 0 {
					left, right = right, left
				}
				hash := mt.HashFn(append(left, right...))
				nextLayer = append(nextLayer, hash)
			}
		}
		nodes = nextLayer
		mt.Layers = append(mt.Layers, nodes)
	}
}

func (mt *MerkleTree) GetRoot() TLeaf {
	if len(mt.Layers) == 0 {
		return TLeaf{}
	}
	return mt.Layers[len(mt.Layers)-1][0]
}

func (mt *MerkleTree) GetProof(leaf TLeaf) ([]Proof, error) {
	leafIndex := -1
	for i, l := range mt.Leaves {
		if bytes.Equal(l, leaf) {
			leafIndex = i
			break
		}
	}
	if leafIndex == -1 {
		return nil, errors.New("leaf not found in tree")
	}
	proof := []Proof{}
	for i := 0; i < len(mt.Layers)-1; i++ {
		layer := mt.Layers[i]
		pairIndex := leafIndex ^ 1
		if pairIndex < len(layer) {
			position := "left"
			if leafIndex%2 == 0 {
				position = "right"
			}
			proof = append(proof, Proof{
				Position: position,
				Data:     layer[pairIndex],
			})
		}
		leafIndex /= 2
	}
	return proof, nil
}

func (mt *MerkleTree) Verify(proof []Proof, targetNode, root TLeaf) bool {
	hash := targetNode
	for _, node := range proof {
		var data []byte
		if node.Position == "left" {
			data = append(node.Data, hash...)
		} else {
			data = append(hash, node.Data...)
		}
		hash = mt.HashFn(data)
	}
	return bytes.Equal(hash, root)
}

// TODO: move to merkle_proof_test.go
// func main() {
// 	leaves := []TLeaf{
// 		sha256Hash([]byte("a")),
// 		sha256Hash([]byte("b")),
// 		sha256Hash([]byte("c")),
// 		sha256Hash([]byte("d")),
// 	}
// 	mt := NewMerkleTree(leaves, Options{SortLeaves: false, SortPairs: false})
// 	root := mt.GetRoot()
// 	fmt.Printf("Root: %x\n", root)

// 	proof, err := mt.GetProof(sha256Hash([]byte("a")))
// 	if err != nil {
// 		fmt.Println("Error:", err)
// 		return
// 	}
// 	for _, p := range proof {
// 		fmt.Printf("Proof: Position=%s, Data=%x\n", p.Position, p.Data)
// 	}

// 	isValid := mt.Verify(proof, sha256Hash([]byte("a")), root)
// 	fmt.Printf("Is valid: %v\n", isValid)
// }
