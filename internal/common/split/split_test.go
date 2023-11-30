package split

import (
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

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
			got := splitChunk(tt.body, tt.chunkByte)
			assert.Equal(t, tt.want, got)
		})
	}
}

func randomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	var seededRand = rand.New(rand.NewSource(time.Now().UnixNano()))

	b := make([]byte, length)
	for i := range b {
		b[i] = charset[seededRand.Intn(len(charset))]
	}
	return string(b)
}

func TestSplitAndMerge(t *testing.T) {
	originalData := []byte(randomString(10000))
	id := "test-id"
	chunkSize := 100
	format := "json"

	dataCh := make(chan []byte)
	doneCh := make(chan bool)
	merger := NewMerger()

	go func() {
		err := Split(id, originalData, chunkSize, format, func(sequence int, chunk []byte) error {
			dataCh <- chunk
			return nil
		})
		if err != nil {
			t.Error(err)
		}
		close(dataCh)
	}()

	go func() {
		for chunk := range dataCh {
			combined, completed, err := Merge(merger, chunk, format)
			if err != nil {
				t.Error(err)
				break
			}
			if completed {
				assert.Equal(t, originalData, combined)
				doneCh <- true
				break
			}
		}
	}()

	<-doneCh
}
