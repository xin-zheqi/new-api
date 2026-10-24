package common

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/jinzhu/copier"
)

func DeepCopy[T any](src *T) (*T, error) {
	if src == nil {
		return nil, fmt.Errorf("copy source cannot be nil")
	}
	var dst T
	err := copier.CopyWithOption(&dst, src, copier.Option{
		DeepCopy: true, IgnoreEmpty: true,
		// Copy opaque JSON and binary payloads in bulk instead of reflecting over
		// every byte. Clone preserves nil/empty values and exact ciphertext bytes,
		// while keeping channel overrides and retries isolated from the source.
		Converters: []copier.TypeConverter{
			{SrcType: json.RawMessage{}, DstType: json.RawMessage{}, Fn: func(src interface{}) (interface{}, error) {
				return json.RawMessage(bytes.Clone(src.(json.RawMessage))), nil
			}},
			{SrcType: []byte{}, DstType: []byte{}, Fn: func(src interface{}) (interface{}, error) {
				return bytes.Clone(src.([]byte)), nil
			}},
		},
	})
	if err != nil {
		return nil, err
	}
	return &dst, nil
}
