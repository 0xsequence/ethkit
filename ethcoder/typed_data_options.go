package ethcoder

import "fmt"

// TypedDataOption bounds resource use during EIP-712 typed-data processing.
// The zero value of every limit means unlimited, so callers passing no
// options keep the unbounded behavior these functions have always had.
type TypedDataOption func(*typedDataOptions)

type typedDataOptions struct {
	maxTypes          uint
	maxFieldsPerType  uint
	maxWalkVisits     uint
	maxArrayElements  uint
	maxRecursionDepth uint
	maxTotalValues    uint
}

func resolveOptions(opts []TypedDataOption) typedDataOptions {
	var o typedDataOptions
	for _, opt := range opts {
		opt(&o)
	}
	return o
}

// WithMaxTypes caps the distinct types a schema may define, counting the
// implicit EIP712Domain when it is not declared explicitly.
func WithMaxTypes(n uint) TypedDataOption { return func(o *typedDataOptions) { o.maxTypes = n } }

// WithMaxFieldsPerType caps the fields any single type may declare.
func WithMaxFieldsPerType(n uint) TypedDataOption {
	return func(o *typedDataOptions) { o.maxFieldsPerType = n }
}

// WithMaxWalkVisits caps ValidateTypeGraph's own traversal, which is
// combinatorial for diamond-shaped but acyclic type graphs.
func WithMaxWalkVisits(n uint) TypedDataOption {
	return func(o *typedDataOptions) { o.maxWalkVisits = n }
}

// WithMaxArrayElements caps the elements in any single array value.
func WithMaxArrayElements(n uint) TypedDataOption {
	return func(o *typedDataOptions) { o.maxArrayElements = n }
}

// WithMaxRecursionDepth caps how deeply message values may nest.
func WithMaxRecursionDepth(n uint) TypedDataOption {
	return func(o *typedDataOptions) { o.maxRecursionDepth = n }
}

// WithMaxTotalValues caps array elements aggregated across the whole message.
func WithMaxTotalValues(n uint) TypedDataOption {
	return func(o *typedDataOptions) { o.maxTotalValues = n }
}

// budgetState is scoped to a single Encode call: unlike the type-hash cache,
// which is schema-derived, these counts come from the message being encoded.
type budgetState struct {
	opts        typedDataOptions
	totalValues uint
}

func (b *budgetState) checkArray(n int) error {
	count := uint(n)
	if b.opts.maxArrayElements > 0 && count > b.opts.maxArrayElements {
		return fmt.Errorf("array has %d elements, exceeds limit of %d", n, b.opts.maxArrayElements)
	}
	b.totalValues += count
	if b.opts.maxTotalValues > 0 && b.totalValues > b.opts.maxTotalValues {
		return fmt.Errorf("typed data exceeds aggregate element budget of %d", b.opts.maxTotalValues)
	}
	return nil
}

func (b *budgetState) checkDepth(depth int) error {
	if b.opts.maxRecursionDepth > 0 && uint(depth) > b.opts.maxRecursionDepth {
		return fmt.Errorf("typed data recursion depth %d exceeds limit of %d", depth, b.opts.maxRecursionDepth)
	}
	return nil
}
