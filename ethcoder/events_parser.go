package ethcoder

import (
	"fmt"
	"strings"
)

type EventDef struct {
	TopicHash  string   `json:"topicHash"`  // the event topic hash, ie. 0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef
	Name       string   `json:"name"`       // the event name, ie. Transfer
	Sig        string   `json:"sig"`        // the event sig, ie. Transfer(address,address,uint256)
	ArgTypes   []string `json:"argTypes"`   // the event arg types, ie. [address, address, uint256]
	ArgNames   []string `json:"argNames"`   // the event arg names, ie. [from, to, value] or ["","",""]
	ArgIndexed []bool   `json:"argIndexed"` // the event arg indexed flag, ie. [true, false, true]
}

func ParseEventDef(event string) (EventDef, error) {
	eventDef := EventDef{
		ArgTypes: []string{},
		ArgNames: []string{},
	}

	var errInvalid = fmt.Errorf("event format is invalid, expecting Method(arg1,arg2,..)")

	if !strings.Contains(event, "(") || !strings.Contains(event, ")") {
		return eventDef, errInvalid
	}

	a := strings.Count(event, "(")
	b := strings.Count(event, ")")
	if a != b || a < 1 {
		return eventDef, errInvalid
	}

	a = strings.Index(event, "(")
	b = strings.LastIndex(event, ")")

	method := strings.TrimSpace(event[:a])
	eventDef.Name = method

	args := strings.TrimSpace(event[a+1 : b])

	if args == "" {
		// no arguments, we are done
		eventDef.Sig = fmt.Sprintf("%s()", method)
	} else {
		// event parser
		tree, err := parseEventArgs(args)
		if err != nil {
			return eventDef, err
		}

		argsSig, typs, indexed, err := groupEventSelectorTree(tree, true)
		if err != nil {
			return eventDef, err
		}
		eventDef.Sig = fmt.Sprintf("%s(%s)", method, argsSig)
		eventDef.ArgTypes = typs
		for i := 0; i < len(typs); i++ {
			eventDef.ArgNames = append(eventDef.ArgNames, "")
		}
		eventDef.ArgIndexed = indexed
	}

	eventDef.TopicHash = Keccak256Hash([]byte(eventDef.Sig)).String()

	return eventDef, nil
}

type eventSelectorTree struct {
	left       string
	indexed    []bool
	tuple      []eventSelectorTree
	tupleArray string
	right      []eventSelectorTree
}

// parseEventArgs parses the event arguments and returns a tree structure
// ie. "address indexed from, address indexed to, uint256 value".
//
// TODO: right now we just parse the argument types, but we should
// also parse out the indexed flag and also the names.
func parseEventArgs(eventArgs string) (eventSelectorTree, error) {
	args := strings.TrimSpace(eventArgs)
	out := eventSelectorTree{}
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
		if strings.LastIndex(p2, "indexed") > x1 || strings.LastIndex(p2, "indexed") > x2 {
			p2indexed = true
		}
		if x1 > x2 {
			p2 = p2[:x1+1]
		} else {
			p2 = p2[:x2+1]
		}

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
		for _, a := range p {
			arg := strings.Split(strings.TrimSpace(a), " ")
			// TODO: lets get the arg name too

			if len(arg) == 3 && arg[1] == "indexed" {
				p1indexed = append(p1indexed, true)
			} else if len(arg) > 0 && arg[0] != "" {
				p1indexed = append(p1indexed, false)
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
	}

	// p2
	if len(p2) > 0 {
		out2, err := parseEventArgs(p2)
		if err != nil {
			return out, err
		}
		out.tuple = append(out.tuple, out2)
		out.tupleArray = p2ar
		out.indexed = append(out.indexed, p2indexed)
	}

	// p3
	if len(p3) > 0 {
		out3, err := parseEventArgs(p3)
		if err != nil {
			return out, err
		}
		out.right = append(out.right, out3)
	}

	return out, nil
}

func groupEventSelectorTree(t eventSelectorTree, include bool) (string, []string, []bool, error) {
	out := ""
	typs := []string{}
	indexed := []bool{}

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
	}

	for _, child := range t.tuple {
		s, _, _, err := groupEventSelectorTree(child, false)
		if err != nil {
			return "", nil, nil, err
		}
		if s != "" {
			b = "(" + strings.TrimRight(s, ",") + ")"
		}
	}
	b += t.tupleArray
	if include && b != "" {
		typs = append(typs, b)
	}

	for _, child := range t.right {
		s, rtyps, i, err := groupEventSelectorTree(child, true)
		if err != nil {
			return "", nil, nil, err
		}
		if s != "" {
			c = strings.TrimRight(s, ",") + ","
		}
		for _, v := range rtyps {
			if v != "" {
				typs = append(typs, v)
			}
		}
		indexed = append(indexed, i...)
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

	return strings.TrimRight(out, ","), typs, indexed, nil
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
