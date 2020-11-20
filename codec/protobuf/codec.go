package protobuf

import (
	"errors"

	"google.golang.org/protobuf/proto"

	"github.com/zippoxer/bow/codec"
)

var (
	ErrNotProtoMessage = errors.New("value is not a proto.Message")
)

type Codec struct{}

func (c Codec) Marshal(v interface{}, in []byte) (out []byte, err error) {
	m, ok := v.(proto.Message)
	if !ok {
		return nil, ErrNotProtoMessage
	}
	return proto.Marshal(m)
}

func (c Codec) Unmarshal(data []byte, v interface{}) error {
	m, ok := v.(proto.Message)
	if !ok {
		return ErrNotProtoMessage
	}
	return proto.Unmarshal(data, m)
}

func (c Codec) Format() codec.Format {
	return codec.Protobuf
}
