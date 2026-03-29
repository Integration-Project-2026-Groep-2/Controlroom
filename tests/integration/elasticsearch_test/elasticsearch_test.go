// Package elasticsearch_test test de verbinding en indexering met de live Elasticsearch server.
//
// Test 1 (Connect): controleert dat de ES client bereikbaar is via een ping.
// Test 2 (IndexHeartbeat): indexeert een heartbeat document in de "heartbeats" index en verwacht geen fout.
// Test 3 (DocumentRetrievable): indexeert een heartbeat en haalt hem daarna op via de document-ID.
// Test 4 (UniqueDocumentID): indexeert twee heartbeats met hetzelfde ID en controleert dat
//
//	er maar één document bestaat (upsert-gedrag).
package elasticsearch_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"integration-project-ehb/controlroom/pkg/xml_gen"
	"os"
	"testing"
	"time"

	elasticsearch "github.com/elastic/go-elasticsearch/v9"
	"github.com/elastic/go-elasticsearch/v9/esapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newESClient bouwt een ES client op basis van omgevingsvariabelen.
// Valt terug op http://localhost:9200 als SERVER of PORT_ELASTICSEARCH niet zijn ingesteld.
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

func TestElasticsearch_Connect(t *testing.T) {
	es := newESClient(t)

	res, err := es.Ping()
	require.NoError(t, err)
	defer res.Body.Close()

	assert.False(t, res.IsError(), "ES ping mislukt: %s", res.String())
}

func TestElasticsearch_IndexHeartbeat(t *testing.T) {
	es := newESClient(t)

	hb := xml_gen.Heartbeat{
		ServiceId: "index-test-service",
		Timestamp: time.Now().UTC(),
	}
	docID := fmt.Sprintf("%s-%d", hb.ServiceId, hb.Timestamp.Unix())

	doc := map[string]interface{}{
		"serviceId": hb.ServiceId,
		"timestamp": hb.Timestamp,
		"indexed":   time.Now(),
	}
	body, err := json.Marshal(doc)
	require.NoError(t, err)

	req := esapi.IndexRequest{
		Index:      "heartbeats",
		DocumentID: docID,
		Body:       bytes.NewReader(body),
		Refresh:    "true",
	}

	res, err := req.Do(context.Background(), es)
	require.NoError(t, err)
	defer res.Body.Close()

	assert.False(t, res.IsError(), "Indexering mislukt: %s", res.String())

	t.Cleanup(func() {
		es.Delete("heartbeats", docID) //nolint
	})
}

func TestElasticsearch_DocumentRetrievable(t *testing.T) {
	es := newESClient(t)

	docID := fmt.Sprintf("retrieve-test-%d", time.Now().UnixNano())
	doc := map[string]interface{}{
		"serviceId": "retrieve-test-service",
		"timestamp": time.Now().UTC(),
	}
	body, err := json.Marshal(doc)
	require.NoError(t, err)

	// Indexeer het document
	indexReq := esapi.IndexRequest{
		Index:      "heartbeats",
		DocumentID: docID,
		Body:       bytes.NewReader(body),
		Refresh:    "true",
	}
	indexRes, err := indexReq.Do(context.Background(), es)
	require.NoError(t, err)
	indexRes.Body.Close()

	// Haal het document op via de document-ID
	getReq := esapi.GetRequest{
		Index:      "heartbeats",
		DocumentID: docID,
	}
	getRes, err := getReq.Do(context.Background(), es)
	require.NoError(t, err)
	defer getRes.Body.Close()

	assert.False(t, getRes.IsError(), "Document niet teruggevonden: %s", getRes.String())

	t.Cleanup(func() {
		es.Delete("heartbeats", docID) //nolint
	})
}

func TestElasticsearch_UniqueDocumentID(t *testing.T) {
	es := newESClient(t)

	docID := fmt.Sprintf("upsert-test-%d", time.Now().UnixNano())

	// Indexeer hetzelfde document-ID twee keer met andere inhoud
	for i := 0; i < 2; i++ {
		doc := map[string]interface{}{
			"serviceId": "upsert-test-service",
			"timestamp": time.Now().UTC(),
			"poging":    i,
		}
		body, err := json.Marshal(doc)
		require.NoError(t, err)

		req := esapi.IndexRequest{
			Index:      "heartbeats",
			DocumentID: docID,
			Body:       bytes.NewReader(body),
			Refresh:    "true",
		}
		res, err := req.Do(context.Background(), es)
		require.NoError(t, err)
		res.Body.Close()
	}

	// Tel het aantal documenten met dit ID — moet precies 1 zijn
	countBody := fmt.Sprintf(`{"query":{"term":{"_id":"%s"}}}`, docID)
	countReq := esapi.CountRequest{
		Index: []string{"heartbeats"},
		Body:  bytes.NewReader([]byte(countBody)),
	}
	countRes, err := countReq.Do(context.Background(), es)
	require.NoError(t, err)
	defer countRes.Body.Close()

	var result map[string]interface{}
	err = json.NewDecoder(countRes.Body).Decode(&result)
	require.NoError(t, err)

	count := int(result["count"].(float64))
	assert.Equal(t, 1, count, "Hetzelfde document-ID mag maar één keer voorkomen (upsert)")

	t.Cleanup(func() {
		es.Delete("heartbeats", docID) //nolint
	})
}
