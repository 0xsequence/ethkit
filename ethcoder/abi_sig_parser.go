package ethcoder

import (
	"fmt"
	"strings"
)

func ParseABISignature(abiSignature string) (ABISignature, error) {
	abiSig := ABISignature{
		ArgTypes:   []string{},
		ArgIndexed: []bool{},
		ArgNames:   []string{},
	}

	var errInvalid = fmt.Errorf("abi format is invalid, expecting Method(arg1,arg2,..)")

	if !strings.Contains(abiSignature, "(") || !strings.Contains(abiSignature, ")") {
		return abiSig, errInvalid
	}

	a := strings.Count(abiSignature, "(")
	b := strings.Count(abiSignature, ")")
	if a != b || a < 1 {
		return abiSig, errInvalid
	}

	a = strings.Index(abiSignature, "(")
	b = strings.LastIndex(abiSignature, ")")

	method := strings.TrimSpace(abiSignature[:a])
	abiSig.Name = method

	args := strings.TrimSpace(abiSignature[a+1 : b])

	if args == "" {
		// no arguments, we are done
		abiSig.Signature = fmt.Sprintf("%s()", method)
	} else {
		// event parser
		tree, err := parseABISignatureArgs(args, 0)
		if err != nil {
			return abiSig, err
		}

		sig, typs, indexed, names, err := groupABISignatureTree(tree, true)
		if err != nil {
			return abiSig, err
		}
		abiSig.Signature = fmt.Sprintf("%s(%s)", method, sig)
		abiSig.ArgTypes = typs
		for i, name := range names {
			if name != "" {
				abiSig.ArgNames = append(abiSig.ArgNames, name)
			} else {
				abiSig.ArgNames = append(abiSig.ArgNames, fmt.Sprintf("arg%d", i+1))
			}
		}
		abiSig.ArgIndexed = indexed
	}

	numIndexed := 0
	for _, indexed := range abiSig.ArgIndexed {
		if indexed {
			numIndexed++
		}
	}
	abiSig.NumIndexed = numIndexed

	abiSig.Hash = Keccak256Hash([]byte(abiSig.Signature)).String()

	return abiSig, nil
}

type abiSignatureTree struct {
	left         string
	indexed      []bool
	names        []string
	tuple        []abiSignatureTree
	tupleArray   string
	tupleIndexed bool
	tupleName    string
	right        []abiSignatureTree
}

