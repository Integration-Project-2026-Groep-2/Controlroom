package config

import (
	"integration-project-ehb/controlroom/internal/cr_rabbitmq"
	"os"

	"github.com/elastic/go-elasticsearch/v9"
)

type cr_consumer_t int8

const (
	HEARTBEAT cr_consumer_t = iota
	LOGGER
	STATUSCHECK
	USER
	COMPANY
	USER_ACK
	CHECK_IN
)

type ConsumerDef struct {
	Type     cr_consumer_t
	Exchange cr_rabbitmq.ExchangeInfo
	Queue    cr_rabbitmq.QueueInfo
	Binding  cr_rabbitmq.BindingInfo
	DLQName  string
	Qos      int
	Passive  bool
}

var ConsumerDefinitions = []ConsumerDef{
	{
		Type:     HEARTBEAT,
		Exchange: cr_rabbitmq.ExchangeInfo{Name: "heartbeat.direct", Kind: "direct", Durable: true},
		Queue:    cr_rabbitmq.QueueInfo{Name: "controlroom.heartbeat.queue", Durable: true},
		Binding:  cr_rabbitmq.BindingInfo{Key: "routing.heartbeat"},
		DLQName:  "controlroom.heartbeat.queue.dlq",
		Qos:      18,
		Passive:  false,
	},
	{
		Type:     STATUSCHECK,
		Exchange: cr_rabbitmq.ExchangeInfo{Name: "statuscheck.direct", Kind: "direct", Durable: true},
		Queue:    cr_rabbitmq.QueueInfo{Name: "controlroom.statuscheck.queue", Durable: true},
		Binding:  cr_rabbitmq.BindingInfo{Key: "routing.statuscheck"},
		DLQName:  "controlroom.statuscheck.queue.dlq",
		Qos:      5,
		Passive:  false,
	},
	{
		Type:     LOGGER,
		Exchange: cr_rabbitmq.ExchangeInfo{Name: "logs.direct", Kind: "direct", Durable: true},
		Queue:    cr_rabbitmq.QueueInfo{Name: "controlroom.logs.queue", Durable: true},
		Binding:  cr_rabbitmq.BindingInfo{Key: "routing.log"},
		DLQName:  "controlroom.logs.queue.dlq",
		Qos:      5,
		Passive:  false,
	},
	{
		Type:     USER,
		Exchange: cr_rabbitmq.ExchangeInfo{Name: "contact.topic", Durable: true},
		Queue:    cr_rabbitmq.QueueInfo{Name: "crm.user.confirmed", Durable: true},
		Binding:  cr_rabbitmq.BindingInfo{Key: "crm.user.confirmed"},
		Qos:      10,
		Passive:  false,
	},
	{
		Type:     COMPANY,
		Exchange: cr_rabbitmq.ExchangeInfo{Name: "contact.topic", Durable: true},
		Queue:    cr_rabbitmq.QueueInfo{Name: "crm.company.confirmed", Durable: true},
		Binding:  cr_rabbitmq.BindingInfo{Key: "crm.company.confirmed"},
		Qos:      10,
		Passive:  false,
	},
	{
		Type:     USER_ACK,
		Exchange: cr_rabbitmq.ExchangeInfo{Name: "controlroom.user.confirmed.direct", Kind: "direct", Durable: true},
		Queue:    cr_rabbitmq.QueueInfo{Name: "controlroom.user.confirmed", Durable: true},
		Binding:  cr_rabbitmq.BindingInfo{Key: "routing.controlroom.user.confirmed"},
		DLQName:  "controlroom.user.confirmed.queue.dlq",
		Qos:      10,
		Passive:  false,
	},
	{
		Type:     CHECK_IN,
		Exchange: cr_rabbitmq.ExchangeInfo{Name: "user.checkin.topic", Kind: "topic", Durable: true},
		Queue:    cr_rabbitmq.QueueInfo{Name: "controlroom.user.checkin", Durable: true},
		Binding:  cr_rabbitmq.BindingInfo{Key: "routing.user.checkin"},
		DLQName:  "controlroom.user.checkin.dlq",
		Qos:      1,
		Passive:  false,
	},
}

var ElasticUrl string = os.Getenv("ELASTICSEARCH_URL")

var ElasticConfig = elasticsearch.Config{
	Addresses: []string{ElasticUrl},
	Username:  os.Getenv("CONTROLROOM_ES_USER"),
	Password:  os.Getenv("CONTROLROOM_ES_PASS"),
}

var Services = [6]string{
	"CRM",
	"FACTURATIE",
	"FRONTEND",
	"MAILING",
	"PLANNING",
	"KASSA",
}
