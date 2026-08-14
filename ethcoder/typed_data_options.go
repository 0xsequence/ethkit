package ethcoder

import "fmt"

// Option configures optional resource limits for EIP-712 typed-data
// processing (ValidateTypeGraph, Encode, EncodeDigest). The zero value of
// every field means "unlimited", matching the unbounded behavior these
// functions have always had when called with no options.
type Option func(*options)

type options struct {
	maxTypes          int
	maxFieldsPerType  int
	maxWalkVisits     int
	maxArrayElements  int
	maxRecursionDepth int
	maxTotalValues    int
}

func resolveOptions(opts []Option) options {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	return o
}

// WithMaxTypes caps the number of distinct types a schema may define,
// including the implicit EIP712Domain type when it isn't declared explicitly.
func WithMaxTypes(n int) Option { return func(o *options) { o.maxTypes = n } }

// WithMaxFieldsPerType caps the number of fields any single type may declare.
func WithMaxFieldsPerType(n int) Option { return func(o *options) { o.maxFieldsPerType = n } }

// WithMaxWalkVisits caps the total number of type-graph nodes ValidateTypeGraph
// visits, bounding its own traversal cost against diamond-shaped (but acyclic)
// type graphs where naive recursion would otherwise blow up combinatorially.
func WithMaxWalkVisits(n int) Option { return func(o *options) { o.maxWalkVisits = n } }

// WithMaxArrayElements caps the number of elements in any single array value
// within the message being encoded.
func WithMaxArrayElements(n int) Option { return func(o *options) { o.maxArrayElements = n } }

// WithMaxRecursionDepth caps how deeply nested arrays and structs in the
// message may be encoded.
func WithMaxRecursionDepth(n int) Option { return func(o *options) { o.maxRecursionDepth = n } }

// WithMaxTotalValues caps the aggregate number of array elements encoded
// across the entire message for a single Encode/EncodeDigest call.
func WithMaxTotalValues(n int) Option { return func(o *options) { o.maxTotalValues = n } }

// budgetState tracks the value-driven traversal budget for one Encode call.
// It is distinct from the type-hash cache: the cache is schema-derived (safe
// to reuse across domain + message), while this counts actual message data.
type budgetState struct {
	opts        options
	totalValues int
}

func (b *budgetState) checkArray(n int) error {
	if b.opts.maxArrayElements > 0 && n > b.opts.maxArrayElements {
		return fmt.Errorf("array has %d elements, exceeds limit of %d", n, b.opts.maxArrayElements)
	}
	b.totalValues += n
	if b.opts.maxTotalValues > 0 && b.totalValues > b.opts.maxTotalValues {
		return fmt.Errorf("typed data exceeds aggregate element budget of %d", b.opts.maxTotalValues)
	}
	return nil
}

func (b *budgetState) checkDepth(depth int) error {
	if b.opts.maxRecursionDepth > 0 && depth > b.opts.maxRecursionDepth {
		return fmt.Errorf("typed data recursion depth %d exceeds limit of %d", depth, b.opts.maxRecursionDepth)
	}
	return nil
}
