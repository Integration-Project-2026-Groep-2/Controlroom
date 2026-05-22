package config

import (
	"integration-project-ehb/controlroom/internal/cr_rabbitmq"
)

type cr_producer_t int8
type cr_producer_events_t int8

const (
	HEARTBEAT_FAILED_EVENT = iota
	HEARTBEAT_SUCCEEDED_EVENT
	RMQ_HEARTBEAT

	WARNING_EVENT
	ERROR_EVENT
	SUMMARY_EVENT
)

const (
	WATCHDOG cr_producer_t = iota
	CONTROLROOM
	RMQ
)

type ProducerDef struct {
	Type     cr_producer_t
	Exchange cr_rabbitmq.ExchangeInfo
	Key      cr_rabbitmq.BindingInfo
	Queue    cr_rabbitmq.QueueInfo
}

var Producer = map[cr_producer_events_t]ProducerDef{

	HEARTBEAT_FAILED_EVENT: {
		Type:     WATCHDOG,
		Exchange: cr_rabbitmq.ExchangeInfo{Name: "ai.events", Kind: "topic", Durable: true},
		Key:      cr_rabbitmq.BindingInfo{Key: "event.heartbeat_failed"},
	},

	HEARTBEAT_SUCCEEDED_EVENT: {
		Type:     WATCHDOG,
		Exchange: cr_rabbitmq.ExchangeInfo{Name: "ai.events", Kind: "topic", Durable: true},
		Key:      cr_rabbitmq.BindingInfo{Key: "event.heartbeat_succeeded"},
	},

	WARNING_EVENT: {
		Type:     WATCHDOG,
		Exchange: cr_rabbitmq.ExchangeInfo{Name: "news.topic", Kind: "topic"},
		Key:      cr_rabbitmq.BindingInfo{Key: "news.warning"},
	},

	ERROR_EVENT: {
		Type:     WATCHDOG,
		Exchange: cr_rabbitmq.ExchangeInfo{Name: "news.topic", Kind: "topic"},
		Key:      cr_rabbitmq.BindingInfo{Key: "news.error"},
	},

	SUMMARY_EVENT: {
		Type:     CONTROLROOM,
		Exchange: cr_rabbitmq.ExchangeInfo{Name: "news.topic", Kind: "direct", Durable: true},
		Key:      cr_rabbitmq.BindingInfo{Key: "news.summary"},
	},

	RMQ_HEARTBEAT: {

		Type:     RMQ,
		Exchange: cr_rabbitmq.ExchangeInfo{Name: "heartbeat.direct", Kind: "direct", Durable: true},
		Key:      cr_rabbitmq.BindingInfo{Key: "routing.heartbeat"},
	},

}
