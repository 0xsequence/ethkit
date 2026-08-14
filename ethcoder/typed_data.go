package ethcoder

import (
	"bytes"
	"fmt"
	"math/big"
	"sort"
	"strconv"
	"strings"

	"github.com/0xsequence/ethkit/go-ethereum/common"
	"github.com/0xsequence/ethkit/go-ethereum/crypto"
)

// EIP-712 -- https://eips.ethereum.org/EIPS/eip-712

type TypedData struct {
	Types       TypedDataTypes         `json:"types"`
	PrimaryType string                 `json:"primaryType"`
	Domain      TypedDataDomain        `json:"domain"`
	Message     map[string]interface{} `json:"message"`
}

type TypedDataTypes map[string][]TypedDataArgument

// maxTypeGraphDepth bounds how deeply types may nest. The encoders below this
// validation (encodeTypeCached, hashStruct, encodeValue) all recurse one frame
// per level, so this ceiling is what keeps a long chain of types from
// exhausting the goroutine stack. It is unconditional rather than an Option
// because UnmarshalJSON validates without any, and no real schema comes close.
const maxTypeGraphDepth = 1024

// ValidateTypeGraph checks the type graph for cycles, unknown field types, and
// excessive nesting depth. A cycle or a deep enough chain would otherwise cause
// runaway recursion in EncodeType/encodeValue and an unrecoverable stack
// overflow, and an unknown field type would fail deep inside the encoders. This
// must be called before any recursive type traversal.
//
// With no options, size is otherwise unbounded (matching prior behavior).
// WithMaxTypes, WithMaxFieldsPerType, and WithMaxWalkVisits additionally bound
// the schema's size and the cost of this traversal itself, which is
// combinatorial for diamond-shaped (but acyclic) type graphs.
func (t TypedDataTypes) ValidateTypeGraph(opts ...Option) error {
	o := resolveOptions(opts)

	if o.maxTypes > 0 {
		typeCount := len(t)
		if _, ok := t["EIP712Domain"]; !ok {
			typeCount++
		}
		if typeCount > o.maxTypes {
			return fmt.Errorf("too many types: %d exceeds limit of %d", typeCount, o.maxTypes)
		}
	}
	if o.maxFieldsPerType > 0 {
		for name, fields := range t {
			if len(fields) > o.maxFieldsPerType {
				return fmt.Errorf("type %q has %d fields, exceeds limit of %d", name, len(fields), o.maxFieldsPerType)
			}
		}
	}

	for name, fields := range t {
		for _, field := range fields {
			if err := t.validateFieldType(field.Type); err != nil {
				return fmt.Errorf("type %q field %q: %w", name, field.Name, err)
			}
		}
	}

	const (
		visiting = 1
		done     = 2
	)
	state := make(map[string]int, len(t))
	visits := make(map[string]int, len(t))

	// The walk is an explicit-stack DFS rather than recursion so that its own
	// depth is heap-bound; maxTypeGraphDepth still caps it, to protect the
	// recursive encoders that run after this passes.
	//
	// depth is the longest downward path from a type, tracked separately from
	// visits: the encoders start from primaryType with a cold cache, so the
	// live DFS stack (which memoization can cut short depending on map
	// iteration order) is not on its own a sound bound on their recursion.
	type frame struct {
		name  string
		field int
		total int
		depth int
	}
	depths := make(map[string]int, len(t))

	tooComplex := func() error {
		return fmt.Errorf("type graph too complex: exceeds %d traversal steps", o.maxWalkVisits)
	}

	sum := 0
	for root := range t {
		if state[root] == done {
			sum += visits[root]
			if o.maxWalkVisits > 0 && sum > o.maxWalkVisits {
				return tooComplex()
			}
			continue
		}

		state[root] = visiting
		stack := []frame{{name: root, total: 1}}

		for len(stack) > 0 {
			top := &stack[len(stack)-1]

			if top.field < len(t[top.name]) {
				base := t[top.name][top.field].Type
				top.field++
				if i := strings.Index(base, "["); i > 0 {
					base = base[:i]
				}
				if _, ok := t[base]; !ok {
					continue
				}
				switch state[base] {
				case visiting:
					return fmt.Errorf("cycle detected in type graph at %q", base)
				case done:
					top.total += visits[base]
					if depths[base] > top.depth {
						top.depth = depths[base]
					}
					if o.maxWalkVisits > 0 && top.total > o.maxWalkVisits {
						return tooComplex()
					}
					continue
				}
				if len(stack) >= maxTypeGraphDepth {
					return fmt.Errorf("type graph too deep: exceeds %d levels", maxTypeGraphDepth)
				}
				state[base] = visiting
				stack = append(stack, frame{name: base, total: 1})
				continue
			}

			depth := top.depth + 1
			if depth > maxTypeGraphDepth {
				return fmt.Errorf("type graph too deep: exceeds %d levels", maxTypeGraphDepth)
			}
			state[top.name] = done
			visits[top.name] = top.total
			depths[top.name] = depth
			total := top.total
			stack = stack[:len(stack)-1]

			if len(stack) > 0 {
				parent := &stack[len(stack)-1]
				parent.total += total
				if depth > parent.depth {
					parent.depth = depth
				}
				if o.maxWalkVisits > 0 && parent.total > o.maxWalkVisits {
					return tooComplex()
				}
				continue
			}
			sum += total
			if o.maxWalkVisits > 0 && sum > o.maxWalkVisits {
				return tooComplex()
			}
		}
	}
	return nil
}

