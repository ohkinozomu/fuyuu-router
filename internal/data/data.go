package data

import "net/http"

type HTTPRequestData struct {
	Method  string      `json:"method"`
	Path    string      `json:"path"`
	Headers http.Header `json:"headers"`
	Body    string      `json:"body"`
}

type HTTPRequestPacket struct {
	RequestID       string          `json:"request_id"`
	HTTPRequestData HTTPRequestData `json:"http_request_data"`
}

type HTTPResponseData struct {
	Body       string `json:"body"`
	StatusCode int    `json:"status_code"`
}

type HTTPResponsePacket struct {
	RequestID        string           `json:"request_id"`
	HTTPResponseData HTTPResponseData `json:"http_response_data"`
}

type LaunchPacket struct {
	AgentID string `json:"agent_id"`
}

type TerminatePacket struct {
	AgentID string `json:"agent_id"`
}
