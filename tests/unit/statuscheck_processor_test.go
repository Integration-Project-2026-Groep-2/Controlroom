package unit_tests

import (
	"encoding/xml"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"integration-project-ehb/controlroom/internal/statuscheck"
	"integration-project-ehb/controlroom/internal/user"
	"integration-project-ehb/controlroom/pkg/gen"
)

func TestProcessStatusCheck_InvalidXML(t *testing.T) {
	err := statuscheck.ProcessStatusCheck(unreachableES(t), []byte("not xml"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unmarshal")
}

func TestProcessStatusCheck_ValidXML_ESUnavailable(t *testing.T) {
	sc := gen.StatusCheck{
		ServiceId: "crm-service",
		Timestamp: time.Now().UTC(),
		Uptime:    3600,
	}
	body, err := xml.Marshal(sc)
	assert.NoError(t, err)
	err = statuscheck.ProcessStatusCheck(unreachableES(t), body)  // Assign the result
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "index")
}

func TestProcessStatusCheck_EmptyBody(t *testing.T) {
	err := statuscheck.ProcessStatusCheck(unreachableES(t), []byte{})
	assert.Error(t, err)
}

func TestProcessStatusCheck_PartialXML(t *testing.T) {
	body := []byte(`<StatusCheck><serviceId>x</serviceId></StatusCheck>`)
	err := statuscheck.ProcessStatusCheck(unreachableES(t), body)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "index")
}

func TestProcessUser_InvalidXML(t *testing.T) {
	err := user.ProcessUser(unreachableES(t), []byte("garbage"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unmarshal")
}

func TestProcessUser_ValidXML_ESUnavailable(t *testing.T) {
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
	err = user.ProcessUser(unreachableES(t), body)  // Assign here too
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "index")
}

func TestProcessUser_EmptyBody(t *testing.T) {
	err := user.ProcessUser(unreachableES(t), []byte{})
	assert.Error(t, err)
}

func TestProcessUser_WrongXMLShape(t *testing.T) {
	hb := gen.Heartbeat{ServiceId: "svc", Timestamp: time.Now().UTC()}
	body, _ := xml.Marshal(hb)
	err := user.ProcessUser(unreachableES(t), body)
	assert.Error(t, err)
}