// validateFieldType rejects any field type that is neither a type defined in
// this schema nor a well-formed EIP-712 primitive. Without this an unknown
// token reaches the primitive decoder, which has no branch for it.
func (t TypedDataTypes) validateFieldType(typ string) error {
	base := typ
	if i := strings.Index(base, "["); i > 0 {
		if !validArraySuffix(typ[i:]) {
			return fmt.Errorf("invalid array suffix in type %q", typ)
		}
		base = base[:i]
	}
	if _, ok := t[base]; ok {
		return nil
	}
	if !isPrimitiveType(base) {
		return fmt.Errorf("unknown type %q", typ)
	}
	return nil
}

// validArraySuffix reports whether s is a run of "[]" / "[N]" groups.
func validArraySuffix(s string) bool {
	for len(s) > 0 {
		if s[0] != '[' {
			return false
		}
		end := strings.IndexByte(s, ']')
		if end < 0 {
			return false
		}
		for _, c := range s[1:end] {
			if c < '0' || c > '9' {
				return false
			}
		}
		s = s[end+1:]
	}
	return true
}

// isPrimitiveType reports whether typ is an EIP-712 atomic or dynamic type.
// Bare "uint"/"int" are rejected: EIP-712 requires the canonical uint256/int256
// spelling, and the packer cannot size them.
func isPrimitiveType(typ string) bool {
	switch typ {
	case "address", "bool", "string", "bytes":
		return true
	}
	if match := regexArgBytes.FindStringSubmatch(typ); len(match) > 0 {
		size, err := strconv.Atoi(match[1])
		return err == nil && size >= 1 && size <= 32
	}
	if match := regexArgNumber.FindStringSubmatch(typ); len(match) > 0 {
		if match[2] == "" {
			return false
		}
		size, err := strconv.Atoi(match[2])
		return err == nil && size >= 8 && size <= 256 && size%8 == 0
	}
	return false
}

// typeInfo is the memoized result of encoding one type's EIP-712 type string
// and its Keccak256 hash, keyed by type name for the lifetime of one cache.
type typeInfo struct {
	encodeType string
	hash       []byte
}

// encodeTypeCached is EncodeType's recursive core, sharing cache across the
// whole call tree so a type reached through multiple paths (a diamond in the
// dependency DAG, or the same struct type appearing in many array elements)
// is only encoded once.
func (t TypedDataTypes) encodeTypeCached(cache map[string]*typeInfo, primaryType string) (*typeInfo, error) {
	if info, ok := cache[primaryType]; ok {
		return info, nil
	}

	args, ok := t[primaryType]
	if !ok {
		return nil, fmt.Errorf("%s type is not defined", primaryType)
	}

	subTypes := []string{}
	s := primaryType + "("

	for i, arg := range args {
		baseType := arg.Type
		if strings.Index(baseType, "[") > 0 {
			baseType = baseType[:strings.Index(baseType, "[")]
		}

		if _, ok := t[baseType]; ok {
			set := false
			for _, v := range subTypes {
				if v == baseType {
					set = true
					break
				}
			}
			if !set {
				subTypes = append(subTypes, baseType)
			}
		}

		s += arg.Type + " " + arg.Name
		if i < len(args)-1 {
			s += ","
		}
	}
	s += ")"

	sort.Strings(subTypes)
	for _, subType := range subTypes {
		subInfo, err := t.encodeTypeCached(cache, subType)
		if err != nil {
			return nil, err
		}
		s += subInfo.encodeType
	}

	info := &typeInfo{encodeType: s, hash: Keccak256([]byte(s))}
	cache[primaryType] = info
	return info, nil
}

