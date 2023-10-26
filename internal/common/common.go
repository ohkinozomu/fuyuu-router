package common

import "fmt"

func RequestTopic(agentID string) string {
	return fmt.Sprintf("%s/request", agentID)
}

func ResponseTopic(agentID string, requestID string) string {
	return fmt.Sprintf("%s/response/%s", agentID, requestID)
}
