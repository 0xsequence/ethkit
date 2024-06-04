package ethcoder

import "testing"

func TestMerkleProof(t *testing.T) {
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

	// isValid := mt.Verify(proof, sha256Hash([]byte("a")), root)
	// fmt.Printf("Is valid: %v\n", isValid)
}
