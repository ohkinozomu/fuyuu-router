package cmd

import (
	"context"
	"strings"

	"github.com/ohkinozomu/fuyuu-router/internal/agent"
	"github.com/ohkinozomu/fuyuu-router/internal/common"
	ncp "github.com/ohkinozomu/neutral-cp"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var id string
var proxyHost string
var labels []string

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
	agentCmd.Flags().StringArrayVar(&labels, "labels", []string{}, "Assign labels to the agent (e.g., --labels key1=value1,key2=value2)")
	agentCmd.Flags().StringVarP(&configPath, "config", "c", "", "config file path")
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

		loggerConfig := zap.Config{
			Level:            atomicLevel,
			Encoding:         "json",
			EncoderConfig:    encoderConfig,
			OutputPaths:      []string{"stderr"},
			ErrorOutputPaths: []string{"stderr"},
		}

		logger, err := loggerConfig.Build()
		if err != nil {
			panic(err)
		}

		config, err := common.CreateConfig(configPath)
		if err != nil {
			logger.Fatal(err.Error())
		}

		if config.Profiling.Registry != "" {
			logger.Info("profiling enabled")
			go func() {
				var registry ncp.Registry
				switch config.Profiling.Registry {
				case "cloudprofiler":
					registry = ncp.CLOUD_PROFILER
				case "pyroscope":
					registry = ncp.PYROSCOPE
				default:
					logger.Fatal("Unknown profiling registry")
				}
				c := ncp.Config{
					Registry:        registry,
					ApplicationName: "fuyuu-router-agent",
					ServerAddress:   config.Profiling.ServerAddress,
					Version:         "TODO",
				}
				ncp := ncp.NeutralCP{Config: c}

				ctx := context.Background()
				err := ncp.Start(ctx)
				if err != nil {
					logger.Fatal(err.Error())
				}
			}()
		}

		if id == "" {
			logger.Fatal("ID is required")
		}

		labelMap := make(map[string]string)
		for _, label := range labels {
			parts := strings.SplitN(label, "=", 2)
			if len(parts) != 2 {
				logger.Fatal("Invalid label format")
			}
			labelMap[parts[0]] = parts[1]
		}

		c := agent.AgentConfig{
			ID:        id,
			ProxyHost: proxyHost,
			Labels:    labelMap,
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
			CommonConfigV2: config,
		}
		agent.Start(c)
		return nil
	},
}
