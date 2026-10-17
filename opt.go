package maago

// Opt is a value that may be left unset: its zero value is unset. The
// generated NewXInput types use it for fields with a default, so
// that a field left out takes its default and a zero value goes in only
// when given with Set.
type Opt[T any] struct {
	v   T
	set bool
}

// Set is an Opt holding v, even the zero value.
func Set[T any](v T) Opt[T] { return Opt[T]{v, true} }

// Or is o's value, or def if o is unset.
func (o Opt[T]) Or(def T) T {
	if o.set {
		return o.v
	}
	return def
}

// Ptr is a pointer to v, for the default of a pointer field.
func Ptr[T any](v T) *T { return &v }
