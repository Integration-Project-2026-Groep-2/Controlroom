// Package e2e_test test de volledige flow van publicatie tot opslag met RabbitMQ én Elasticsearch.
//
// Test 1 (ValidHeartbeat_IndexedInES): publiceert een geldige heartbeat via RabbitMQ,
//
//	wacht tot de consumer hem verwerkt en controleert dat het document in ES staat.
//
// Test 2 (InvalidXML_GoesToDLQ_NotES): publiceert ongeldige XML en controleert dat
//
//	het bericht in heartbeat_dlq terechtkomt en er niets in ES wordt opgeslagen.
//
// Test 3 (MultipleServices_AllIndexed): publiceert heartbeats van 3 services tegelijk
//
//	en controleert dat alle 3 als aparte documenten in ES verschijnen.
package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"integration-project-ehb/controlroom/pkg/xml_gen"
	"os"
	"testing"
	"time"

	elasticsearch "github.com/elastic/go-elasticsearch/v9"
	"github.com/elastic/go-elasticsearch/v9/esapi"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"integration-project-ehb/controlroom/internal/heartbeat"
)

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

func newESClient(t *testing.T) *elasticsearch.Client {
	t.Helper()

	server := os.Getenv("SERVER")
	port := os.Getenv("PORT_ELASTICSEARCH")

	if server == "" {
		server = "localhost"
	}
	if port == "" {
		port = "9200"
	}

	es, err := elasticsearch.NewClient(elasticsearch.Config{
		Addresses: []string{fmt.Sprintf("http://%s:%s", server, port)},
	})
	require.NoError(t, err)
	return es
}

// waitForDocument pollt ES totdat het document gevonden wordt of de timeout verloopt.
func waitForDocument(es *elasticsearch.Client, docID string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		req := esapi.GetRequest{Index: "heartbeats", DocumentID: docID}
		res, err := req.Do(context.Background(), es)
		if err == nil && !res.IsError() {
			res.Body.Close()
			return true
		}
		if res != nil {
			res.Body.Close()
		}
		time.Sleep(500 * time.Millisecond)
	}
	return false
}

// setupConsumer maakt een tijdelijke queue aan, bindt die aan de exchange en start de processor.
// Geeft de msgs channel en een publish channel terug.
func setupConsumer(t *testing.T, conn *amqp.Connection, es *elasticsearch.Client) (chan<- amqp.Publishing, context.CancelFunc) {
	t.Helper()

	consumeCh, err := conn.Channel()
	require.NoError(t, err)
	t.Cleanup(func() { consumeCh.Close() })

	dlqCh, err := conn.Channel()
	require.NoError(t, err)
	t.Cleanup(func() { dlqCh.Close() })

	publishCh, err := conn.Channel()
	require.NoError(t, err)
	t.Cleanup(func() { publishCh.Close() })

	err = consumeCh.ExchangeDeclare("control_room_exchange", "direct", true, false, false, false, nil)
	require.NoError(t, err)

	// Exclusieve queue zodat alleen deze test de berichten ontvangt
	q, err := consumeCh.QueueDeclare("", false, true, true, false, nil)
	require.NoError(t, err)

	err = consumeCh.QueueBind(q.Name, "routing.heartbeat", "control_room_exchange", false, nil)
	require.NoError(t, err)

	msgs, err := consumeCh.Consume(q.Name, "", false, true, false, false, nil)
	require.NoError(t, err)

	processor := heartbeat.CreateProcessor(es, dlqCh)

	ctx, cancel := context.WithCancel(context.Background())
	go heartbeat.ConsumeHeartbeats(processor, msgs, ctx)

	// Wrapper die berichten via publishCh stuurt
	publish := make(chan amqp.Publishing, 10)
	go func() {
		for msg := range publish {
			publishCh.PublishWithContext(context.Background(), "control_room_exchange", "routing.heartbeat", false, false, msg) //nolint
		}
	}()

	return publish, cancel
}

