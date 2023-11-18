package common

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
)

func ServeMetrics(logger *zap.Logger) {
	http.Handle("/metrics", promhttp.Handler())
	err := http.ListenAndServe(":2223", nil)
	if err != nil {
		logger.Error("Error serving metrics: " + err.Error())
	}
}
