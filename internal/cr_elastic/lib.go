// - simplified way of building elastic queries for your service
// - we can expand on this in the future by
// - 1) adding parameters
// - 2) writing new functions
// - if you read this you stink
package cr_elastic

type QueryClause any

type RangeClause struct {
	Range map[string]map[string]string `json:"range"`
}

type TermClause struct {
	Term map[string]string `json:"term"`
}

type BoolQuery struct {
	Filter []any `json:"filter"`
}

type ElasticsearchQuery struct {
	Size  int        `json:"size"`
	Query *BoolQuery `json:"query"`
}

func NewElasticQuery(serviceName string) *ElasticsearchQuery {
	return &ElasticsearchQuery{
		Size: 0,
		Query: &BoolQuery{
			Filter: []any{
				RangeClause{
					Range: map[string]map[string]string{
						"timestamp": {"gte": "now-60s"},
					},
				},
				TermClause{
					Term: map[string]string{
						"service_id.keyword": serviceName,
					},
				},
			},
		},
	}
}
