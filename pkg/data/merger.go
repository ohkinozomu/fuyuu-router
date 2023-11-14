package data

import (
	"sort"
)

type Merger struct {
	chunks map[string]map[int][]byte
}

func NewMerger() *Merger {
	return &Merger{
		chunks: make(map[string]map[int][]byte),
	}
}

func (m *Merger) AddChunk(chunk *HTTPBodyChunk) {
	if _, exists := m.chunks[chunk.RequestId]; !exists {
		m.chunks[chunk.RequestId] = make(map[int][]byte)
	}
	m.chunks[chunk.RequestId][int(chunk.Sequence)] = chunk.Data
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
