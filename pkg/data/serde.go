package data

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	reflect "reflect"

	"github.com/thanos-io/objstore"
	"google.golang.org/protobuf/proto"
)

func HTTPHeaderToProtoHeaders(httpHeader http.Header) HTTPHeaders {
	headers := make(map[string]*HeaderValueList)
	for k, v := range httpHeader {
		headers[k] = &HeaderValueList{Values: v}
	}
	return HTTPHeaders{
		Headers: headers,
	}
}

func Serialize[T proto.Message](value T, format string) ([]byte, error) {
	var err error
	var payload []byte
	switch format {
	case "json":
		payload, err = json.Marshal(value)
	case "protobuf":
		payload, err = proto.Marshal(value)
	default:
		return nil, fmt.Errorf("unknown format: %s", format)
	}
	if err != nil {
		return nil, err
	}

	return payload, nil
}

func Deserialize[T proto.Message](payload []byte, format string) (T, error) {
	var err error
	var result T

	tType := reflect.TypeOf(result)
	if tType.Kind() == reflect.Ptr {
		tType = tType.Elem()
	}
	tValue := reflect.New(tType)
	result = tValue.Interface().(T)

	switch format {
	case "json":
		err = json.Unmarshal(payload, result)
	case "protobuf":
		err = proto.Unmarshal(payload, result)
	default:
		return result, fmt.Errorf("unknown format: %s", format)
	}

	return result, err
}

func decodeStorageRelay(body []byte, bucket objstore.Bucket) ([]byte, error) {
	rc, err := bucket.Get(context.Background(), string(body))
	if err != nil {
		return nil, err
	}

	data, err := io.ReadAll(rc)
	if err != nil {
		return nil, err
	}

	err = rc.Close()
	if err != nil {
		return nil, err
	}
	return data, nil
}

func DeserializeHTTPRequestData(b []byte, format string, bucket objstore.Bucket) (*HTTPRequestData, error) {
	httpRequestData, err := Deserialize[*HTTPRequestData](b, format)
	if err != nil {
		return nil, err
	}

	if httpRequestData.Body.Type == "storage_relay" {
		data, err := decodeStorageRelay(httpRequestData.Body.Body, bucket)
		if err != nil {
			return nil, err
		}
		httpRequestData.Body.Body = data
	}

	return httpRequestData, nil
}

func DeserializeHTTPResponseData(b []byte, format string, bucket objstore.Bucket) (*HTTPResponseData, error) {
	httpResponseData, err := Deserialize[*HTTPResponseData](b, format)
	if err != nil {
		return nil, err
	}

	if httpResponseData.Body.Type == "storage_relay" {
		data, err := decodeStorageRelay(httpResponseData.Body.Body, bucket)
		if err != nil {
			return nil, err
		}
		httpResponseData.Body.Body = data
	}

	return httpResponseData, nil
}