// parseEventArgs parses the event arguments and returns a tree structure
// ie. "address indexed from, address indexed to, uint256 value".
func parseABISignatureArgs(eventArgs string, iteration int) (abiSignatureTree, error) {
	args := strings.TrimSpace(eventArgs)
	// if iteration == 0 {
	// 	args = strings.ReplaceAll(eventArgs, "  ", "")
	// }

	out := abiSignatureTree{}
	if args == "" {
		return out, nil
	}

	if args[len(args)-1] != ',' {
		args += ","
	}

	a := strings.Index(args, "(")

	p1 := ""
	p2 := ""
	p2ar := ""
	p2indexed := false
	p2name := ""
	p3 := ""

	if a < 0 {
		p1 = args
	} else {
		p1 = strings.TrimSpace(args[:a])
	}

	if a >= 0 {
		z, err := findParensCloseIndex(args[a:])
		if err != nil {
			return out, err
		}
		z += a + 1

		x := strings.Index(args[z:], ",")
		if x > 0 {
			z += x + 1
		}

		p2 = strings.TrimSpace(args[a:z])

		// remove end params
		x1 := strings.LastIndex(p2, "]")
		x2 := strings.LastIndex(p2, ")")
		xx := max(x1, x2)

		// get indexed/var name from end
		n := strings.Split(strings.TrimSpace(p2[xx+1:]), " ")
		n0 := ""
		if len(n) > 0 {
			n0 = strings.TrimRight(n[0], ",")
		}
		if len(n) == 1 && n0 != "indexed" {
			p2name = n0
		} else if len(n) == 1 && n0 == "indexed" {
			p2indexed = true
		} else if len(n) == 2 && n0 == "indexed" {
			p2indexed = true
			p2name = strings.TrimRight(n[1], ",")
		}

		// split indexed/var name from end
		p2 = p2[:xx+1]

		// split array from tuple
		x1 = strings.LastIndex(p2, "]")
		x2 = strings.LastIndex(p2, ")")
		if x1 > 0 && x2 < x1 {
			p2ar = p2[x2+1 : x1+1]
			p2 = p2[:x2+1]
		}
		p2 = p2[1 : len(p2)-1]

		p3 = strings.TrimSpace(args[z:])
	}

	// p1
	if len(p1) > 0 {
		p := strings.Split(p1, ",")
		s := ""
		p1indexed := []bool{}
		p1names := []string{}

		for _, a := range p {
			arg := strings.Split(strings.TrimSpace(a), " ")

			if len(arg) > 3 {
				return out, fmt.Errorf("invalid event argument format")
			}

			if len(arg) == 3 && arg[1] == "indexed" {
				p1indexed = append(p1indexed, true)
				p1names = append(p1names, arg[2])
			} else if len(arg) == 3 && arg[1] != "indexed" {
				return out, fmt.Errorf("invalid event indexed argument format")
			} else if len(arg) == 2 && arg[1] == "indexed" {
				p1indexed = append(p1indexed, true)
				p1names = append(p1names, "")
			} else if len(arg) > 0 && arg[0] != "" {
				p1indexed = append(p1indexed, false)
				if len(arg) > 1 {
					p1names = append(p1names, arg[1])
				} else {
					p1names = append(p1names, "")
				}
			}

			typ := strings.TrimSpace(arg[0])
			if len(typ) > 0 {
				s += typ + ","
			}
		}
		if len(s) > 0 {
			s = s[:len(s)-1]
		}
		out.left = s
		out.indexed = p1indexed
		out.names = p1names
	}

	// p2
	if len(p2) > 0 {
		out2, err := parseABISignatureArgs(p2, iteration+1)
		if err != nil {
			return out, err
		}
		out.tuple = append(out.tuple, out2)
		out.tupleArray = p2ar
		out.tupleIndexed = p2indexed
		out.tupleName = p2name
	}

	// p3
	if len(p3) > 0 {
		out3, err := parseABISignatureArgs(p3, iteration+1)
		if err != nil {
			return out, err
		}
		out.right = append(out.right, out3)
	}

	return out, nil
}

func groupABISignatureTree(t abiSignatureTree, include bool) (string, []string, []bool, []string, error) {
	out := ""
	typs := []string{}
	indexed := []bool{}
	names := []string{}

	a := ""
	b := ""
	c := ""

	a = t.left
	if t.left != "" {
		out += t.left + ","
	}
	if include {
		p := strings.Split(t.left, ",")
		for _, v := range p {
			if v != "" {
				typs = append(typs, v)
			}
		}
		indexed = append(indexed, t.indexed...)
		names = append(names, t.names...)
	}

	for _, child := range t.tuple {
		s, _, _, _, err := groupABISignatureTree(child, false)
		if err != nil {
			return "", nil, nil, nil, err
		}
		if s != "" {
			b = "(" + strings.TrimRight(s, ",") + ")"
		}
	}
	b += t.tupleArray
	if include && b != "" {
		typs = append(typs, b)
		indexed = append(indexed, t.tupleIndexed)
		names = append(names, t.tupleName)
	}

	for _, child := range t.right {
		s, rtyps, rindexed, rnames, err := groupABISignatureTree(child, true)
		if err != nil {
			return "", nil, nil, nil, err
		}
		if s != "" {
			c = strings.TrimRight(s, ",") + ","
		}
		for _, v := range rtyps {
			if v != "" {
				typs = append(typs, v)
			}
		}
		indexed = append(indexed, rindexed...)
		names = append(names, rnames...)
	}

	if len(a) > 0 {
		out = a + ","
	}
	if len(b) > 0 {
		out += b + ","
	}
	if len(c) > 0 {
		out += c
	}

	return strings.TrimRight(out, ","), typs, indexed, names, nil
}

func findParensCloseIndex(args string) (int, error) {
	n := 0
	for i, c := range args {
		if c == '(' {
			n++
		} else if c == ')' {
			n--
			if n == 0 {
				return i, nil
			}
		}
	}
	return -1, fmt.Errorf("invalid function args, no closing parenthesis found")
}
