package common

func RequestObjectName(agentID, requstID string) string {
	return agentID + "/" + requstID + "/request"
}
func ResponseObjectName(agentID, requstID string) string {
	return agentID + "/" + requstID + "/response"
}
