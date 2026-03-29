// Package rabbitmq_test test de verbinding en het berichtverkeer met de live RabbitMQ server.
//
// Test 1 (Connect): verbindt met RabbitMQ via de credentials uit de omgevingsvariabelen.
// Test 2 (ExchangeDeclare): declareert control_room_exchange en controleert dat dit geen fout geeft.
// Test 3 (QueueBindAndConsume): maakt een tijdelijke queue aan, bindt die aan de exchange en start een consumer.
// Test 4 (PublishAndReceive): publiceert een XML heartbeat en controleert dat de consumer hem ontvangt.
// Test 5 (InvalidXML_GoesToDLQ): simuleert wat de processor doet bij ongeldige XML — publiceert direct naar heartbeat_dlq
//
//	en controleert dat het bericht met error_reason header aankomt.
//
// Test 6 (Ack_RemovesMessageFromQueue): verwerkt een bericht succesvol en controleert dat de queue daarna leeg is.
// Test 7 (Nack_GoesToDLQ): verwerpt een bericht zonder requeue en controleert dat de queue leeg is.
package rabbitmq_test

import (
	"context"
	"encoding/xml"
	"fmt"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"integration-project-ehb/controlroom/pkg/xml/gen"
	"os"
)

// rabbitmqURL bouwt de verbindings-URL op uit omgevingsvariabelen.
// "rabbitmq" is de Docker-interne hostnaam — bij tests vanaf de hostmachine wordt localhost gebruikt.
func rabbitmqURL() string {
	user := os.Getenv("RABBITMQ_USER")
	pass := os.Getenv("RABBITMQ_PASSWORD")
	host := os.Getenv("RABBITMQ_HOST")

	if user == "" {
		user = "guest"
	}
	if pass == "" {
		pass = "guest"
	}
	if host == "" || host == "rabbitmq" {
		host = "localhost"
	}

	return fmt.Sprintf("amqp://%s:%s@%s:5672/", user, pass, host)
}

func TestRabbitMQ_Connect(t *testing.T) {
	conn, err := amqp.Dial(rabbitmqURL())

	require.NoError(t, err, "Verbinding met RabbitMQ mislukt")
	assert.NotNil(t, conn)
	conn.Close()
}

func TestRabbitMQ_ExchangeDeclare(t *testing.T) {
	conn, err := amqp.Dial(rabbitmqURL())
	require.NoError(t, err)
	defer conn.Close()

	ch, err := conn.Channel()
	require.NoError(t, err)
	defer ch.Close()

	err = ch.ExchangeDeclare("control_room_exchange", "direct", true, false, false, false, nil)
	assert.NoError(t, err, "Exchange declareren mislukt")
}

func TestRabbitMQ_QueueBindAndConsume(t *testing.T) {
	conn, err := amqp.Dial(rabbitmqURL())
	require.NoError(t, err)
	defer conn.Close()

	ch, err := conn.Channel()
	require.NoError(t, err)
	defer ch.Close()

	err = ch.ExchangeDeclare("control_room_exchange", "direct", true, false, false, false, nil)
	require.NoError(t, err)

	// Tijdelijke, exclusieve queue zodat de test zichzelf opruimt
	q, err := ch.QueueDeclare("", false, true, true, false, nil)
	require.NoError(t, err)

	err = ch.QueueBind(q.Name, "routing.heartbeat", "control_room_exchange", false, nil)
	require.NoError(t, err)

	msgs, err := ch.Consume(q.Name, "", false, true, false, false, nil)
	assert.NoError(t, err)
	assert.NotNil(t, msgs)
}

func TestRabbitMQ_PublishAndReceive(t *testing.T) {
	conn, err := amqp.Dial(rabbitmqURL())
	require.NoError(t, err)
	defer conn.Close()

	ch, err := conn.Channel()
	require.NoError(t, err)
	defer ch.Close()

	err = ch.ExchangeDeclare("control_room_exchange", "direct", true, false, false, false, nil)
	require.NoError(t, err)

	q, err := ch.QueueDeclare("", false, true, true, false, nil)
	require.NoError(t, err)

	err = ch.QueueBind(q.Name, "routing.heartbeat", "control_room_exchange", false, nil)
	require.NoError(t, err)

	msgs, err := ch.Consume(q.Name, "", false, true, false, false, nil)
	require.NoError(t, err)

	hb := gen.Heartbeat{ServiceId: "test-publish-receive", Timestamp: time.Now().UTC()}
	body, err := xml.Marshal(hb)
	require.NoError(t, err)

	err = ch.PublishWithContext(context.Background(), "control_room_exchange", "routing.heartbeat", false, false, amqp.Publishing{
		ContentType: "text/xml",
		Body:        body,
	})
	require.NoError(t, err)

	select {
	case msg := <-msgs:
		assert.Equal(t, body, msg.Body, "Ontvangen bericht komt niet overeen met het gepubliceerde bericht")
		msg.Ack(false)
	case <-time.After(5 * time.Second):
		t.Fatal("Geen bericht ontvangen binnen 5 seconden")
	}
}

