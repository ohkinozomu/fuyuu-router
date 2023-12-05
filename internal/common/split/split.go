package split

import (
	"github.com/ohkinozomu/fuyuu-router/pkg/data"
)

func Merge(merger *Merger, body []byte, format string) (combined []byte, completed bool, err error) {
	chunk, err := data.DeserializeHTTPBodyChunk(body, format)
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
