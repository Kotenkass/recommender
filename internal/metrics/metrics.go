package metrics

import (
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// Recorder owns Prometheus metrics used by the recommender.
type Recorder struct {
	Registry   *prometheus.Registry
	Sent       prometheus.Counter
	Failed     prometheus.Counter
	LLMLatency prometheus.Histogram
}

var (
	defaultMu          sync.Mutex
	defaultRecorder    *Recorder
	defaultRecorderSet bool
)

// NewRecorder creates a fresh recorder with its own Prometheus registry.
func NewRecorder() *Recorder {
	return NewRecorderWithRegistry(prometheus.NewRegistry())
}

// NewRecorderWithRegistry creates a recorder backed by a specific Prometheus registry.
func NewRecorderWithRegistry(registry *prometheus.Registry) *Recorder {
	if registry == nil {
		registry = prometheus.NewRegistry()
	}
	sent := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "reco_sent_total",
		Help: "Total number of recommendations successfully published to Redis send_message.",
	})
	failed := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "reco_failed_total",
		Help: "Total number of failed recommendation attempts by chat_id.",
	})
	latency := prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "llm_latency_seconds",
		Help:    "OpenRouter LLM request latency in seconds.",
		Buckets: prometheus.DefBuckets,
	})
	registry.MustRegister(sent, failed, latency)
	return &Recorder{Registry: registry, Sent: sent, Failed: failed, LLMLatency: latency}
}

// NewDefaultRecorder returns a process-wide recorder registered on the default Prometheus registry.
func NewDefaultRecorder() *Recorder {
	defaultMu.Lock()
	defer defaultMu.Unlock()
	if defaultRecorderSet {
		return defaultRecorder
	}
	sent := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "reco_sent_total",
		Help: "Total number of recommendations successfully published to Redis send_message.",
	})
	failed := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "reco_failed_total",
		Help: "Total number of failed recommendation attempts by chat_id.",
	})
	latency := prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "llm_latency_seconds",
		Help:    "OpenRouter LLM request latency in seconds.",
		Buckets: prometheus.DefBuckets,
	})
	prometheus.MustRegister(sent, failed, latency)
	defaultRecorder = &Recorder{Registry: prometheus.DefaultRegisterer.(*prometheus.Registry), Sent: sent, Failed: failed, LLMLatency: latency}
	defaultRecorderSet = true
	return defaultRecorder
}

func (r *Recorder) SentRecommendation() {
	r.Sent.Inc()
}

func (r *Recorder) FailedRecommendation() {
	r.Failed.Inc()
}

func (r *Recorder) ObserveLLM(_ string, latency time.Duration) {
	r.LLMLatency.Observe(latency.Seconds())
}
