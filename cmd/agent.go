package cmd

import (
	"github.com/ohkinozomu/fuyuu-router/internal/agent"
	"github.com/ohkinozomu/fuyuu-router/internal/common"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var id string
var proxyHost string

func init() {
	rootCmd.AddCommand(agentCmd)
	agentCmd.Flags().StringVarP(&id, "id", "", "", "ID of this agent")
	agentCmd.Flags().StringVarP(&mqttBroker, "mqtt-broker", "b", "", "MQTT Broker URL")
	agentCmd.Flags().StringVarP(&proxyHost, "proxy-host", "", "", "Proxy host")
	agentCmd.Flags().StringVarP(&username, "username", "u", "", "user name")
	agentCmd.Flags().StringVarP(&password, "password", "p", "", "password")
	agentCmd.Flags().StringVarP(&logLevel, "loglevel", "l", "info", "set log level (debug, info, warn, error, panic, fatal)")
	agentCmd.Flags().StringVarP(&caFile, "cafile", "", "", "CA file")
	agentCmd.Flags().StringVarP(&cert, "cert", "", "", "cert file")
	agentCmd.Flags().StringVarP(&key, "key", "", "", "key file")
	agentCmd.Flags().StringVarP(&protocol, "protocol", "", "http1", "Currently only \"http1\" is supported")
}

var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "agent",
	Long:  `agent`,
	RunE: func(cmd *cobra.Command, args []string) error {
		atomicLevel := zap.NewAtomicLevel()

		switch logLevel {
		case "debug":
			atomicLevel.SetLevel(zap.DebugLevel)
		case "info":
			atomicLevel.SetLevel(zap.InfoLevel)
		case "warn":
			atomicLevel.SetLevel(zap.WarnLevel)
		case "error":
			atomicLevel.SetLevel(zap.ErrorLevel)
		case "panic":
			atomicLevel.SetLevel(zap.PanicLevel)
		case "fatal":
			atomicLevel.SetLevel(zap.FatalLevel)
		default:
			panic("Unknown log level: " + logLevel)
		}

		encoderConfig := zap.NewProductionEncoderConfig()
		encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

		config := zap.Config{
			Level:            atomicLevel,
			Encoding:         "json",
			EncoderConfig:    encoderConfig,
			OutputPaths:      []string{"stderr"},
			ErrorOutputPaths: []string{"stderr"},
		}

		logger, err := config.Build()
		if err != nil {
			panic(err)
		}

		c := agent.AgentConfig{
			ID:        id,
			ProxyHost: proxyHost,
			CommonConfig: common.CommonConfig{
				MQTTBroker: mqttBroker,
				Username:   username,
				Password:   password,
				Logger:     logger,
				CAFile:     caFile,
				Cert:       cert,
				Key:        key,
				Protocol:   protocol,
			},
		}
		agent.Start(c)
		return nil
	},
}
