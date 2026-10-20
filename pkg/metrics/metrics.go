package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

// Namespace for all metrics.
const Namespace = "raspishika"

// Counters for proxy lifecycle events.
var (
	// ProxyRestartsTotal counts how many times the bot was restarted on a
	// (possibly different) proxy due to a connection failure.
	ProxyRestartsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: Namespace,
			Name:      "proxy_restarts_total",
			Help:      "Number of Telegram bot restarts caused by proxy failure.",
		},
		[]string{"reason"},
	)

	// ProxySwitchesTotal counts how many times the bot switched to a new
	// proxy (different from the previous one).
	ProxySwitchesTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: Namespace,
			Name:      "proxy_switches_total",
			Help:      "Number of proxy switches performed by the Telegram bot.",
		},
		[]string{"from", "to"},
	)
)

// Counters for broadcast coverage and delivery.
var (
	// BroadcastChatsTotal counts how many chats were considered for a
	// broadcast of a given kind.
	BroadcastChatsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: Namespace,
			Name:      "broadcast_chats_total",
			Help:      "Number of chats considered for broadcast, by kind.",
		},
		[]string{"kind"},
	)

	// BroadcastChatsDisabledTotal counts how many chats were skipped because
	// their subscription was disabled due to permanent delivery errors.
	BroadcastChatsDisabledTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: Namespace,
			Name:      "broadcast_chats_disabled_total",
			Help:      "Number of chats skipped from broadcast due to disabled subscriptions, by kind.",
		},
		[]string{"kind"},
	)

	// BroadcastSentTotal counts how many broadcast messages were sent
	// successfully.
	BroadcastSentTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: Namespace,
			Name:      "broadcast_sent_total",
			Help:      "Number of broadcast messages sent successfully, by kind.",
		},
		[]string{"kind"},
	)

	// BroadcastFailedTotal counts how many broadcast messages failed to send.
	BroadcastFailedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: Namespace,
			Name:      "broadcast_failed_total",
			Help:      "Number of broadcast messages that failed to send, by kind.",
		},
		[]string{"kind"},
	)
)

// Register registers all metrics with the given Prometheus registerer.
func Register(r prometheus.Registerer) error {
	metrics := []prometheus.Collector{
		ProxyRestartsTotal,
		ProxySwitchesTotal,
		BroadcastChatsTotal,
		BroadcastChatsDisabledTotal,
		BroadcastSentTotal,
		BroadcastFailedTotal,
	}
	for _, m := range metrics {
		if err := r.Register(m); err != nil {
			return err
		}
	}
	return nil
}
