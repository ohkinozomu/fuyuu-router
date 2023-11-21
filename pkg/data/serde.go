package data

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

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

func DeserializeRequestPacket(payload []byte, format string) (*HTTPRequestPacket, error) {
	var err error
	requestPacket := HTTPRequestPacket{}
	switch format {
	case "json":
		err = json.Unmarshal(payload, &requestPacket)
	case "protobuf":
		err = proto.Unmarshal(payload, &requestPacket)
	default:
		return nil, fmt.Errorf("unknown format: %s", format)
	}
	return &requestPacket, err
}

func DeserializeResponsePacket(payload []byte, format string) (*HTTPResponsePacket, error) {
	var err error
	responsePacket := HTTPResponsePacket{}
	switch format {
	case "json":
		err = json.Unmarshal(payload, &responsePacket)
	case "protobuf":
		err = proto.Unmarshal(payload, &responsePacket)
	default:
		return nil, fmt.Errorf("unknown format: %s", format)
	}
	return &responsePacket, err
}

func DeserializeHTTPRequestData(b []byte, format string, bucket objstore.Bucket) (*HTTPRequestData, error) {
	var httpRequestData HTTPRequestData
	switch format {
	case "json":
		if err := json.Unmarshal(b, &httpRequestData); err != nil {
			return nil, fmt.Errorf("error unmarshalling message: %v", err)
		}
	case "protobuf":
		if err := proto.Unmarshal(b, &httpRequestData); err != nil {
			return nil, fmt.Errorf("error unmarshalling message: %v", err)
		}
	default:
		return nil, fmt.Errorf("unknown format: %v", format)
	}

	if httpRequestData.Body.Type == "storage_relay" {
		rc, err := bucket.Get(context.Background(), string(httpRequestData.Body.Body))
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
		httpRequestData.Body.Body = data
	}

	return &httpRequestData, nil
}

func DeserializeHTTPResponseData(b []byte, format string, bucket objstore.Bucket) (*HTTPResponseData, error) {
	var httpResponseData HTTPResponseData
	switch format {
	case "json":
		if err := json.Unmarshal(b, &httpResponseData); err != nil {
			return nil, fmt.Errorf("error unmarshalling message: %v", err)
		}
	case "protobuf":
		if err := proto.Unmarshal(b, &httpResponseData); err != nil {
			return nil, fmt.Errorf("error unmarshalling message: %v", err)
		}
	default:
		return nil, fmt.Errorf("unknown format: %v", format)
	}

	if httpResponseData.Body.Type == "storage_relay" {
		rc, err := bucket.Get(context.Background(), string(httpResponseData.Body.Body))
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
		httpResponseData.Body.Body = data
	}

	return &httpResponseData, nil
}

func DeserializeHTTPBodyChunk(payload []byte, format string) (*HTTPBodyChunk, error) {
	var err error
	httpBodyChunk := HTTPBodyChunk{}
	switch format {
	case "json":
		err = json.Unmarshal(payload, &httpBodyChunk)
	case "protobuf":
		err = proto.Unmarshal(payload, &httpBodyChunk)
	default:
		return nil, fmt.Errorf("unknown format: %s", format)
	}
	return &httpBodyChunk, err
}
