package data

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHTTPHeaderToProtoHeaders(t *testing.T) {
	httpHeader := map[string][]string{
		"Content-Type": {"application/json"},
	}
	protoHeaders := HTTPHeaderToProtoHeaders(httpHeader)
	assert.Equal(t, httpHeader["Content-Type"], protoHeaders.GetHeaders()["Content-Type"].GetValues())
}
