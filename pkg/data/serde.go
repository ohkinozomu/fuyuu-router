package data

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/thanos-io/objstore"
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
		// fallback
		if err != nil {
			payload, err = packet.MarshalVT()
			if err != nil {
				return nil, err
			}
		}
	case "protobuf":
		payload, err = packet.MarshalVT()
		// fallback
		if err != nil {
			payload, err = json.Marshal(packet)
			if err != nil {
				return nil, err
			}
		}
	default:
		return nil, fmt.Errorf("unknown format: %s", format)
	}

	return payload, nil
}

func DeserializeRequestPacket(payload []byte, format string) (*HTTPRequestPacket, error) {
	var err error
	requestPacket := HTTPRequestPacket{}
	switch format {
	case "json":
		err = json.Unmarshal(payload, &requestPacket)
		// fallback
		if err != nil {
			err = requestPacket.UnmarshalVT(payload)
			if err != nil {
				return &requestPacket, err
			}
		}
	case "protobuf":
		err = requestPacket.UnmarshalVT(payload)
		// fallback
		if err != nil {
			err = json.Unmarshal(payload, &requestPacket)
			if err != nil {
				return &requestPacket, err
			}
		}
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
		// fallback
		if err != nil {
			responsePayload, err = responsePacket.MarshalVT()
			if err != nil {
				return nil, err
			}
		}
	case "protobuf":
		responsePayload, err = responsePacket.MarshalVT()
		// fallback
		if err != nil {
			responsePayload, err = json.Marshal(responsePacket)
			if err != nil {
				return nil, err
			}
		}
	default:
		return nil, fmt.Errorf("unknown format: %s", format)
	}

	return responsePayload, err
}

func DeserializeResponsePacket(payload []byte, format string) (*HTTPResponsePacket, error) {
	var err error
	responsePacket := HTTPResponsePacket{}
	switch format {
	case "json":
		err = json.Unmarshal(payload, &responsePacket)
		// fallback
		if err != nil {
			err = responsePacket.UnmarshalVT(payload)
			if err != nil {
				return &responsePacket, err
			}
		}
	case "protobuf":
		err = responsePacket.UnmarshalVT(payload)
		// fallback
		if err != nil {
			err = json.Unmarshal(payload, &responsePacket)
			if err != nil {
				return &responsePacket, err
			}
		}
	default:
		return nil, fmt.Errorf("unknown format: %s", format)
	}
	return &responsePacket, err
}

func SerializeHTTPRequestData(httpRequestData *HTTPRequestData, format string) ([]byte, error) {
	var b []byte
	var err error
	switch format {
	case "json":
		b, err = json.Marshal(httpRequestData)
		// fallback
		if err != nil {
			b, err = httpRequestData.MarshalVT()
			if err != nil {
				return nil, err
			}
		}
	case "protobuf":
		b, err = httpRequestData.MarshalVT()
		// fallback
		if err != nil {
			b, err = json.Marshal(httpRequestData)
			if err != nil {
				return nil, err
			}
		}
	default:
		return nil, fmt.Errorf("unknown format: %s", format)
	}
	return b, nil
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
	var httpRequestData HTTPRequestData
	switch format {
	case "json":
		err := json.Unmarshal(b, &httpRequestData)
		if err != nil {
			// fallback
			if err := httpRequestData.UnmarshalVT(b); err != nil {
				return nil, fmt.Errorf("error unmarshalling message: %v", err)
			}
			return nil, fmt.Errorf("error unmarshalling message: %v", err)
		}
	case "protobuf":
		err := httpRequestData.UnmarshalVT(b)
		if err != nil {
			// fallback
			if err := json.Unmarshal(b, &httpRequestData); err != nil {
				return nil, fmt.Errorf("error unmarshalling message: %v", err)
			}
			return nil, fmt.Errorf("error unmarshalling message: %v", err)
		}
	default:
		return nil, fmt.Errorf("unknown format: %v", format)
	}

	if httpRequestData.Body.Type == "storage_relay" {
		data, err := decodeStorageRelay(httpRequestData.Body.Body, bucket)
		if err != nil {
			return nil, err
		}
		httpRequestData.Body.Body = data
	}

	return &httpRequestData, nil
}

func SerializeHTTPResponseData(httpResponseData *HTTPResponseData, format string) ([]byte, error) {
	var b []byte
	var err error
	switch format {
	case "json":
		b, err = json.Marshal(httpResponseData)
		if err != nil {
			// fallback
			b, err = httpResponseData.MarshalVT()
			if err != nil {
				return nil, err
			}
		}
	case "protobuf":
		b, err = httpResponseData.MarshalVT()
		if err != nil {
			// fallback
			b, err = json.Marshal(httpResponseData)
			if err != nil {
				return nil, err
			}
		}
	default:
		return nil, fmt.Errorf("unknown format: %s", format)
	}
	return b, nil
}

func DeserializeHTTPResponseData(b []byte, format string, bucket objstore.Bucket) (*HTTPResponseData, error) {
	var httpResponseData HTTPResponseData
	switch format {
	case "json":
		err := json.Unmarshal(b, &httpResponseData)
		if err != nil {
			// fallback
			if err := httpResponseData.UnmarshalVT(b); err != nil {
				return nil, fmt.Errorf("error unmarshalling message: %v", err)
			}
			return nil, fmt.Errorf("error unmarshalling message: %v", err)
		}
	case "protobuf":
		err := httpResponseData.UnmarshalVT(b)
		if err != nil {
			// fallback
			if err := json.Unmarshal(b, &httpResponseData); err != nil {
				return nil, fmt.Errorf("error unmarshalling message: %v", err)
			}
			return nil, fmt.Errorf("error unmarshalling message: %v", err)
		}
	default:
		return nil, fmt.Errorf("unknown format: %v", format)
	}

	if httpResponseData.Body.Type == "storage_relay" {
		data, err := decodeStorageRelay(httpResponseData.Body.Body, bucket)
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
		// fallback
		if err != nil {
			b, err = httpBodyChunk.MarshalVT()
			if err != nil {
				return nil, err
			}
		}
	case "protobuf":
		b, err = httpBodyChunk.MarshalVT()
		// fallback
		if err != nil {
			b, err = json.Marshal(httpBodyChunk)
			if err != nil {
				return nil, err
			}
		}
	default:
		return nil, fmt.Errorf("unknown format: %s", format)
	}

	return b, err
}

func DeserializeHTTPBodyChunk(payload []byte, format string) (*HTTPBodyChunk, error) {
	var err error
	httpBodyChunk := HTTPBodyChunk{}
	switch format {
	case "json":
		err = json.Unmarshal(payload, &httpBodyChunk)
		if err != nil {
			// fallback
			err = httpBodyChunk.UnmarshalVT(payload)
			if err != nil {
				return nil, err
			}
		}
	case "protobuf":
		err = httpBodyChunk.UnmarshalVT(payload)
		if err != nil {
			// fallback
			err = json.Unmarshal(payload, &httpBodyChunk)
			if err != nil {
				return nil, err
			}
		}
	default:
		return nil, fmt.Errorf("unknown format: %s", format)
	}
	return &httpBodyChunk, nil
}
