package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"

	"github.com/eclipse/paho.golang/packets"
	"github.com/eclipse/paho.golang/paho"
	"github.com/ohkinozomu/fuyuu-router/internal/common"
	"github.com/ohkinozomu/fuyuu-router/internal/data"
	"go.uber.org/zap"
)

type Router struct {
	messageChan chan string
	client      *paho.Client
	id          string
	proxyHost   string
	logger      *zap.Logger
}

var _ paho.Router = (*Router)(nil)

func NewRouter(messageChan chan string, c AgentConfig) *Router {
	conn, err := common.TCPConnect(c.CommonConfig)
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
	}
}

func sendHTTPRequest(proxyHost string, data data.HTTPRequestData) (string, int, error) {
	url := "http://" + proxyHost + data.Path
	body := bytes.NewBufferString(data.Body)
	req, err := http.NewRequest(data.Method, url, body)
	if err != nil {
		return "", 0, err
	}
	for key, values := range data.Headers {
		for _, value := range values {
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
	if err := json.Unmarshal(p.Payload, &requestPacket); err != nil {
		r.logger.Error("Error unmarshalling message", zap.Error(err))
		return
	}

	var responseData data.HTTPResponseData
	httpResponse, statusCode, err := sendHTTPRequest(r.proxyHost, requestPacket.HTTPRequestData)
	if err != nil {
		r.logger.Error("Error sending HTTP request", zap.Error(err))
		responseData = data.HTTPResponseData{
			Body:       err.Error(),
			StatusCode: http.StatusInternalServerError,
		}
	} else {
		responseData = data.HTTPResponseData{
			Body:       httpResponse,
			StatusCode: statusCode,
		}
	}

	responsePacket := data.HTTPResponsePacket{
		RequestID:        requestPacket.RequestID,
		HTTPResponseData: responseData,
	}

	responseTopic := common.ResponseTopic(r.id, requestPacket.RequestID)
	responsePayload, err := json.Marshal(responsePacket)
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
