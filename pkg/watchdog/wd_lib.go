package watchdog

import (
	amqp "github.com/rabbitmq/amqp091-go"
	"os"
	"sync/atomic"
)

// State trackers
var WDServices = [6]string{
	"CRM",
	"FACTURATIE",
	"FRONTEND",
	"MAILING",
	"PLANNING",
	"KASSA",
}

var WDServiceState = map[string]bool{
	"CRM":        false,
	"FACTURATIE": false,
	"FRONTEND":   false,
	"MAILING":    false,
	"PLANNING":   false,
	"KASSA":      false,
}

var WDQueue = make(chan string, 50)
var WDWebhook = os.Getenv("TEAMS_WEBHOOK_URL")

// dont know if this is actually a good idea
var WDPubChan atomic.Pointer[amqp.Channel]

func SetPubChannel(ch *amqp.Channel) {
	WDPubChan.Store(ch)
}
