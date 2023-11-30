package split

import (
	"github.com/ohkinozomu/fuyuu-router/pkg/data"
)

func splitChunk(body []byte, chunkByte int) [][]byte {
	var chunks [][]byte
	for i := 0; i < len(body); i += chunkByte {
		end := i + chunkByte
		if end > len(body) {
			end = len(body)
		}
		chunks = append(chunks, body[i:end])
	}
	return chunks
}

func Split(id string, bytes []byte, chunkSize int, format string, callbackFn func(int, []byte) error) error {
	chunks := splitChunk(bytes, chunkSize)

	for sequence, chunk := range chunks {
		httpBodyChunk := data.HTTPBodyChunk{
			RequestId: id,
			Total:     int32(len(chunks)),
			Sequence:  int32(sequence + 1),
			Data:      chunk,
		}
		b, err := data.Serialize(&httpBodyChunk, format)
		if err != nil {
			return err
		}
		if err = callbackFn(sequence, b); err != nil {
			return err
		}
	}
	return nil
}

func Merge(merger *Merger, body []byte, format string) (combined []byte, completed bool, err error) {
	chunk, err := data.Deserialize[*data.HTTPBodyChunk](body, format)
	if err != nil {
		return nil, false, err
	}
	merger.AddChunk(chunk)

	if merger.IsComplete(chunk) {
		combined := merger.GetCombinedData(chunk)
		return combined, true, nil
	}
	return nil, false, nil
}
