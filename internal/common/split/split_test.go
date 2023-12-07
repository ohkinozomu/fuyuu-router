package split

import (
	"testing"

	"github.com/ohkinozomu/fuyuu-router/pkg/data"
	"github.com/stretchr/testify/assert"
)

func TestMerge(t *testing.T) {
	merger := NewMerger()
	assert.NotNil(t, merger)

	dummyData1 := []byte{0x01, 0x02, 0x03}
	dummyData2 := []byte{0x04, 0x05, 0x06}
	requestID := "testRequest"

	chunk1 := &data.HTTPBodyChunk{
		RequestId: requestID,
		Sequence:  1,
		Data:      dummyData1,
		IsLast:    false,
	}
	merger.AddChunk(chunk1)

	chunk2 := &data.HTTPBodyChunk{
		RequestId: requestID,
		Sequence:  2,
		Data:      dummyData2,
		IsLast:    true,
	}
	merger.AddChunk(chunk2)

	assert.True(t, merger.IsComplete(chunk2))

	combinedData := merger.GetCombinedData(chunk2)
	expectedData := append(dummyData1, dummyData2...)
	assert.Equal(t, expectedData, combinedData)

	_, _, err := Merge(merger, []byte("invalid data"), "invalid format")
	assert.NotNil(t, err)
}
