package data

import (
	"encoding/json"
	"fmt"
	"net/http"

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
		return &HTTPRequestPacket{}, err
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
		return &HTTPResponsePacket{}, fmt.Errorf("unknown format: %s", format)
	}
	return &responsePacket, err
}

func SerializeHTTPResponseData(httpResponseData *HTTPResponseData, format string) ([]byte, error) {
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
	return b, nil
}

func DeserializeHTTPResponseData(b []byte, format string) (*HTTPResponseData, error) {
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
