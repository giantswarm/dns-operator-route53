package metrics

import (
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/giantswarm/dns-operator-route53/pkg/cloud/awserrors"

	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"

	dnscache "github.com/giantswarm/dns-operator-route53/pkg/cloud/cache"
)

const (
	metricAWSSubsystem       = "aws"
	metricRequestCountKey    = "api_requests_total"
	metricRequestDurationKey = "api_request_duration_seconds"
	metricAPICallRetries     = "api_call_retries"
	metricServiceLabel       = "service"
	metricOperationLabel     = "operation"
	metricControllerLabel    = "controller"
	metricStatusCodeLabel    = "status_code"
	metricErrorCodeLabel     = "error_code"
	metricCacheSubsystem     = "cache"
)

var (
	awsRequestCount = prometheus.NewCounterVec(prometheus.CounterOpts{
		Subsystem: metricAWSSubsystem,
		Name:      metricRequestCountKey,
		Help:      "Total number of AWS requests",
	}, []string{metricControllerLabel, metricServiceLabel, metricOperationLabel, metricStatusCodeLabel, metricErrorCodeLabel})
	awsRequestDurationSeconds = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Subsystem: metricAWSSubsystem,
		Name:      metricRequestDurationKey,
		Help:      "Latency of HTTP requests to AWS",
	}, []string{metricControllerLabel, metricServiceLabel, metricOperationLabel})
	awsCallRetries = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Subsystem: metricAWSSubsystem,
		Name:      metricAPICallRetries,
		Help:      "Number of retries made against an AWS API",
		Buckets:   []float64{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
	}, []string{metricControllerLabel, metricServiceLabel, metricOperationLabel})
	cacheItems = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Subsystem: metricCacheSubsystem,
		Name:      "items",
		Help:      "Number of items in the cache",
	}, []string{metricControllerLabel})
	cacheHits = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Subsystem: metricCacheSubsystem,
		Name:      "hits",
		Help:      "Number of cache hits",
	}, []string{metricControllerLabel})
	cacheSize = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Subsystem: metricCacheSubsystem,
		Name:      "size",
		Help:      "Size of cache in bytes",
	}, []string{metricControllerLabel})
)

func init() {
	metrics.Registry.MustRegister(awsRequestCount)
	metrics.Registry.MustRegister(awsRequestDurationSeconds)
	metrics.Registry.MustRegister(awsCallRetries)
	metrics.Registry.MustRegister(cacheItems)
	metrics.Registry.MustRegister(cacheHits)
	metrics.Registry.MustRegister(cacheSize)
}

func CaptureRequestMetrics(controller string) func(r *request.Request) {
	return func(r *request.Request) {
		duration := time.Since(r.AttemptTime)
		operation := r.Operation.Name
		service := endpointToService(r.ClientInfo.Endpoint)
		statusCode := "0"
		errorCode := ""
		if r.HTTPResponse != nil {
			statusCode = strconv.Itoa(r.HTTPResponse.StatusCode)
		}
		if r.Error != nil {
			var ok bool
			if errorCode, ok = awserrors.Code(r.Error); !ok {
				errorCode = "internal"
			}
		}
		awsRequestCount.WithLabelValues(controller, service, operation, statusCode, errorCode).Inc()
		awsRequestDurationSeconds.WithLabelValues(controller, service, operation).Observe(duration.Seconds())
		awsCallRetries.WithLabelValues(controller, service, operation).Observe(float64(r.RetryCount))
		cacheItems.WithLabelValues(controller).Set(float64(dnscache.DNSOperatorCache.Len()))
		cacheHits.WithLabelValues(controller).Set(float64(dnscache.DNSOperatorCache.Stats().Hits))
		cacheSize.WithLabelValues(controller).Set(float64(dnscache.DNSOperatorCache.Capacity()))
	}
}

func endpointToService(endpoint string) string {
	endpointURL, err := url.Parse(endpoint)
	// If possible extract the service name, else return entire endpoint address
	if err == nil {
		host := endpointURL.Host
		components := strings.Split(host, ".")
		if len(components) > 0 {
			return components[0]
		}
	}
	return endpoint
}
