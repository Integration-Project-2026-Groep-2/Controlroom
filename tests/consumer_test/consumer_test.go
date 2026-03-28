// Package consumer_test test of de heartbeat consumer correct stopt in twee situaties.
//
// Test 1 (StopsOnContextCancel): simuleert een graceful shutdown (bijv. docker stop) via context cancel
// → verwacht dat ConsumeHeartbeats binnen 1 seconde stopt.
//
// Test 2 (StopsOnClosedChannel): simuleert een weggevallen RabbitMQ verbinding via een gesloten channel
// → verwacht dat ConsumeHeartbeats binnen 1 seconde stopt.
//
package main

import (
	"context"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"integration-project-ehb/controlroom/internal/heartbeat"
)

func TestConsumeHeartbeats_StopsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	msgs := make(chan amqp.Delivery)
	done := make(chan struct{})

	go func() {
		heartbeat.ConsumeHeartbeats(nil, msgs, ctx)
		close(done)
	}()

	cancel()

	select {
	case <-done:
		// geslaagd
	case <-time.After(time.Second):
		t.Fatal("ConsumeHeartbeats stopte niet na context cancel")
	}
}

func TestConsumeHeartbeats_StopsOnClosedChannel(t *testing.T) {
	ctx := context.Background()
	msgs := make(chan amqp.Delivery)
	done := make(chan struct{})

	go func() {
		heartbeat.ConsumeHeartbeats(nil, msgs, ctx)
		close(done)
	}()

	close(msgs)

	select {
	case <-done:
		// geslaagd
	case <-time.After(time.Second):
		t.Fatal("ConsumeHeartbeats stopte niet na gesloten channel")
	}
}
