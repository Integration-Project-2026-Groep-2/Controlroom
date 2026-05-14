package watchdog

import (
	"net/http"
	"os"
	"sync"
	"sync/atomic"

	amqp "github.com/rabbitmq/amqp091-go"
)

var (
	Services = []string{
		"CRM",
		"FACTURATIE",
		"FRONTEND",
		"MAILING",
		"PLANNING",
		"KASSA",
	}

	ServiceState = map[string]bool{
		"CRM":        false,
		"FACTURATIE": false,
		"FRONTEND":   false,
		"MAILING":    false,
		"PLANNING":   false,
		"KASSA":      false,
	}

	StateMutex sync.RWMutex

	WatchdogQueue = make(chan string, 50)
	TeamsWebhook  = os.Getenv("TEAMS_WEBHOOK_URL")
	WatchdogChan  atomic.Pointer[amqp.Channel]
)

func SetPubChannel(ch *amqp.Channel) {
	WatchdogChan.Store(ch)
}

type EventType string
type SeverityLevel string

const (
	HeartbeatFailed EventType = "heartbeat_failed"
	HeartbeatOnline EventType = "heartbeat_online"
)

const (
	Critical SeverityLevel = "critical"
	Warning  SeverityLevel = "warning"
	Info     SeverityLevel = "info"
)

type AlertEvent struct {
	Event     EventType          `json:"event"`
	Source    string             `json:"source"`
	Timestamp string             `json:"timestamp"`
	Payload   *AlertEventPayload `json:"payload"`
}

type AlertEventPayload struct {
	Summary       string              `json:"summary"`
	Severity      SeverityLevel       `json:"severity"`
	Component     string              `json:"component"`
	Group         string              `json:"group"`
	Class         string              `json:"class"`
	CustomDetails *CustomAlertDetails `json:"custom_details"`
}

type CustomAlertDetails struct {
	HeartbeatCountLast60s int64  `json:"heartbeat_count_last_60s"`
	Threshold             int64  `json:"threshold"`
	LastCheckAt           string `json:"last_check_at"`
}

type TeamsAlertSender struct {
	webhookURL string
	httpClient *http.Client
}

type TeamsAlertCard struct {
	Type        string           `json:"type"`
	Attachments []TeamAttachment `json:"attachments"`
}

type TeamAttachment struct {
	ContentType string        `json:"contentType"`
	Content     *AdaptiveCard `json:"content"`
}

type AdaptiveCard struct {
	Schema  string      `json:"$schema"`
	Type    string      `json:"type"`
	Version string      `json:"version"`
	Body    []TextBlock `json:"body"`
}

type TextBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
	Wrap bool   `json:"wrap"`
}

type HeartbeatStatusEvent struct {
	Event     string                  `json:"event"`
	Source    string                  `json:"source"`
	Timestamp string                  `json:"timestamp"`
	Payload   *HeartbeatStatusPayload `json:"payload"`
}

type HeartbeatStatusPayload struct {
	Summary       string                  `json:"summary"`
	Severity      string                  `json:"severity"`
	Component     string                  `json:"component"`
	Group         string                  `json:"group"`
	Class         string                  `json:"class"`
	CustomDetails *HeartbeatCustomDetails `json:"custom_details"`
}

type HeartbeatCustomDetails struct {
	HeartbeatCountLast60s float64 `json:"heartbeat_count_last_60s"`
	Threshold             int     `json:"threshold"`
	LastCheckAt           string  `json:"last_check_at"`
}
