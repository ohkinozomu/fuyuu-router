package agent

import "github.com/ohkinozomu/fuyuu-router/internal/common"

type AgentConfig struct {
	ID        string
	ProxyHost string
	Labels    map[string]string
	common.CommonConfig
}
