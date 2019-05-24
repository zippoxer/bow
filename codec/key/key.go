// Package key implements the standard encoding and decoding of Bow keys.
package key

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/zippoxer/bow/codec"
)

type Codec struct{}

func (c Codec) Marshal(v interface{}, in []byte) ([]byte, error) {
	switch k := v.(type) {
	case codec.Marshaler:
		return k.Marshal(in)
	case []byte:
		return k, nil
	case string:
		return []byte(k), nil
	case byte:
		return []byte{k}, nil
	case uint16, uint32, uint64, *uint16, *uint32, *uint64,
		[]uint16, []uint32, []uint64, int8, int16, int32, int64,
		*int8, *int16, *int32, *int64, []int8, []int16, []int32, []int64:
		b := bytes.NewBuffer(make([]byte, 8))
		if err := binary.Write(b, binary.BigEndian, k); err != nil {
			return nil, err
		}
		return b.Bytes(), nil
	case int:
		return c.Marshal(int64(k), in)
	case uint:
		return c.Marshal(uint64(k), in)
	case *int:
		return c.Marshal(int64(*k), in)
	case *uint:
		return c.Marshal(uint64(*k), in)
	case []int:
		b := bytes.NewBuffer(make([]byte, len(k)*8))
		for _, n := range k {
			if err := binary.Write(b, binary.BigEndian, int64(n)); err != nil {
				return nil, err
			}
		}
		return b.Bytes(), nil
	case []uint:
		b := bytes.NewBuffer(make([]byte, len(k)*8))
		for _, n := range k {
			if err := binary.Write(b, binary.BigEndian, uint64(n)); err != nil {
				return nil, err
			}
		}
		return b.Bytes(), nil
	}
	return nil, fmt.Errorf("%T is not a valid key type", v)
}

func (c Codec) Unmarshal(data []byte, v interface{}) error {
	if len(data) == 0 {
		return nil
	}
	switch v := v.(type) {
	case codec.Unmarshaler:
		return v.Unmarshal(data)
	case *[]byte:
		// Copy key to v.
		if cap(*v) < len(data) {
			*v = make([]byte, len(data))
		}
		copy(*v, data)
		*v = (*v)[:len(data)]
	case *string:
		*v = string(data)
	case *byte:
		*v = data[0]
	case *uint16, *uint32, *uint64, *uint, *[]uint16, *[]uint32, *[]uint64, *[]uint,
		*int8, *int16, *int32, *int64, *int, *[]int8, *[]int16, *[]int32, *[]int64, *[]int:
		if err := binary.Read(bytes.NewReader(data), binary.BigEndian, v); err != nil {
			return err
		}
	default:
		return fmt.Errorf("%T is not a valid key type", v)
	}
	return nil
}

func (c Codec) Format() codec.Format {
	return codec.Binary
}
