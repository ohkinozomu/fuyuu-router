package data

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/klauspost/compress/zstd"
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
	if format == "json" {
		payload, err = json.Marshal(packet)
	} else if format == "protobuf" {
		payload, err = proto.Marshal(packet)
	} else {
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
	if format == "json" {
		err = json.Unmarshal(payload, &requestPacket)
	} else if format == "protobuf" {
		err = proto.Unmarshal(payload, &requestPacket)
	} else {
		return nil, err
	}
	return &requestPacket, err
}

func SerializeResponsePacket(responsePacket *HTTPResponsePacket, format string) ([]byte, error) {
	var err error
	var responsePayload []byte
	if format == "json" {
		responsePayload, err = json.Marshal(responsePacket)
	} else if format == "protobuf" {
		responsePayload, err = proto.Marshal(responsePacket)
	} else {
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
	if format == "json" {
		err = json.Unmarshal(payload, &responsePacket)
	} else if format == "protobuf" {
		err = proto.Unmarshal(payload, &responsePacket)
	} else {
		return nil, fmt.Errorf("unknown format: %s", format)
	}
	return &responsePacket, err
}

func SerializeHTTPRequestData(httpRequestData *HTTPRequestData, format string, encoder *zstd.Encoder) ([]byte, error) {
	var b []byte
	var err error
	if format == "json" {
		b, err = json.Marshal(httpRequestData)
		if err != nil {
			return nil, err
		}
	} else if format == "protobuf" {
		b, err = proto.Marshal(httpRequestData)
		if err != nil {
			return nil, err
		}
	} else {
		return nil, fmt.Errorf("unknown format: %s", format)
	}
	if encoder != nil {
		b = encoder.EncodeAll(b, nil)
	}
	return b, nil
}

func DeserializeHTTPRequestData(b []byte, compress string, format string, decoder *zstd.Decoder) (*HTTPRequestData, error) {
	var err error
	if compress == "zstd" && decoder != nil {
		b, err = decoder.DecodeAll(b, nil)
		if err != nil {
			return nil, err
		}
	}

	var httpRequestData HTTPRequestData
	if format == "json" {
		if err := json.Unmarshal(b, &httpRequestData); err != nil {
			return nil, fmt.Errorf("error unmarshalling message: %v", err)
		}
	} else if format == "protobuf" {
		if err := proto.Unmarshal(b, &httpRequestData); err != nil {
			return nil, fmt.Errorf("error unmarshalling message: %v", err)
		}
	} else {
		return nil, fmt.Errorf("unknown format: %v", format)
	}
	return &httpRequestData, nil
}

func SerializeHTTPResponseData(httpResponseData *HTTPResponseData, format string, encoder *zstd.Encoder) ([]byte, error) {
	var b []byte
	var err error
	if format == "json" {
		b, err = json.Marshal(httpResponseData)
		if err != nil {
			return nil, err
		}
	} else if format == "protobuf" {
		b, err = proto.Marshal(httpResponseData)
		if err != nil {
			return nil, err
		}
	} else {
		return nil, fmt.Errorf("unknown format: %s", format)
	}
	if encoder != nil {
		b = encoder.EncodeAll(b, nil)
	}
	return b, nil
}

func DeserializeHTTPResponseData(b []byte, compress string, format string, decoder *zstd.Decoder) (*HTTPResponseData, error) {
	var err error
	if compress == "zstd" && decoder != nil {
		b, err = decoder.DecodeAll(b, nil)
		if err != nil {
			return nil, err
		}
	}
	var httpResponseData HTTPResponseData
	if format == "json" {
		if err := json.Unmarshal(b, &httpResponseData); err != nil {
			return nil, fmt.Errorf("error unmarshalling message: %v", err)
		}
	} else if format == "protobuf" {
		if err := proto.Unmarshal(b, &httpResponseData); err != nil {
			return nil, fmt.Errorf("error unmarshalling message: %v", err)
		}
	} else {
		return nil, fmt.Errorf("unknown format: %v", format)
	}
	return &httpResponseData, nil
}
