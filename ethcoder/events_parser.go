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
	NumIndexed int      `json:"-"`
}

func (e EventDef) String() string {
	if !(len(e.ArgTypes) == len(e.ArgIndexed) && len(e.ArgTypes) == len(e.ArgNames)) {
		return "<invalid event definition>"
	}
	s := ""
	for i := range e.ArgTypes {
		s += e.ArgTypes[i]
		if e.ArgIndexed[i] {
			s += " indexed"
		}
		if e.ArgNames[i] != "" {
			s += " " + e.ArgNames[i]
		}
		if i < len(e.ArgTypes)-1 {
			s += ","
		}
	}
	return fmt.Sprintf("%s(%s)", e.Name, s)
}

func ParseEventDef(event string) (EventDef, error) {
	eventDef := EventDef{
		ArgTypes:   []string{},
		ArgIndexed: []bool{},
		ArgNames:   []string{},
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
		tree, err := parseEventArgs(args, 0)
		if err != nil {
			return eventDef, err
		}

		sig, typs, indexed, names, err := groupEventSelectorTree(tree, true)
		if err != nil {
			return eventDef, err
		}
		eventDef.Sig = fmt.Sprintf("%s(%s)", method, sig)
		eventDef.ArgTypes = typs
		for i, name := range names {
			if name != "" {
				eventDef.ArgNames = append(eventDef.ArgNames, name)
			} else {
				eventDef.ArgNames = append(eventDef.ArgNames, fmt.Sprintf("arg%d", i+1))
			}
		}
		eventDef.ArgIndexed = indexed
	}

	numIndexed := 0
	for _, indexed := range eventDef.ArgIndexed {
		if indexed {
			numIndexed++
		}
	}
	eventDef.NumIndexed = numIndexed

	eventDef.TopicHash = Keccak256Hash([]byte(eventDef.Sig)).String()

	return eventDef, nil
}

type eventSelectorTree struct {
	left         string
	indexed      []bool
	names        []string
	tuple        []eventSelectorTree
	tupleArray   string
	tupleIndexed bool
	tupleName    string
	right        []eventSelectorTree
}

// parseEventArgs parses the event arguments and returns a tree structure
// ie. "address indexed from, address indexed to, uint256 value".
func parseEventArgs(eventArgs string, iteration int) (eventSelectorTree, error) {
	args := strings.TrimSpace(eventArgs)
	// if iteration == 0 {
	// 	args = strings.ReplaceAll(eventArgs, "  ", "")
	// }

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
		out2, err := parseEventArgs(p2, iteration+1)
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
		out3, err := parseEventArgs(p3, iteration+1)
		if err != nil {
			return out, err
		}
		out.right = append(out.right, out3)
	}

	return out, nil
}

func groupEventSelectorTree(t eventSelectorTree, include bool) (string, []string, []bool, []string, error) {
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
		s, _, _, _, err := groupEventSelectorTree(child, false)
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
		s, rtyps, rindexed, rnames, err := groupEventSelectorTree(child, true)
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
