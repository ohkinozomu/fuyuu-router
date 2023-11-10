package agent

import (
	"bytes"
	"context"
	"net/http"
	"time"

	"github.com/eclipse/paho.golang/packets"
	"github.com/eclipse/paho.golang/paho"
	"github.com/klauspost/compress/zstd"
	"github.com/ohkinozomu/fuyuu-router/internal/common"
	"github.com/ohkinozomu/fuyuu-router/pkg/data"
	"github.com/ohkinozomu/fuyuu-router/pkg/topics"
	"go.uber.org/zap"
)

type Router struct {
	messageChan chan string
	client      *paho.Client
	id          string
	proxyHost   string
	logger      *zap.Logger
	protocol    string
	format      string
	encoder     *zstd.Encoder
	decoder     *zstd.Decoder
}

var _ paho.Router = (*Router)(nil)

func NewRouter(messageChan chan string, c AgentConfig) *Router {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	conn, err := common.TCPConnect(ctx, c.CommonConfig)
	if err != nil {
		c.Logger.Fatal("Error: " + err.Error())
	}
	clientConfig := paho.ClientConfig{
		Conn: conn,
	}
	client := paho.NewClient(clientConfig)

	_, err = client.Connect(context.Background(), common.MQTTConnect(c.CommonConfig))
	if err != nil {
		c.Logger.Fatal("Error connecting to MQTT broker: " + err.Error())
	}

	var encoder *zstd.Encoder
	var decoder *zstd.Decoder
	if c.CommonConfigV2.Networking.Compress == "zstd" {
		encoder, err = zstd.NewWriter(nil)
		if err != nil {
			c.Logger.Fatal(err.Error())
		}

		decoder, err = zstd.NewReader(nil)
		if err != nil {
			c.Logger.Fatal(err.Error())
		}
	}

	return &Router{
		messageChan: messageChan,
		client:      client,
		id:          c.ID,
		proxyHost:   c.ProxyHost,
		logger:      c.Logger,
		protocol:    c.Protocol,
		format:      c.CommonConfigV2.Networking.Format,
		encoder:     encoder,
		decoder:     decoder,
	}
}

func sendHTTP1Request(proxyHost string, data *data.HTTPRequestData) (string, int, http.Header, error) {
	var responseHeader http.Header

	url := "http://" + proxyHost + data.Path
	body := bytes.NewBufferString(data.Body)
	req, err := http.NewRequest(data.Method, url, body)
	if err != nil {
		return "", http.StatusInternalServerError, responseHeader, err
	}
	for key, values := range data.Headers.GetHeaders() {
		for _, value := range values.GetValues() {
			req.Header.Add(key, value)
		}
	}
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", http.StatusInternalServerError, responseHeader, err
	}
	defer resp.Body.Close()
	responseBody := new(bytes.Buffer)
	_, err = responseBody.ReadFrom(resp.Body)
	if err != nil {
		return "", resp.StatusCode, responseHeader, err
	}
	return responseBody.String(), resp.StatusCode, resp.Header, nil
}

func (r *Router) Route(p *packets.Publish) {
	requestPacket, err := data.DeserializeRequestPacket(p.Payload, r.format)
	if err != nil {
		r.logger.Error("Error deserializing request packet", zap.Error(err))
		return
	}

	var responsePacket data.HTTPResponsePacket
	httpRequestData, err := data.DeserializeHTTPRequestData(requestPacket.HttpRequestData, r.format, r.decoder)
	if err != nil {
		r.logger.Error("Error deserializing request data", zap.Error(err))
		return
	}

	if r.protocol == "http1" {
		var responseData data.HTTPResponseData
		httpResponse, statusCode, responseHeader, err := sendHTTP1Request(r.proxyHost, httpRequestData)
		if err != nil {
			r.logger.Error("Error sending HTTP request", zap.Error(err))
			protoHeaders := data.HTTPHeaderToProtoHeaders(responseHeader)
			responseData = data.HTTPResponseData{
				Body:       err.Error(),
				StatusCode: http.StatusInternalServerError,
				Headers:    &protoHeaders,
			}
		} else {
			protoHeaders := data.HTTPHeaderToProtoHeaders(responseHeader)
			responseData = data.HTTPResponseData{
				Body:       httpResponse,
				StatusCode: int32(statusCode),
				Headers:    &protoHeaders,
			}
		}
		b, err := data.SerializeHTTPResponseData(&responseData, r.format, r.encoder)
		if err != nil {
			r.logger.Error("Error serializing response data", zap.Error(err))
			return
		}
		responsePacket = data.HTTPResponsePacket{
			RequestId:        requestPacket.RequestId,
			HttpResponseData: b,
		}
	} else {
		r.logger.Error("Unknown protocol: " + r.protocol)
		return
	}

	responseTopic := topics.ResponseTopic(r.id, requestPacket.RequestId)

	responsePayload, err := data.SerializeResponsePacket(&responsePacket, r.format)
	if err != nil {
		r.logger.Error("Error serializing response packet", zap.Error(err))
		return
	}
	_, err = r.client.Publish(context.Background(), &paho.Publish{
		Topic:   responseTopic,
		QoS:     0,
		Payload: responsePayload,
	})
	if err != nil {
		r.logger.Error("Error publishing response", zap.Error(err))
		return
	}
}

func (r *Router) RegisterHandler(string, paho.MessageHandler) {}

func (r *Router) UnregisterHandler(string) {}

func (r *Router) SetDebugLogger(paho.Logger) {}
