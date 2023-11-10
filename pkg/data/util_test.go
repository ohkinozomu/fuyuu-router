package data

import (
	"net/http"
	"testing"

	"github.com/klauspost/compress/zstd"
	"github.com/stretchr/testify/assert"
)

func TestHTTPHeaderToProtoHeaders(t *testing.T) {
	httpHeader := map[string][]string{
		"Content-Type": {"application/json"},
	}
	protoHeaders := HTTPHeaderToProtoHeaders(httpHeader)
	assert.Equal(t, httpHeader["Content-Type"], protoHeaders.GetHeaders()["Content-Type"].GetValues())
}

func TestSerializedRequestPacket(t *testing.T) {
	encoder, err := zstd.NewWriter(nil)
	if err != nil {
		t.Fatal(err)
	}
	decoder, err := zstd.NewReader(nil)
	if err != nil {
		t.Fatal(err)
	}

	testCases := []struct {
		format  string
		encoder *zstd.Encoder
		decoder *zstd.Decoder
	}{
		{
			format:  "json",
			encoder: nil,
			decoder: nil,
		},
		{
			format:  "protobuf",
			encoder: nil,
			decoder: nil,
		},
		{
			format:  "json",
			encoder: encoder,
			decoder: decoder,
		},
		{
			format:  "protobuf",
			encoder: encoder,
			decoder: decoder,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.format, func(t *testing.T) {
			httpRequestData := HTTPRequestData{
				Method: "GET",
				Path:   "/",
				Headers: &HTTPHeaders{
					Headers: map[string]*HeaderValueList{
						"Content-Type": {
							Values: []string{"application/json"},
						},
					},
				},
				Body: "test",
			}
			b, err := SerializeHTTPRequestData(&httpRequestData, "json")
			if err != nil {
				t.Fatal(err)
			}

			httpRequestPacket := HTTPRequestPacket{
				RequestId:       "test",
				HttpRequestData: b,
			}
			serializedRequestPacket, err := SerializeRequestPacket(&httpRequestPacket, testCase.format, testCase.encoder)
			if err != nil {
				t.Fatal(err)
			}
			deserializedRequestPacket, err := DeserializeRequestPacket(serializedRequestPacket, testCase.format, testCase.decoder)
			if err != nil {
				t.Fatal(err)
			}
			deserializedHTTPRequestData, err := DeserializeHTTPRequestData(deserializedRequestPacket.GetHttpRequestData(), "json")
			if err != nil {
				t.Fatal(err)
			}
			assert.Equal(t, httpRequestData.Body, deserializedHTTPRequestData.Body)
			assert.Equal(t, httpRequestData.Method, deserializedHTTPRequestData.Method)
			assert.Equal(t, httpRequestData.Path, deserializedHTTPRequestData.Path)
			assert.Equal(t, httpRequestData.Headers.GetHeaders()["Content-Type"].GetValues(), deserializedHTTPRequestData.Headers.GetHeaders()["Content-Type"].GetValues())
			assert.Equal(t, httpRequestPacket.GetRequestId(), deserializedRequestPacket.GetRequestId())
		})
	}
}

func TestSerializedResponsePacket(t *testing.T) {
	encoder, err := zstd.NewWriter(nil)
	if err != nil {
		t.Fatal(err)
	}
	decoder, err := zstd.NewReader(nil)
	if err != nil {
		t.Fatal(err)
	}

	testCases := []struct {
		format  string
		encoder *zstd.Encoder
		decoder *zstd.Decoder
	}{
		{
			format:  "json",
			encoder: nil,
			decoder: nil,
		},
		{
			format:  "protobuf",
			encoder: nil,
			decoder: nil,
		},
		{
			format:  "json",
			encoder: encoder,
			decoder: decoder,
		},
		{
			format:  "protobuf",
			encoder: encoder,
			decoder: decoder,
		},
	}

	headers := HTTPHeaderToProtoHeaders(http.Header{
		"Date":           {"Thu, 09 Nov 2023 13:50:27 GMT"},
		"Content-Type":   {"text/plain; charset=utf-8"},
		"Content-Length": {"31"},
	})

	for _, testCase := range testCases {
		t.Run(testCase.format, func(t *testing.T) {
			httpReponseData := HTTPResponseData{
				StatusCode: 200,
				Headers:    &headers,
				Body:       "test",
			}
			b, err := SerializeHTTPResponseData(&httpReponseData, "json")

			httpResponsePacket := HTTPResponsePacket{
				RequestId:        "test",
				HttpResponseData: b,
			}
			serializedResponsePacket, err := SerializeResponsePacket(&httpResponsePacket, testCase.format, testCase.encoder)
			if err != nil {
				t.Fatal(err)
			}
			deserializedResponsePacket, err := DeserializeResponsePacket(serializedResponsePacket, testCase.format, testCase.decoder)
			if err != nil {
				t.Fatal(err)
			}
			deserializedHTTPResponseData, err := DeserializeHTTPResponseData(deserializedResponsePacket.GetHttpResponseData(), "json")
			if err != nil {
				t.Fatal(err)
			}
			assert.Equal(t, httpReponseData.Body, deserializedHTTPResponseData.Body)
			assert.Equal(t, httpReponseData.StatusCode, deserializedHTTPResponseData.StatusCode)
			assert.Equal(t, httpReponseData.Headers.GetHeaders()["Content-Type"].GetValues(), deserializedHTTPResponseData.Headers.GetHeaders()["Content-Type"].GetValues())
			assert.Equal(t, httpResponsePacket.GetRequestId(), deserializedResponsePacket.GetRequestId())
		})
	}
}

func TestHTTPResponseDataSerialize(t *testing.T) {
	testCases := []struct {
		format string
	}{
		{
			format: "json",
		},
		{
			format: "protobuf",
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.format, func(t *testing.T) {
			headers := HTTPHeaderToProtoHeaders(http.Header{
				"Date":           {"Thu, 09 Nov 2023 13:50:27 GMT"},
				"Content-Type":   {"text/plain; charset=utf-8"},
				"Content-Length": {"31"},
			})

			httpResponseData := HTTPResponseData{
				StatusCode: 200,
				Headers:    &headers,
				Body:       "test",
			}

			serializedResponseData, err := SerializeHTTPResponseData(&httpResponseData, testCase.format)
			if err != nil {
				t.Fatal(err)
			}

			deserializedResponseData, err := DeserializeHTTPResponseData(serializedResponseData, testCase.format)
			if err != nil {
				t.Fatal(err)
			}

			assert.Equal(t, httpResponseData.Body, deserializedResponseData.Body)
			assert.Equal(t, httpResponseData.StatusCode, deserializedResponseData.StatusCode)
			assert.Equal(t, httpResponseData.Headers.GetHeaders()["Content-Type"].GetValues(), deserializedResponseData.Headers.GetHeaders()["Content-Type"].GetValues())
		})
	}
}
