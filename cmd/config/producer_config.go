package config

import (
    "integration-project-ehb/controlroom/internal/cr_rabbitmq"
)

type cr_producer_t int8
type cr_producer_events_t int8

const (
    HEARTBEAT_FAILED_EVENT = iota
    HEARTBEAT_SUCCEEDED_EVENT
    NEWS_EVENT
)

const (
    WATCHDOG cr_producer_t = iota
)

type ProducerDef struct {
    Type     cr_producer_t
    Exchange cr_rabbitmq.ExchangeInfo
    Key      cr_rabbitmq.BindingInfo
    Queue    cr_rabbitmq.QueueInfo
}

var ProducerDefinitions = map[string]cr_producer_events_t{
    "watchdog.heartbeat_failed":   HEARTBEAT_FAILED_EVENT,
    "watchdog.heartbeat_success":  HEARTBEAT_SUCCEEDED_EVENT,
    "watchdog.news":               NEWS_EVENT,
}

var Producer = map[cr_producer_events_t]ProducerDef{
    HEARTBEAT_FAILED_EVENT: {
        Type: WATCHDOG,
        Exchange: cr_rabbitmq.ExchangeInfo{
            Name:    "ai.events",
            Kind:    "topic",
            Durable: true,
        },
        Key: cr_rabbitmq.BindingInfo{
            Key: "event.heartbeat_failed",
        },
    },
    HEARTBEAT_SUCCEEDED_EVENT: {
        Type: WATCHDOG,
        Exchange: cr_rabbitmq.ExchangeInfo{
            Name:    "ai.events",
            Kind:    "topic",
            Durable: true,
        },
        Key: cr_rabbitmq.BindingInfo{
            Key: "event.heartbeat_succeeded",
        },
    },
}
