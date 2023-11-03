package common

import "fmt"

func RequestTopic(agentID string) string {
	return fmt.Sprintf("fuyuu-router/agent/%s/request", agentID)
}

func ResponseTopic(agentID string, requestID string) string {
	return fmt.Sprintf("fuyuu-router/agent/%s/response/%s", agentID, requestID)
}
