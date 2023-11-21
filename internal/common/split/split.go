package split

import (
	"github.com/ohkinozomu/fuyuu-router/pkg/data"
)

func Split(id string, bytes []byte, chunkSize int, format string, processFn func(int, []byte) (any, error), sendFn func(any) error) error {
	chunks := data.SplitChunk(bytes, chunkSize)

	for sequence, chunk := range chunks {
		httpBodyChunk := data.HTTPBodyChunk{
			RequestId: id,
			Total:     int32(len(chunks)),
			Sequence:  int32(sequence + 1),
			Data:      chunk,
		}
		b, err := data.SerializeHTTPBodyChunk(&httpBodyChunk, format)
		if err != nil {
			return err
		}
		payload, err := processFn(sequence, b)
		if err != nil {
			return err
		}
		if err = sendFn(payload); err != nil {
			return err
		}
	}
	return nil
}
