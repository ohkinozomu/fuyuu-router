package split

import (
	"reflect"
	"testing"

	"github.com/ohkinozomu/fuyuu-router/pkg/data"
	"github.com/stretchr/testify/assert"
)

func TestNewMerger(t *testing.T) {
	merger := NewMerger()
	if merger == nil {
		t.Error("NewMerger returned nil")
	}
	if merger.chunks == nil {
		t.Error("NewMerger did not initialize chunks map")
	}
}

func TestAddChunkAndIsComplete(t *testing.T) {
	merger := NewMerger()
	chunk := &data.HTTPBodyChunk{
		RequestId: "test",
		Sequence:  1,
		IsLast:    false,
		Data:      []byte("part1"),
	}

	merger.AddChunk(chunk)
	assert.Equal(t, merger.chunks[chunk.RequestId].data[int(chunk.Sequence)], chunk.Data)

	if merger.IsComplete(chunk) {
		t.Error("IsComplete should return false when the total number of chunks has not been reached")
	}

	chunk2 := &data.HTTPBodyChunk{
		RequestId: "test",
		Sequence:  2,
		IsLast:    true,
		Data:      []byte("part2"),
	}

	merger.AddChunk(chunk2)
	assert.Equal(t, merger.chunks[chunk2.RequestId].data[int(chunk2.Sequence)], chunk2.Data)

	if !merger.IsComplete(chunk2) {
		t.Error("IsComplete should return true when all chunks have been added")
	}

	if merger.chunks[chunk2.RequestId].total != int(chunk2.Sequence) {
		t.Errorf("total was not set correctly when IsLast is true")
	}
}

func TestGetCombinedData(t *testing.T) {
	merger := NewMerger()
	chunk1 := &data.HTTPBodyChunk{
		RequestId: "test",
		Sequence:  1,
		IsLast:    false,
		Data:      []byte("part1"),
	}
	chunk2 := &data.HTTPBodyChunk{
		RequestId: "test",
		Sequence:  2,
		IsLast:    true,
		Data:      []byte("part2"),
	}

	merger.AddChunk(chunk1)
	merger.AddChunk(chunk2)

	combined := merger.GetCombinedData(chunk1)
	expected := []byte("part1part2")
	if !reflect.DeepEqual(combined, expected) {
		t.Errorf("GetCombinedData returned %v, expected %v", combined, expected)
	}
}
