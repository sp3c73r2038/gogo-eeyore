package eeyore

import (
	"github.com/ugorji/go/codec"
)

func MsgpackPackb(m interface{}) ([]byte, error) {
	var b []byte
	var h codec.Handle = new(codec.MsgpackHandle)
	var enc *codec.Encoder = codec.NewEncoderBytes(&b, h)
	err := enc.Encode(m)
	return b, err
}

func MsgpackUnpackb(data []byte, dest interface{}) error {
	var h codec.MsgpackHandle
	h.RawToString = true

	var dec *codec.Decoder = codec.NewDecoderBytes(data, &h)
	err := dec.Decode(dest)

	return err
}
