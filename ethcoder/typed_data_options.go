package ethcoder

import "fmt"

// Option bounds resource use during EIP-712 typed-data processing. The zero
// value of every limit means unlimited, so callers passing no options keep the
// unbounded behavior these functions have always had.
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

// WithMaxTypes caps the distinct types a schema may define, counting the
// implicit EIP712Domain when it is not declared explicitly.
func WithMaxTypes(n int) Option { return func(o *options) { o.maxTypes = n } }

// WithMaxFieldsPerType caps the fields any single type may declare.
func WithMaxFieldsPerType(n int) Option { return func(o *options) { o.maxFieldsPerType = n } }

// WithMaxWalkVisits caps ValidateTypeGraph's own traversal, which is
// combinatorial for diamond-shaped but acyclic type graphs.
func WithMaxWalkVisits(n int) Option { return func(o *options) { o.maxWalkVisits = n } }

// WithMaxArrayElements caps the elements in any single array value.
func WithMaxArrayElements(n int) Option { return func(o *options) { o.maxArrayElements = n } }

// WithMaxRecursionDepth caps how deeply message values may nest.
func WithMaxRecursionDepth(n int) Option { return func(o *options) { o.maxRecursionDepth = n } }

// WithMaxTotalValues caps array elements aggregated across the whole message.
func WithMaxTotalValues(n int) Option { return func(o *options) { o.maxTotalValues = n } }

// budgetState is scoped to a single Encode call: unlike the type-hash cache,
// which is schema-derived, these counts come from the message being encoded.
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
