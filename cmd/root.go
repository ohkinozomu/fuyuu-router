package cmd

import (
	"github.com/spf13/cobra"
)

var (
	rootCmd = &cobra.Command{
		Use:   "fuyuu-router",
		Short: "fuyuu-router",
		Long:  `fuyuu-router`,
	}
	mqttBroker string
	username   string
	password   string
	logLevel   string
	caFile     string
	cert       string
	key        string
	protocol   string
)

func Execute() error {
	return rootCmd.Execute()
}
