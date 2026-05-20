package config

import (
	"fmt"
	"os"

	"github.com/elastic/go-elasticsearch/v9"
)

var ElasticUrl string = os.Getenv("ELASTICSEARCH_URL")

var ElasticConfig = elasticsearch.Config{
	Addresses: []string{ElasticUrl},
	Username:  os.Getenv("CONTROLROOM_ES_USER"),
	Password:  os.Getenv("CONTROLROOM_ES_PASS"),
}

// TODO(nasr): replace all of the services with a dynamic query to elastic that gathers the different service names
// or we could do this while consuming the services. also interesting...
var Services = [6]string{
	"CRM",
	"FACTURATIE",
	"FRONTEND",
	"MAILING",
	"PLANNING",
	"KASSA",
}

var Hostname string

var KbnXsrfToken = "true"

var KibanaURL 	= fmt.Sprintf("%s:%s", os.Getenv("http://localhost"), os.Getenv("PORT_KIBANA"))
var DashboardID = os.Getenv("DASHBOARD_ID")
var DataViewId  = os.Getenv("DATAVIEW_ID")