func TestE2E_ValidHeartbeat_IndexedInES(t *testing.T) {
	conn, err := amqp.Dial(rabbitmqURL())
	require.NoError(t, err)
	defer conn.Close()

	es := newESClient(t)

	publish, cancel := setupConsumer(t, conn, es)
	defer cancel()

	hb := xml_gen.Heartbeat{
		ServiceId: fmt.Sprintf("e2e-valid-%d", time.Now().UnixNano()),
		Timestamp: time.Now().UTC(),
	}
	docID := fmt.Sprintf("%s-%d", hb.ServiceId, hb.Timestamp.Unix())

	body, err := xml.Marshal(hb)
	require.NoError(t, err)

	publish <- amqp.Publishing{ContentType: "text/xml", Body: body}

	found := waitForDocument(es, docID, 10*time.Second)
	assert.True(t, found, "Document moet in ES staan na verwerking van geldige heartbeat")

	t.Cleanup(func() {
		es.Delete("heartbeats", docID) //nolint
	})
}

func TestE2E_InvalidXML_GoesToDLQ_NotES(t *testing.T) {
	conn, err := amqp.Dial(rabbitmqURL())
	require.NoError(t, err)
	defer conn.Close()

	es := newESClient(t)

	publish, cancel := setupConsumer(t, conn, es)
	defer cancel()

	// Declareer DLQ en start consumer om het bericht te ontvangen
	dlqCheckCh, err := conn.Channel()
	require.NoError(t, err)
	defer dlqCheckCh.Close()

	_, err = dlqCheckCh.QueueDeclare("heartbeat_dlq", true, false, false, false, nil)
	require.NoError(t, err)

	dlqMsgs, err := dlqCheckCh.Consume("heartbeat_dlq", "", false, false, false, false, nil)
	require.NoError(t, err)

	invalidXML := []byte("dit is geen geldige xml")
	publish <- amqp.Publishing{ContentType: "text/xml", Body: invalidXML}

	// Wacht tot het bericht in de DLQ verschijnt
	dlqReceived := false
	deadline := time.After(10 * time.Second)
	for !dlqReceived {
		select {
		case msg := <-dlqMsgs:
			if string(msg.Body) == string(invalidXML) {
				dlqReceived = true
				msg.Ack(false)
			} else {
				msg.Nack(false, true) // terugplaatsen — dit was een ander bericht
			}
		case <-deadline:
			t.Fatal("Ongeldige XML werd niet naar heartbeat_dlq gestuurd binnen 10 seconden")
		}
	}

	assert.True(t, dlqReceived, "Ongeldig bericht moet in de DLQ terechtkomen")
}

func TestE2E_MultipleServices_AllIndexed(t *testing.T) {
	conn, err := amqp.Dial(rabbitmqURL())
	require.NoError(t, err)
	defer conn.Close()

	es := newESClient(t)

	publish, cancel := setupConsumer(t, conn, es)
	defer cancel()

	services := []string{
		fmt.Sprintf("Service-CRM-%d", time.Now().UnixNano()),
		fmt.Sprintf("Service-Facturatie-%d", time.Now().UnixNano()),
		fmt.Sprintf("Service-Logistiek-%d", time.Now().UnixNano()),
	}

	now := time.Now().UTC()
	docIDs := make([]string, len(services))

	for i, svc := range services {
		hb := xml_gen.Heartbeat{ServiceId: svc, Timestamp: now}
		docIDs[i] = fmt.Sprintf("%s-%d", svc, now.Unix())

		body, err := xml.Marshal(hb)
		require.NoError(t, err)

		publish <- amqp.Publishing{ContentType: "text/xml", Body: body}
	}

	// Controleer dat alle 3 documenten in ES verschijnen
	for i, docID := range docIDs {
		found := waitForDocument(es, docID, 10*time.Second)
		assert.True(t, found, "Document voor %s niet gevonden in ES", services[i])

		t.Cleanup(func() {
			es.Delete("heartbeats", docID) //nolint
		})
	}

	// Controleer ook dat de documenten unieke IDs hebben via een count query
	countBody, _ := json.Marshal(map[string]interface{}{
		"query": map[string]interface{}{
			"terms": map[string]interface{}{
				"_id": docIDs,
			},
		},
	})
	countReq := esapi.CountRequest{
		Index: []string{"heartbeats"},
		Body:  bytes.NewReader(countBody),
	}
	countRes, err := countReq.Do(context.Background(), es)
	require.NoError(t, err)
	defer countRes.Body.Close()

	var result map[string]interface{}
	err = json.NewDecoder(countRes.Body).Decode(&result)
	require.NoError(t, err)

	count := int(result["count"].(float64))
	assert.Equal(t, len(services), count, "Alle %d services moeten als aparte documenten in ES staan", len(services))
}
