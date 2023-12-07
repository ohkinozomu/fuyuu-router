package split

import (
	"log"
	"sort"
	"sync"

	"github.com/ohkinozomu/fuyuu-router/pkg/data"
)

type chunkData struct {
	total int
	data  map[int][]byte
}

type Merger struct {
	chunks map[string]chunkData
	mu     sync.Mutex
}

func NewMerger() *Merger {
	return &Merger{
		chunks: make(map[string]chunkData),
		mu:     sync.Mutex{},
	}
}

func (m *Merger) AddChunk(chunk *data.HTTPBodyChunk) {
	m.mu.Lock()
	defer m.mu.Unlock()

	c, exists := m.chunks[chunk.RequestId]
	if !exists {
		c = chunkData{
			data: make(map[int][]byte),
		}
	}

	c.data[int(chunk.Sequence)] = chunk.Data

	if chunk.IsLast {
		c.total = int(chunk.Sequence)
	}

	m.chunks[chunk.RequestId] = c
}

func (m *Merger) DeleteChunk(requestId string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.chunks, requestId)
}

func (m *Merger) IsComplete(chunk *data.HTTPBodyChunk) bool {
	chunkData, exists := m.chunks[chunk.RequestId]
	log.Println(chunkData.total, len(chunkData.data))
	return exists && len(chunkData.data) == chunkData.total
}

func (m *Merger) GetCombinedData(chunk *data.HTTPBodyChunk) []byte {
	m.mu.Lock()
	defer m.mu.Unlock()

	chunkData, exists := m.chunks[chunk.RequestId]
	if !exists || len(chunkData.data) != chunkData.total {
		return nil
	}

	sequences := make([]int, 0, len(chunkData.data))
	for seq := range chunkData.data {
		sequences = append(sequences, seq)
	}
	sort.Ints(sequences)

	totalSize := 0
	for _, seq := range sequences {
		totalSize += len(chunkData.data[seq])
	}
	combinedData := make([]byte, totalSize)

	currentIndex := 0
	for _, seq := range sequences {
		copy(combinedData[currentIndex:], chunkData.data[seq])
		currentIndex += len(chunkData.data[seq])
	}

	return combinedData
}
