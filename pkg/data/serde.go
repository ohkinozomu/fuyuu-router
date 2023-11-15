package data

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/klauspost/compress/zstd"
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

func SerializeRequestPacket(packet *HTTPRequestPacket, format string) ([]byte, error) {
	var err error
	var payload []byte
	switch format {
	case "json":
		payload, err = json.Marshal(packet)
	case "protobuf":
		payload, err = proto.Marshal(packet)
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

func SerializeResponsePacket(responsePacket *HTTPResponsePacket, format string) ([]byte, error) {
	var err error
	var responsePayload []byte
	switch format {
	case "json":
		responsePayload, err = json.Marshal(responsePacket)
	case "protobuf":
		responsePayload, err = proto.Marshal(responsePacket)
	default:
		return nil, fmt.Errorf("unknown format: %s", format)
	}
	if err != nil {
		return nil, err
	}

	return responsePayload, err
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

func SerializeHTTPRequestData(httpRequestData *HTTPRequestData, format string, encoder *zstd.Encoder) ([]byte, error) {
	var b []byte
	var err error
	switch format {
	case "json":
		b, err = json.Marshal(httpRequestData)
		if err != nil {
			return nil, err
		}
	case "protobuf":
		b, err = proto.Marshal(httpRequestData)
		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unknown format: %s", format)
	}
	if encoder != nil {
		b = encoder.EncodeAll(b, nil)
	}
	return b, nil
}

func DeserializeHTTPRequestData(b []byte, compress string, format string, decoder *zstd.Decoder, bucket objstore.Bucket) (*HTTPRequestData, error) {
	var err error
	if compress == "zstd" && decoder != nil {
		b, err = decoder.DecodeAll(b, nil)
		if err != nil {
			return nil, err
		}
	}

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

func SerializeHTTPResponseData(httpResponseData *HTTPResponseData, format string, encoder *zstd.Encoder) ([]byte, error) {
	var b []byte
	var err error
	switch format {
	case "json":
		b, err = json.Marshal(httpResponseData)
		if err != nil {
			return nil, err
		}
	case "protobuf":
		b, err = proto.Marshal(httpResponseData)
		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unknown format: %s", format)
	}
	if encoder != nil {
		b = encoder.EncodeAll(b, nil)
	}
	return b, nil
}

func DeserializeHTTPResponseData(b []byte, compress string, format string, decoder *zstd.Decoder, bucket objstore.Bucket) (*HTTPResponseData, error) {
	var err error
	if compress == "zstd" && decoder != nil {
		b, err = decoder.DecodeAll(b, nil)
		if err != nil {
			return nil, err
		}
	}
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

func SerializeHTTPBodyChunk(httpBodyChunk *HTTPBodyChunk, format string) ([]byte, error) {
	var err error
	var b []byte
	switch format {
	case "json":
		b, err = json.Marshal(httpBodyChunk)
	case "protobuf":
		b, err = proto.Marshal(httpBodyChunk)
	default:
		return nil, fmt.Errorf("unknown format: %s", format)
	}
	if err != nil {
		return nil, err
	}

	return b, err
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
