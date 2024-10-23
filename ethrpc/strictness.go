package ethrpc

// StrictnessLevel is the level of strictness for validation when unmarshalling
// blocks and transactions from RPC responses from a node.
type StrictnessLevel uint8

const (
	StrictnessLevel_None   StrictnessLevel = iota // 0: disabled, no validation on blocks or transactions (default)
	StrictnessLevel_Semi                          // 1: semi-strict transactions – validates only transaction V, R, S values
	StrictnessLevel_Strict                        // 2: strict block and transactions – validates block hash, sender address, and transaction signatures
)

var StrictnessLevels = map[uint8]string{
	0: "NONE",
	1: "SEMI",
	2: "STRICT",
}

func (x StrictnessLevel) String() string {
	return StrictnessLevels[uint8(x)]
}
