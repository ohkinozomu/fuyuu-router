package data

import (
	"net/http"
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

func TestSerializedRequestPacket(t *testing.T) {
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
			httpRequestPacket := HTTPRequestPacket{
				RequestId: "test",
				HttpRequestData: &HTTPRequestData{
					Method: "GET",
					Path:   "/",
					Headers: &HTTPHeaders{
						Headers: map[string]*HeaderValueList{
							"Content-Type": &HeaderValueList{
								Values: []string{"application/json"},
							},
						},
					},
					Body: "test",
				},
			}
			serializedRequestPacket, err := SerializeRequestPacket(&httpRequestPacket, testCase.format)
			if err != nil {
				t.Fatal(err)
			}
			deserializedRequestPacket, err := DeserializeRequestPacket(serializedRequestPacket, testCase.format)
			if err != nil {
				t.Fatal(err)
			}
			assert.Equal(t, httpRequestPacket.GetHttpRequestData().Body, deserializedRequestPacket.GetHttpRequestData().Body)
			assert.Equal(t, httpRequestPacket.GetHttpRequestData().Method, deserializedRequestPacket.GetHttpRequestData().Method)
			assert.Equal(t, httpRequestPacket.GetHttpRequestData().Path, deserializedRequestPacket.GetHttpRequestData().Path)
			assert.Equal(t, httpRequestPacket.GetHttpRequestData().Headers.GetHeaders()["Content-Type"].GetValues(), deserializedRequestPacket.GetHttpRequestData().Headers.GetHeaders()["Content-Type"].GetValues())
			assert.Equal(t, httpRequestPacket.GetRequestId(), deserializedRequestPacket.GetRequestId())
		})
	}
}

func TestSerializedResponsePacket(t *testing.T) {
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

	headers := HTTPHeaderToProtoHeaders(http.Header{
		"Date":           {"Thu, 09 Nov 2023 13:50:27 GMT"},
		"Content-Type":   {"text/plain; charset=utf-8"},
		"Content-Length": {"31"},
	})

	for _, testCase := range testCases {
		t.Run(testCase.format, func(t *testing.T) {
			httpResponsePacket := HTTPResponsePacket{
				RequestId: "test",
				HttpResponseData: &HTTPResponseData{
					StatusCode: 200,
					Headers:    &headers,
					Body:       "test",
				},
			}
			serializedResponsePacket, err := SerializeResponsePacket(&httpResponsePacket, testCase.format)
			if err != nil {
				t.Fatal(err)
			}
			deserializedResponsePacket, err := DeserializeResponsePacket(serializedResponsePacket, testCase.format)
			if err != nil {
				t.Fatal(err)
			}
			assert.Equal(t, httpResponsePacket.GetHttpResponseData().Body, deserializedResponsePacket.GetHttpResponseData().Body)
			assert.Equal(t, httpResponsePacket.GetHttpResponseData().StatusCode, deserializedResponsePacket.GetHttpResponseData().StatusCode)
			assert.Equal(t, httpResponsePacket.GetHttpResponseData().Headers.GetHeaders()["Content-Type"].GetValues(), deserializedResponsePacket.GetHttpResponseData().Headers.GetHeaders()["Content-Type"].GetValues())
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
