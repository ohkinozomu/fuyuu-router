package data

import (
	"sort"
	"sync"

	"go.uber.org/zap"
)

type Merger struct {
	chunks map[string]map[int][]byte
	logger *zap.Logger
	mu     sync.Mutex
}

func NewMerger(logger *zap.Logger) *Merger {
	return &Merger{
		chunks: make(map[string]map[int][]byte),
		logger: logger,
		mu:     sync.Mutex{},
	}
}

func (m *Merger) AddChunk(chunk *HTTPBodyChunk) {
	// Avoid concurrent map writes
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.chunks[chunk.RequestId]; !exists {
		m.chunks[chunk.RequestId] = make(map[int][]byte)
	}
	m.chunks[chunk.RequestId][int(chunk.Sequence)] = chunk.Data
	m.logger.Debug("Chunk added")
}

func (m *Merger) IsComplete(chunk *HTTPBodyChunk) bool {
	return len(m.chunks[chunk.RequestId]) == int(chunk.Total)
}

func (m *Merger) GetCombinedData(chunk *HTTPBodyChunk) []byte {
	if !m.IsComplete(chunk) {
		return nil
	}

	var sequences []int
	for seq := range m.chunks[chunk.RequestId] {
		sequences = append(sequences, seq)
	}
	sort.Ints(sequences)

	var combinedData []byte
	for _, seq := range sequences {
		combinedData = append(combinedData, m.chunks[chunk.RequestId][seq]...)
	}

	return combinedData
}

func SplitChunk(body []byte, chunkByte int) [][]byte {
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
