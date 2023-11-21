package split

import (
	"github.com/ohkinozomu/fuyuu-router/pkg/data"
)

func Split(bytes []byte, chunkSize int, processFn func(int, []byte, int) (any, error), sendFn func(any) error) error {
	chunks := data.SplitChunk(bytes, chunkSize)

	for sequence, chunk := range chunks {
		payload, err := processFn(sequence, chunk, len(chunks))
		if err != nil {
			return err
		}
		if err = sendFn(payload); err != nil {
			return err
		}
	}
	return nil
}
