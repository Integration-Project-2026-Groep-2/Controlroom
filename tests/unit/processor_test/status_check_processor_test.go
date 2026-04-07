package processor_test

import (
	"encoding/xml"
	"testing"
	"time"

	"github.com/elastic/go-elasticsearch/v9"
	"github.com/stretchr/testify/assert"

	"integration-project-ehb/controlroom/internal/statuscheck"
	"integration-project-ehb/controlroom/internal/userobject"
	"integration-project-ehb/controlroom/pkg/gen"
)

func unreachableES(t *testing.T) *elasticsearch.Client {
	t.Helper()
	es, _ := elasticsearch.NewClient(elasticsearch.Config{
		Addresses: []string{"http://localhost:9999"},
	})
	return es
}

func TestProcessStatusCheck_InvalidXML(t *testing.T) {
	processor := statuscheck.NewStatusCheckProcessor(unreachableES(t))

	err := processor([]byte("not xml"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unmarshal")
}

func TestProcessStatusCheck_ValidXML_ESUnavailable(t *testing.T) {
	processor := statuscheck.NewStatusCheckProcessor(unreachableES(t))

	sc := gen.StatusCheckType{
		ServiceId: "crm-service",
		Timestamp: time.Now().UTC(),
		Uptime:    3600,
	}
	body, err := xml.Marshal(sc)
	assert.NoError(t, err)

	err = processor(body)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "index")
}

func TestProcessStatusCheck_EmptyBody(t *testing.T) {
	processor := statuscheck.NewStatusCheckProcessor(unreachableES(t))

	err := processor([]byte{})
	assert.Error(t, err)
}

func TestProcessStatusCheck_PartialXML(t *testing.T) {
	processor := statuscheck.NewStatusCheckProcessor(unreachableES(t))

	// valid XML structure but missing required fields — unmarshal succeeds,
	// but we still expect an error from the index step (ES unreachable)
	body := []byte(`<StatusCheckType><serviceId>x</serviceId></StatusCheckType>`)
	err := processor(body)
	assert.Error(t, err)
	// the error must come from indexing, not unmarshalling
	assert.Contains(t, err.Error(), "index")
}

func TestProcessUser_InvalidXML(t *testing.T) {
	processor := userobject.NewUserProcessor(unreachableES(t))

	err := processor([]byte("garbage"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unmarshal")
}

func TestProcessUser_ValidXML_ESUnavailable(t *testing.T) {
	processor := userobject.NewUserProcessor(unreachableES(t))

	u := gen.UserConfirmed{
		Id:          gen.UUIDType("550e8400-e29b-41d4-a716-446655440000"),
		Email:       gen.EmailType("test@example.com"),
		FirstName:   "Test",
		LastName:    "User",
		Role:        gen.UserRoleTypeVISITOR,
		IsActive:    true,
		GdprConsent: true,
		ConfirmedAt: gen.ISO8601DateTimeType(time.Now().UTC().Format(time.RFC3339)),
	}
	body, err := xml.Marshal(u)
	assert.NoError(t, err)

	err = processor(body)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "index")
}

func TestProcessUser_EmptyBody(t *testing.T) {
	processor := userobject.NewUserProcessor(unreachableES(t))

	err := processor([]byte{})
	assert.Error(t, err)
}

func TestProcessUser_WrongXMLShape(t *testing.T) {
	processor := userobject.NewUserProcessor(unreachableES(t))

	// heartbeat XML fed to user processor: unmarshal won't error (Go XML is
	// permissive), but ES is unreachable so index will fail
	hb := gen.Heartbeat{ServiceId: "svc", Timestamp: time.Now().UTC()}
	body, _ := xml.Marshal(hb)

	err := processor(body)
	assert.Error(t, err)
}
