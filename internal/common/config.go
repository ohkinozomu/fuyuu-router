package common

import "go.uber.org/zap"

type CommonConfig struct {
	MQTTBroker string
	Username   string
	Password   string
	Logger     *zap.Logger
	CAFile     string
	Cert       string
	Key        string
	Protocol   string
}

type CommonConfigV2 struct {
	Profiling struct {
		Registry      string `mapstructure:"registry"`
		ServerAddress string `mapstructure:"server_address"`
	}
}
