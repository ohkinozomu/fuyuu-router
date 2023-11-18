package data

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func TestNewMerger(t *testing.T) {
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	loggerConfig := zap.Config{
		Level:            zap.NewAtomicLevel(),
		Encoding:         "json",
		EncoderConfig:    encoderConfig,
		OutputPaths:      []string{"stderr"},
		ErrorOutputPaths: []string{"stderr"},
	}

	logger, err := loggerConfig.Build()
	if err != nil {
		t.Fatal(err)
	}
	merger := NewMerger(logger)
	if merger == nil {
		t.Error("NewMerger returned nil")
	}
	if merger.chunks == nil {
		t.Error("NewMerger did not initialize chunks map")
	}
}

func TestAddChunkAndIsComplete(t *testing.T) {
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	loggerConfig := zap.Config{
		Level:            zap.NewAtomicLevel(),
		Encoding:         "json",
		EncoderConfig:    encoderConfig,
		OutputPaths:      []string{"stderr"},
		ErrorOutputPaths: []string{"stderr"},
	}

	logger, err := loggerConfig.Build()
	if err != nil {
		t.Fatal(err)
	}
	merger := NewMerger(logger)
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
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	loggerConfig := zap.Config{
		Level:            zap.NewAtomicLevel(),
		Encoding:         "json",
		EncoderConfig:    encoderConfig,
		OutputPaths:      []string{"stderr"},
		ErrorOutputPaths: []string{"stderr"},
	}

	logger, err := loggerConfig.Build()
	if err != nil {
		t.Fatal(err)
	}
	merger := NewMerger(logger)
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

func TestSplitChunk(t *testing.T) {
	tests := []struct {
		name      string
		body      []byte
		chunkByte int
		want      [][]byte
	}{
		{
			name:      "Empty input",
			body:      []byte(""),
			chunkByte: 4,
			want:      nil,
		},
		{
			name:      "Regular input, complete division",
			body:      []byte("abcdef"),
			chunkByte: 2,
			want:      [][]byte{[]byte("ab"), []byte("cd"), []byte("ef")},
		},
		{
			name:      "Regular input, incomplete division",
			body:      []byte("abcdefgh"),
			chunkByte: 3,
			want:      [][]byte{[]byte("abc"), []byte("def"), []byte("gh")},
		},
		{
			name:      "chunkByte larger than input",
			body:      []byte("abc"),
			chunkByte: 10,
			want:      [][]byte{[]byte("abc")},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SplitChunk(tt.body, tt.chunkByte)
			assert.Equal(t, tt.want, got)
		})
	}
}
