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
	testCases := []struct {
		format   string
		compress string
		encoder  *zstd.Encoder
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
			body := []byte("test")

			if testCase.compress == "zstd" && testCase.encoder != nil {
				body = testCase.encoder.EncodeAll(body, nil)
			}

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
				Body: &HTTPBody{
					Body: body,
					Type: "data",
				},
			}
			b, err := SerializeHTTPRequestData(&httpRequestData, testCase.format)
			if err != nil {
				t.Fatal(err)
			}

			httpRequestPacket := HTTPRequestPacket{
				RequestId:       "test",
				HttpRequestData: b,
				Compress:        testCase.compress,
			}
			serializedRequestPacket, err := SerializeRequestPacket(&httpRequestPacket, testCase.format)
			if err != nil {
				t.Fatal(err)
			}
			deserializedRequestPacket, err := DeserializeRequestPacket(serializedRequestPacket, testCase.format)
			if err != nil {
				t.Fatal(err)
			}
			deserializedHTTPRequestData, err := DeserializeHTTPRequestData(deserializedRequestPacket.GetHttpRequestData(), testCase.format, nil)
			if err != nil {
				t.Fatal(err)
			}
			assert.Equal(t, httpRequestData.Body.Body, deserializedHTTPRequestData.Body.Body)
			assert.Equal(t, httpRequestData.Method, deserializedHTTPRequestData.Method)
			assert.Equal(t, httpRequestData.Path, deserializedHTTPRequestData.Path)
			assert.Equal(t, httpRequestData.Headers.GetHeaders()["Content-Type"].GetValues(), deserializedHTTPRequestData.Headers.GetHeaders()["Content-Type"].GetValues())
			assert.Equal(t, httpRequestPacket.GetRequestId(), deserializedRequestPacket.GetRequestId())
		})
	}
}

func TestSerializedResponsePacket(t *testing.T) {
	testCases := []struct {
		compress string
		format   string
	}{
		{
			format: "json",
		},
		{
			format: "protobuf",
		},
	}

	headers := HTTPHeaderToProtoHeaders(http.Header{
		"Date":           {"Thu, 09 Nov 2023 13:50:27 GMT"},
		"Content-Type":   {"text/plain; charset=utf-8"},
		"Content-Length": {"31"},
	})

	for _, testCase := range testCases {
		t.Run(testCase.format, func(t *testing.T) {
			body := []byte("test")

			httpReponseData := HTTPResponseData{
				StatusCode: 200,
				Headers:    &headers,
				Body: &HTTPBody{
					Body: body,
					Type: "data",
				},
			}
			b, err := SerializeHTTPResponseData(&httpReponseData, testCase.format)
			if err != nil {
				t.Fatal(err)
			}

			httpResponsePacket := HTTPResponsePacket{
				RequestId:        "test",
				HttpResponseData: b,
				Compress:         testCase.compress,
			}
			serializedResponsePacket, err := SerializeResponsePacket(&httpResponsePacket, testCase.format)
			if err != nil {
				t.Fatal(err)
			}
			deserializedResponsePacket, err := DeserializeResponsePacket(serializedResponsePacket, testCase.format)
			if err != nil {
				t.Fatal(err)
			}
			deserializedHTTPResponseData, err := DeserializeHTTPResponseData(deserializedResponsePacket.GetHttpResponseData(), testCase.format, nil)
			if err != nil {
				t.Fatal(err)
			}
			assert.Equal(t, httpReponseData.Body.Body, deserializedHTTPResponseData.Body.Body)
			assert.Equal(t, httpReponseData.StatusCode, deserializedHTTPResponseData.StatusCode)
			assert.Equal(t, httpReponseData.Headers.GetHeaders()["Content-Type"].GetValues(), deserializedHTTPResponseData.Headers.GetHeaders()["Content-Type"].GetValues())
			assert.Equal(t, httpResponsePacket.GetRequestId(), deserializedResponsePacket.GetRequestId())
		})
	}
}

func TestHTTPResponseDataSerialize(t *testing.T) {
	encoder, err := zstd.NewWriter(nil)
	if err != nil {
		t.Fatal(err)
	}
	decoder, err := zstd.NewReader(nil)
	if err != nil {
		t.Fatal(err)
	}

	testCases := []struct {
		compress string
		format   string
		encoder  *zstd.Encoder
		decoder  *zstd.Decoder
	}{
		{
			compress: "none",
			format:   "json",
			encoder:  nil,
			decoder:  nil,
		},
		{
			compress: "none",
			format:   "protobuf",
			encoder:  nil,
			decoder:  nil,
		},
		{
			compress: "zstd",
			format:   "json",
			encoder:  encoder,
			decoder:  decoder,
		},
		{
			compress: "zstd",
			format:   "protobuf",
			encoder:  encoder,
			decoder:  decoder,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.format, func(t *testing.T) {
			headers := HTTPHeaderToProtoHeaders(http.Header{
				"Date":           {"Thu, 09 Nov 2023 13:50:27 GMT"},
				"Content-Type":   {"text/plain; charset=utf-8"},
				"Content-Length": {"31"},
			})

			body := []byte("test")

			if testCase.compress == "zstd" && testCase.encoder != nil {
				body = testCase.encoder.EncodeAll(body, nil)
			}

			httpResponseData := HTTPResponseData{
				StatusCode: 200,
				Headers:    &headers,
				Body:       &HTTPBody{Body: body, Type: "data"},
			}

			serializedResponseData, err := SerializeHTTPResponseData(&httpResponseData, testCase.format)
			if err != nil {
				t.Fatal(err)
			}

			deserializedResponseData, err := DeserializeHTTPResponseData(serializedResponseData, testCase.format, nil)
			if err != nil {
				t.Fatal(err)
			}

			assert.Equal(t, httpResponseData.Body.Body, deserializedResponseData.Body.Body)
			assert.Equal(t, httpResponseData.StatusCode, deserializedResponseData.StatusCode)
			assert.Equal(t, httpResponseData.Headers.GetHeaders()["Content-Type"].GetValues(), deserializedResponseData.Headers.GetHeaders()["Content-Type"].GetValues())
		})
	}
}
