package codec

type Format byte

const (
	Binary Format = iota
	JSON
	MessagePack
)

// Codec marshals and unmarshals types.
type Codec interface {
	// Marshal returns the encoded form of v.
	// If in is provided with enough capacity, Marshal copies
	// into in, avoiding allocation.
	Marshal(v interface{}, in []byte) (out []byte, err error)

	// Unmarshal decodes data into v.
	Unmarshal(data []byte, v interface{}) error

	// Format returns the data interchange format.
	Format() Format
}

// Marshaler is the interface implemented
// by types that can marshal themselves.
type Marshaler interface {
	// If in is provided with enough capacity, Marshal copies
	// into in, avoiding allocation.
	Marshal(in []byte) (out []byte, err error)
}

// Marshaler is the interface implemented
// by types that can unmarshal themselves.
type Unmarshaler interface {
	Unmarshal(data []byte) error
}
