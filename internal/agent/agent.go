package agent

import (
	"context"
	"encoding/json"
	"time"

	"github.com/eclipse/paho.golang/paho"
	"github.com/ohkinozomu/fuyuu-router/internal/common"
	"github.com/ohkinozomu/fuyuu-router/pkg/data"
	"github.com/ohkinozomu/fuyuu-router/pkg/topics"
)

type server struct {
	client      *paho.Client
	messageChan chan string
}

func newServer(c AgentConfig) server {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	conn, err := common.TCPConnect(ctx, c.CommonConfig)
	if err != nil {
		c.Logger.Fatal("Error: " + err.Error())
	}
	messageChan := make(chan string)
	clientConfig := paho.ClientConfig{
		Conn:   conn,
		Router: NewRouter(messageChan, c),
	}
	client := paho.NewClient(clientConfig)

	return server{
		client:      client,
		messageChan: messageChan,
	}
}

func Start(c AgentConfig) {
	if c.Protocol != "http1" {
		c.Logger.Fatal("Unknown protocol: " + c.Protocol)
	}

	s := newServer(c)
	connect := common.MQTTConnect(c.CommonConfig)
	teminatePacket := data.TerminatePacket{
		AgentId: c.ID,
		Labels:  c.Labels,
	}

	terminatePayload, err := json.Marshal(&teminatePacket)
	if err != nil {
		c.Logger.Fatal(err.Error())
	}

	connect.WillMessage = &paho.WillMessage{
		Retain:  false,
		QoS:     0,
		Topic:   topics.TerminateTopic(),
		Payload: terminatePayload,
	}
	_, err = s.client.Connect(context.Background(), connect)
	if err != nil {
		c.Logger.Fatal("Error connecting to MQTT broker: " + err.Error())
	}

	launchPacket := data.LaunchPacket{
		AgentId: c.ID,
		Labels:  c.Labels,
	}
	launchPayload, err := json.Marshal(&launchPacket)
	if err != nil {
		c.Logger.Fatal(err.Error())
	}

	_, err = s.client.Publish(context.Background(), &paho.Publish{
		Topic: topics.LaunchTopic(),
		// Maybe it should be 1.
		QoS:     0,
		Payload: launchPayload,
	})
	if err != nil {
		c.Logger.Error(err.Error())
	}

	_, err = s.client.Subscribe(context.Background(), &paho.Subscribe{
		Subscriptions: []paho.SubscribeOptions{
			{
				Topic: topics.RequestTopic(c.ID),
				QoS:   0,
			},
		},
	})
	if err != nil {
		c.Logger.Fatal(err.Error())
	}
	select {}
}
