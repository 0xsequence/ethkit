package ethcoder

import (
	"fmt"
	"strings"

	"github.com/0xsequence/ethkit"
)

// EventTopicHash returns the keccak256 hash of the event signature
//
// e.g. "Transfer(address indexed from, address indexed to, uint256 value)"
// will return 0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef
func EventTopicHash(event string) (ethkit.Hash, error) {
	eventSig := parseEventSignature(event)
	if eventSig == "" {
		return ethkit.Hash{}, fmt.Errorf("ethcoder: event format is invalid, expecting Method(arg1,arg2,..)")
	}
	return Keccak256Hash([]byte(eventSig)), nil
}

func parseEventSignature(event string) string {
	if !strings.Contains(event, "(") || !strings.Contains(event, ")") {
		return ""
	}
	p := strings.Split(event, "(")
	if len(p) != 2 {
		return ""
	}
	method := p[0]

	args := strings.TrimSuffix(p[1], ")")
	if args == "" {
		return fmt.Sprintf("%s()", method)
	}

	typs := []string{}
	p = strings.Split(args, ",")
	for _, a := range p {
		typ := strings.Split(strings.TrimSpace(a), " ")[0]
		typs = append(typs, typ)
	}

	return fmt.Sprintf("%s(%s)", method, strings.Join(typs, ","))
}
