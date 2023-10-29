package cmd

import (
	"github.com/ohkinozomu/fuyuu-router/internal/common"
	"github.com/ohkinozomu/fuyuu-router/internal/hub"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func init() {
	rootCmd.AddCommand(hubCmd)
	hubCmd.Flags().StringVarP(&mqttBroker, "mqtt-broker", "b", "", "MQTT Broker URL")
	hubCmd.Flags().StringVarP(&username, "username", "u", "", "user name")
	hubCmd.Flags().StringVarP(&password, "password", "p", "", "password")
	hubCmd.Flags().StringVarP(&logLevel, "loglevel", "l", "info", "set log level (debug, info, warn, error, panic, fatal)")
	hubCmd.Flags().StringVarP(&caFile, "cafile", "", "", "CA file")
	hubCmd.Flags().StringVarP(&cert, "cert", "", "", "cert file")
	hubCmd.Flags().StringVarP(&key, "key", "", "", "key file")
	hubCmd.Flags().StringVarP(&protocol, "protocol", "", "http1", "Currently only \"http1\" is supported")
}

var hubCmd = &cobra.Command{
	Use:   "hub",
	Short: "hub",
	Long:  `hub`,
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

		c := hub.HubConfig{
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
		hub.Start(c)
		return nil
	},
}
