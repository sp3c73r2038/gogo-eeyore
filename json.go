package eeyore

import (
	"github.com/ugorji/go/codec"
)

func JsonPackb(m interface{}) ([]byte, error) {
	var b []byte
	var h codec.Handle = new(codec.JsonHandle)
	var enc *codec.Encoder = codec.NewEncoderBytes(&b, h)
	err := enc.Encode(m)
	return b, err
}

func JsonUnpackb(data []byte, dest interface{}) error {
	var h codec.JsonHandle

	var dec *codec.Decoder = codec.NewDecoderBytes(data, &h)
	err := dec.Decode(&dest)

	return err
}
