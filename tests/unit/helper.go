package unit_tests

import (
	"github.com/elastic/go-elasticsearch/v9"
	"testing"
)

func unreachableES(t *testing.T) *elasticsearch.Client {
	t.Helper()
	es, _ := elasticsearch.NewClient(elasticsearch.Config{
		Addresses: []string{"http://localhost:9999"},
	})
	return es
}