func (t TypedDataTypes) EncodeType(primaryType string) (string, error) {
	info, err := t.encodeTypeCached(make(map[string]*typeInfo), primaryType)
	if err != nil {
		return "", err
	}
	return info.encodeType, nil
}

func (t TypedDataTypes) Map() map[string]map[string]string {
	out := map[string]map[string]string{}
	for k, v := range t {
		m := make(map[string]string, len(v))
		for _, arg := range v {
			m[arg.Name] = arg.Type
		}
		out[k] = m
	}
	return out
}

func (t TypedDataTypes) TypeHash(primaryType string) ([]byte, error) {
	info, err := t.encodeTypeCached(make(map[string]*typeInfo), primaryType)
	if err != nil {
		return nil, err
	}
	return info.hash, nil
}

type TypedDataArgument struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type TypedDataDomain struct {
	Name              string          `json:"name,omitempty"`
	Version           string          `json:"version,omitempty"`
	ChainID           *big.Int        `json:"chainId,omitempty"`
	VerifyingContract *common.Address `json:"verifyingContract,omitempty"`
	Salt              *common.Hash    `json:"salt,omitempty"`
}

func (t TypedDataDomain) Map() map[string]interface{} {
	m := map[string]interface{}{}
	if t.Name != "" {
		m["name"] = t.Name
	}
	if t.Version != "" {
		m["version"] = t.Version
	}
	if t.ChainID != nil {
		m["chainId"] = t.ChainID
	}
	if t.VerifyingContract != nil {
		m["verifyingContract"] = *t.VerifyingContract
	}
	if t.Salt != nil {
		m["salt"] = *t.Salt
	}
	return m
}

func (t *TypedData) HashStruct(primaryType string, data map[string]interface{}) ([]byte, error) {
	return t.hashStruct(make(map[string]*typeInfo), &budgetState{}, 0, primaryType, data)
}

// hashStruct is HashStruct's recursive core. cache and budget are shared
// across the whole call tree of one Encode/EncodeDigest call (domain and
// message alike), so a struct type reached through many array elements has
// its type-hash computed once, and value-driven traversal cost (array
// length, nesting depth) is checked against a single aggregate budget.
func (t *TypedData) hashStruct(cache map[string]*typeInfo, budget *budgetState, depth int, primaryType string, data map[string]interface{}) ([]byte, error) {
	if err := budget.checkDepth(depth); err != nil {
		return nil, err
	}
	info, err := t.Types.encodeTypeCached(cache, primaryType)
	if err != nil {
		return nil, err
	}
	encodedData, err := t.encodeData(cache, budget, depth, primaryType, data)
	if err != nil {
		return nil, err
	}
	v, err := SolidityPack([]string{"bytes32", "bytes"}, []interface{}{BytesToBytes32(info.hash), encodedData})
	if err != nil {
		return nil, err
	}
	return Keccak256(v), nil
}

func (t *TypedData) encodeData(cache map[string]*typeInfo, budget *budgetState, depth int, primaryType string, data map[string]interface{}) ([]byte, error) {
	args, ok := t.Types[primaryType]
	if !ok {
		return nil, fmt.Errorf("%s type is unknown", primaryType)
	}
	if len(args) != len(data) {
		return nil, fmt.Errorf("encoding failed for type %s, expecting %d arguments but received %d data values", primaryType, len(args), len(data))
	}

	encodedTypes := make([]string, len(args))
	encodedValues := make([]interface{}, len(args))

	for i, arg := range args {
		dataValue, ok := data[arg.Name]
		if !ok {
			return nil, fmt.Errorf("data value missing for type %s with argument name %s", primaryType, arg.Name)
		}

		encValue, err := t.encodeValue(cache, budget, depth, arg.Type, dataValue)
		if err != nil {
			return nil, fmt.Errorf("failed to encode %s: %w", arg.Name, err)
		}
		encodedTypes[i] = "bytes"
		encodedValues[i] = encValue
	}

	return SolidityPack(encodedTypes, encodedValues)
}

