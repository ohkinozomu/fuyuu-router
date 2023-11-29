package split

import (
	"sort"
	"sync"

	"github.com/ohkinozomu/fuyuu-router/pkg/data"
)

type Merger struct {
	chunks map[string]map[int][]byte
	mu     sync.Mutex
}

func NewMerger() *Merger {
	return &Merger{
		chunks: make(map[string]map[int][]byte),
		mu:     sync.Mutex{},
	}
}

func (m *Merger) AddChunk(chunk *data.HTTPBodyChunk) {
	// Avoid concurrent map writes
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.chunks[chunk.RequestId]; !exists {
		m.chunks[chunk.RequestId] = make(map[int][]byte)
	}
	m.chunks[chunk.RequestId][int(chunk.Sequence)] = chunk.Data
}

func (m *Merger) DeleteChunk(requestId string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.chunks, requestId)
}

func (m *Merger) IsComplete(chunk *data.HTTPBodyChunk) bool {
	return len(m.chunks[chunk.RequestId]) == int(chunk.Total)
}

func (m *Merger) GetCombinedData(chunk *data.HTTPBodyChunk) []byte {
	// Avoid concurrent map read and map write
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.IsComplete(chunk) {
		return nil
	}

	sequences := make([]int, 0, len(m.chunks[chunk.RequestId]))
	for seq := range m.chunks[chunk.RequestId] {
		sequences = append(sequences, seq)
	}
	sort.Ints(sequences)

	totalSize := 0
	for _, seq := range sequences {
		totalSize += len(m.chunks[chunk.RequestId][seq])
	}
	combinedData := make([]byte, totalSize)
	currentIndex := 0
	for _, seq := range sequences {
		copy(combinedData[currentIndex:], m.chunks[chunk.RequestId][seq])
		currentIndex += len(m.chunks[chunk.RequestId][seq])
	}

	return combinedData
}
