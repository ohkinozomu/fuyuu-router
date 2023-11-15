package data

import (
	"reflect"
	"testing"
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
	chunk := &HTTPBodyChunk{
		RequestId: "test",
		Sequence:  1,
		Total:     2,
		Data:      []byte("part1"),
	}

	merger.AddChunk(chunk)
	if !reflect.DeepEqual(merger.chunks[chunk.RequestId][int(chunk.Sequence)], chunk.Data) {
		t.Errorf("AddChunk did not add the chunk data correctly")
	}

	if merger.IsComplete(chunk) {
		t.Error("IsComplete should return false when the total number of chunks has not been reached")
	}

	chunk2 := &HTTPBodyChunk{
		RequestId: "test",
		Sequence:  2,
		Total:     2,
		Data:      []byte("part2"),
	}

	merger.AddChunk(chunk2)
	if !merger.IsComplete(chunk2) {
		t.Error("IsComplete should return true when all chunks have been added")
	}
}

func TestGetCombinedData(t *testing.T) {
	merger := NewMerger()
	chunk1 := &HTTPBodyChunk{
		RequestId: "test",
		Sequence:  1,
		Total:     2,
		Data:      []byte("part1"),
	}
	chunk2 := &HTTPBodyChunk{
		RequestId: "test",
		Sequence:  2,
		Total:     2,
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