// encodeValue handles the recursive encoding of values according to their types
func (t *TypedData) encodeValue(cache map[string]*typeInfo, budget *budgetState, depth int, typ string, value interface{}) ([]byte, error) {
	// Handle arrays
	if strings.Index(typ, "[") > 0 {
		baseType := typ[:strings.Index(typ, "[")]
		values, ok := value.([]interface{})
		if !ok {
			return nil, fmt.Errorf("expected array for type %s", typ)
		}

		// Budget checks happen before allocating encodedValues or recursing
		// into any element, so an oversized array is rejected up front.
		if err := budget.checkArray(len(values)); err != nil {
			return nil, err
		}
		if err := budget.checkDepth(depth + 1); err != nil {
			return nil, err
		}

		encodedValues := make([][]byte, len(values))
		for i, val := range values {
			encoded, err := t.encodeValue(cache, budget, depth+1, baseType, val)
			if err != nil {
				return nil, fmt.Errorf("failed to encode array element %d: %w", i, err)
			}
			encodedValues[i] = encoded
		}

		// For arrays, we concatenate the encoded values and hash the result
		concat := bytes.Join(encodedValues, nil)
		return Keccak256(concat), nil
	}

	// Handle bytes and string
	if typ == "bytes" || typ == "string" {
		var bytesValue []byte
		if v, ok := value.([]byte); ok {
			bytesValue = v
		} else if v, ok := value.(string); ok {
			bytesValue = []byte(v)
		} else {
			return nil, fmt.Errorf("invalid value for type %s", typ)
		}
		return Keccak256(bytesValue), nil
	}

	// Handle custom struct types
	if _, isCustomType := t.Types[typ]; isCustomType {
		mapVal, ok := value.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("invalid value for custom type %s", typ)
		}
		encoded, err := t.hashStruct(cache, budget, depth+1, typ, mapVal)
		if err != nil {
			return nil, fmt.Errorf("failed to encode custom type %s: %w", typ, err)
		}
		return PadZeros(encoded, 32)
	}

	// Handle primitive types
	packed, err := SolidityPack([]string{typ}, []interface{}{value})
	if err != nil {
		return nil, err
	}
	return PadZeros(packed, 32)
}

// Encode returns the digest of the typed data and the fully encoded EIP712 typed data message.
//
// NOTE:
// * the digest is the hash of the fully encoded EIP712 message
// * the encoded message is the fully encoded EIP712 message (0x1901 + domain + hashStruct(message))
//
// opts optionally bound both the schema (ValidateTypeGraph) and the message
// data being encoded — see WithMaxTypes, WithMaxFieldsPerType,
// WithMaxWalkVisits, WithMaxArrayElements, WithMaxRecursionDepth, and
// WithMaxTotalValues. With no opts, behavior is unbounded, matching prior
// versions of this function.
func (t *TypedData) Encode(opts ...Option) ([]byte, []byte, error) {
	if err := t.Types.ValidateTypeGraph(opts...); err != nil {
		return nil, nil, err
	}

	EIP191_HEADER := "0x1901" // EIP191 for typed data
	eip191Header, err := HexDecode(EIP191_HEADER)
	if err != nil {
		return nil, nil, err
	}

	// cache and budget are shared across the domain and message hash-struct
	// calls below, so type-hash work and the value-traversal budget are
	// scoped to this single Encode call, not per hash-struct invocation.
	cache := make(map[string]*typeInfo)
	budget := &budgetState{opts: resolveOptions(opts)}

	// Prepare hash struct for the domain
	domainHash, err := t.hashStruct(cache, budget, 0, "EIP712Domain", t.Domain.Map())
	if err != nil {
		return nil, nil, err
	}

	// Prepare hash struct for the message object
	messageHash, err := t.hashStruct(cache, budget, 0, t.PrimaryType, t.Message)
	if err != nil {
		return nil, nil, err
	}

	encodedMessage, err := SolidityPack([]string{"bytes", "bytes32", "bytes32"}, []interface{}{eip191Header, domainHash, messageHash})
	if err != nil {
		return nil, nil, err
	}

	digest := crypto.Keccak256(encodedMessage)

	return digest, encodedMessage, nil
}

// EncodeDigest returns the digest of the typed data message. See Encode for
// the optional resource-limit opts.
func (t *TypedData) EncodeDigest(opts ...Option) ([]byte, error) {
	digest, _, err := t.Encode(opts...)
	if err != nil {
		return nil, err
	}
	return digest, nil
}
