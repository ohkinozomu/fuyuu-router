package hub

import (
	"github.com/google/uuid"
	"github.com/mustafaturan/bus/v3"
)

type uuidGenerator struct{}

func (g uuidGenerator) Generate() string {
	return uuid.New().String()
}

var _ bus.IDGenerator = (*uuidGenerator)(nil)

func newBus() *bus.Bus {
	b, err := bus.NewBus(uuidGenerator{})
	if err != nil {
		panic(err)
	}

	return b
}
