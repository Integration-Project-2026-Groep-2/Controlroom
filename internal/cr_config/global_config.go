package config

import (
	"os"
	//	"strings"

	"github.com/elastic/go-elasticsearch/v9"
)

// NOTE(nasr): apparantly equal fold is case insentive

// pfff see the logger im anoyed by this
// var DebugMode bool = strings.EqualFold(os.Getenv("DebugMode"), "true")

type kibanaConfig struct {
	Url               string
	UserName          string
	Password          string
	DashboardUser     string
	DashboardPassword string
}

var KibanaConfig = kibanaConfig{
	Url:               os.Getenv("KIBANA_URL"),
	UserName:          os.Getenv("KIBANA_USERNAME"),
	Password:          os.Getenv("KIBANA_PASSWORD"),
	DashboardUser:     os.Getenv("DASHBOARD_USER"),
	DashboardPassword: os.Getenv("DASHBOARD_PASSWORD"),
}

var ElasticConfig = elasticsearch.Config{
	Addresses: []string{os.Getenv("ELASTICSEARCH_URL")},
	Username:  os.Getenv("CONTROLROOM_ES_USER"),
	Password:  os.Getenv("CONTROLROOM_ES_PASS"),
}

// TODO(nasr): replace all of the services with a dynamic query to elastic that gathers the different service names
// or we could do this while consuming the services. also interesting...

// NOTE(nasr): hello this is future me talking about dynamic stuff. okay we are going to keep a list
// of the default services. we're going to make use of the dynamic array trait of arrays in this lanague
// and because increasing them isn't something that happens very often. we are going to do it.
// so we keep a default list of the services, and we push stuff on to that array whenever we receive something new
// if we stop receiving something we remove it. but for the intialization we should keep it i think
var Services = [6]string{
	"CRM",
	"FACTURATIE",
	"FRONTEND",
	"MAILING",
	"PLANNING",
	"KASSA",
}

var Hostname string

var LogsDashboardId = os.Getenv("DASHBOARD_LOGS_ID")
var LogsDataViewId = os.Getenv("DATAVIEW_LOGS_ID")

var HeartbeatDashboardId = os.Getenv("DASHBOARD_HEARTBEATS_ID")
var HeartbeatDataviewId = os.Getenv("DATAVIEW_HEARTBEATS_ID")

var KbnXsrfToken = "true"
var McpMasterUrl = os.Getenv("MCP_MASTER_URL")

const GithubBaseAPI = "https://api.github.com"

// -- github

const GithubJarvisName = "jarvis el morabit cowe"
const GithubJarvisMail = "controlroom-master@integration-project.local"
