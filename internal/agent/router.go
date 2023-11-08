package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/eclipse/paho.golang/packets"
	"github.com/eclipse/paho.golang/paho"
	"github.com/ohkinozomu/fuyuu-router/internal/common"
	"github.com/ohkinozomu/fuyuu-router/pkg/data"
	"github.com/ohkinozomu/fuyuu-router/pkg/topics"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

type Router struct {
	messageChan chan string
	client      *paho.Client
	id          string
	proxyHost   string
	logger      *zap.Logger
	protocol    string
	format      string
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

	return &Router{
		messageChan: messageChan,
		client:      client,
		id:          c.ID,
		proxyHost:   c.ProxyHost,
		logger:      c.Logger,
		protocol:    c.Protocol,
		format:      c.CommonConfigV2.Networking.Format,
	}
}

func sendHTTP1Request(proxyHost string, data *data.HTTPRequestData) (string, int, error) {
	url := "http://" + proxyHost + data.Path
	body := bytes.NewBufferString(data.Body)
	req, err := http.NewRequest(data.Method, url, body)
	if err != nil {
		return "", 0, err
	}
	for key, values := range data.Headers.GetHeaders() {
		for _, value := range values.GetValues() {
			req.Header.Add(key, value)
		}
	}
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	responseBody := new(bytes.Buffer)
	_, err = responseBody.ReadFrom(resp.Body)
	if err != nil {
		return "", resp.StatusCode, err
	}
	return responseBody.String(), resp.StatusCode, nil
}

func (r *Router) Route(p *packets.Publish) {
	requestPacket := data.HTTPRequestPacket{}

	if r.format == "json" {
		if err := json.Unmarshal(p.Payload, &requestPacket); err != nil {
			r.logger.Error("Error unmarshalling message", zap.Error(err))
			return
		}
	} else if r.format == "protobuf" {
		if err := proto.Unmarshal(p.Payload, &requestPacket); err != nil {
			r.logger.Error("Error unmarshalling message", zap.Error(err))
			return
		}
	} else {
		r.logger.Error("Unknown format: " + r.format)
		return
	}

	var responsePacket data.HTTPResponsePacket

	if r.protocol == "http1" {
		var responseData data.HTTPResponseData
		httpResponse, statusCode, err := sendHTTP1Request(r.proxyHost, requestPacket.HttpRequestData)
		if err != nil {
			r.logger.Error("Error sending HTTP request", zap.Error(err))
			responseData = data.HTTPResponseData{
				Body:       err.Error(),
				StatusCode: http.StatusInternalServerError,
			}
		} else {
			responseData = data.HTTPResponseData{
				Body:       httpResponse,
				StatusCode: int32(statusCode),
			}
		}

		responsePacket = data.HTTPResponsePacket{
			RequestId:        requestPacket.RequestId,
			HttpResponseData: &responseData,
		}
	} else {
		r.logger.Error("Unknown protocol: " + r.protocol)
		return
	}

	responseTopic := topics.ResponseTopic(r.id, requestPacket.RequestId)

	var responsePayload []byte
	var err error
	if r.format == "json" {
		responsePayload, err = json.Marshal(&responsePacket)
		if err != nil {
			r.logger.Fatal(err.Error())
		}
	} else if r.format == "protobuf" {
		responsePayload, err = proto.Marshal(&responsePacket)
		if err != nil {
			r.logger.Fatal(err.Error())
		}
	} else {
		r.logger.Fatal("Unknown format: " + r.format)
	}

	if err != nil {
		r.logger.Error("Error marshalling response data", zap.Error(err))
		return
	}

	_, err = r.client.Publish(context.Background(), &paho.Publish{
		Topic:   responseTopic,
		QoS:     0,
		Payload: responsePayload,
	})
	if err != nil {
		panic(err)
	}
}

func (r *Router) RegisterHandler(string, paho.MessageHandler) {}

func (r *Router) UnregisterHandler(string) {}

func (r *Router) SetDebugLogger(paho.Logger) {}