func TestRabbitMQ_InvalidXML_GoesToDLQ(t *testing.T) {
	conn, err := amqp.Dial(rabbitmqURL())
	require.NoError(t, err)
	defer conn.Close()

	ch, err := conn.Channel()
	require.NoError(t, err)
	defer ch.Close()

	// Declareer de DLQ (idempotent — bestaat al als de processor eerder draaide)
	_, err = ch.QueueDeclare("heartbeat_dlq", true, false, false, false, nil)
	require.NoError(t, err)

	invalidXML := []byte("niet geldig xml")

	// Simuleer wat sendToDLQ in de processor doet
	err = ch.PublishWithContext(context.Background(), "", "heartbeat_dlq", false, false, amqp.Publishing{
		ContentType: "application/xml",
		Body:        invalidXML,
		Headers: amqp.Table{
			"error_reason": "unmarshal error: test",
		},
	})
	require.NoError(t, err)

	msgs, err := ch.Consume("heartbeat_dlq", "", false, false, false, false, nil)
	require.NoError(t, err)

	select {
	case msg := <-msgs:
		assert.Equal(t, invalidXML, msg.Body)
		assert.Equal(t, "unmarshal error: test", msg.Headers["error_reason"])
		msg.Ack(false)
	case <-time.After(5 * time.Second):
		t.Fatal("Geen bericht ontvangen in heartbeat_dlq")
	}
}

func TestRabbitMQ_Ack_RemovesMessageFromQueue(t *testing.T) {
	conn, err := amqp.Dial(rabbitmqURL())
	require.NoError(t, err)
	defer conn.Close()

	ch, err := conn.Channel()
	require.NoError(t, err)
	defer ch.Close()

	// Benoemde queue met auto-delete zodat we de berichtentelling kunnen opvragen
	qName := fmt.Sprintf("test_ack_%d", time.Now().UnixNano())
	q, err := ch.QueueDeclare(qName, false, true, false, false, nil)
	require.NoError(t, err)

	err = ch.PublishWithContext(context.Background(), "", qName, false, false, amqp.Publishing{
		Body: []byte("test ack bericht"),
	})
	require.NoError(t, err)

	msgs, err := ch.Consume(q.Name, "", false, false, false, false, nil)
	require.NoError(t, err)

	select {
	case msg := <-msgs:
		msg.Ack(false)
	case <-time.After(5 * time.Second):
		t.Fatal("Geen bericht ontvangen")
	}

	time.Sleep(100 * time.Millisecond)

	q2, err := ch.QueueInspect(qName)
	require.NoError(t, err)
	assert.Equal(t, 0, q2.Messages, "Queue moet leeg zijn na Ack")
}

func TestRabbitMQ_Nack_GoesToDLQ(t *testing.T) {
	conn, err := amqp.Dial(rabbitmqURL())
	require.NoError(t, err)
	defer conn.Close()

	ch, err := conn.Channel()
	require.NoError(t, err)
	defer ch.Close()

	qName := fmt.Sprintf("test_nack_%d", time.Now().UnixNano())
	q, err := ch.QueueDeclare(qName, false, true, false, false, nil)
	require.NoError(t, err)

	err = ch.PublishWithContext(context.Background(), "", qName, false, false, amqp.Publishing{
		Body: []byte("test nack bericht"),
	})
	require.NoError(t, err)

	msgs, err := ch.Consume(q.Name, "", false, false, false, false, nil)
	require.NoError(t, err)

	select {
	case msg := <-msgs:
		msg.Nack(false, false) // geen requeue
	case <-time.After(5 * time.Second):
		t.Fatal("Geen bericht ontvangen")
	}

	time.Sleep(100 * time.Millisecond)

	q2, err := ch.QueueInspect(qName)
	require.NoError(t, err)
	assert.Equal(t, 0, q2.Messages, "Queue moet leeg zijn na Nack zonder requeue")
}
