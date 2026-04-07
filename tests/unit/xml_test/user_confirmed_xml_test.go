package xml_test

import (
	"encoding/xml"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"integration-project-ehb/controlroom/pkg/gen"
)

func makeValidUser() gen.UserConfirmed {
	return gen.UserConfirmed{
		Id:          "550e8400-e29b-41d4-a716-446655440000",
		Email:       "jane.doe@example.com",
		FirstName:   "Jane",
		LastName:    "Doe",
		Role:        gen.UserRoleTypeADMIN,
		IsActive:    true,
		GdprConsent: true,
		ConfirmedAt: gen.ISO8601DateTimeType(time.Now().UTC().Format(time.RFC3339)),
	}
}

func TestUserConfirmed_MarshalUnmarshal(t *testing.T) {
	original := makeValidUser()

	data, err := xml.Marshal(original)
	assert.NoError(t, err)

	var result gen.UserConfirmed
	err = xml.Unmarshal(data, &result)
	assert.NoError(t, err)

	assert.Equal(t, original.Id, result.Id)
	assert.Equal(t, original.Email, result.Email)
	assert.Equal(t, original.FirstName, result.FirstName)
	assert.Equal(t, original.LastName, result.LastName)
	assert.Equal(t, original.Role, result.Role)
	assert.Equal(t, original.IsActive, result.IsActive)
	assert.Equal(t, original.GdprConsent, result.GdprConsent)
	assert.Equal(t, original.ConfirmedAt, result.ConfirmedAt)
}

func TestUserConfirmed_XMLTagNames(t *testing.T) {
	u := makeValidUser()

	data, err := xml.Marshal(u)
	assert.NoError(t, err)

	xmlStr := string(data)
	assert.True(t, strings.Contains(xmlStr, "<UserConfirmed>"), "expected root tag <UserConfirmed>")
	assert.True(t, strings.Contains(xmlStr, "<id>"), "expected tag <id>")
	assert.True(t, strings.Contains(xmlStr, "<email>"), "expected tag <email>")
	assert.True(t, strings.Contains(xmlStr, "<firstName>"), "expected tag <firstName>")
	assert.True(t, strings.Contains(xmlStr, "<lastName>"), "expected tag <lastName>")
	assert.True(t, strings.Contains(xmlStr, "<role>"), "expected tag <role>")
	assert.True(t, strings.Contains(xmlStr, "<isActive>"), "expected tag <isActive>")
	assert.True(t, strings.Contains(xmlStr, "<gdprConsent>"), "expected tag <gdprConsent>")
	assert.True(t, strings.Contains(xmlStr, "<confirmedAt>"), "expected tag <confirmedAt>")
}

func TestUserConfirmed_OptionalFieldsAbsent(t *testing.T) {
	// phone, companyId, badgeCode are minOccurs=0 — they should not appear in
	// the XML when empty, and unmarshalling without them should leave them zero
	u := makeValidUser()
	// do not set Phone, CompanyId, BadgeCode

	data, err := xml.Marshal(u)
	assert.NoError(t, err)

	var result gen.UserConfirmed
	err = xml.Unmarshal(data, &result)
	assert.NoError(t, err)

	assert.Equal(t, "", result.Phone)
	assert.Equal(t, gen.UUIDType(""), result.CompanyId)
	assert.Equal(t, "", result.BadgeCode)
}

func TestUserConfirmed_OptionalFieldsPresent(t *testing.T) {
	u := makeValidUser()
	u.Phone = "+32477000000"
	u.CompanyId = "a3b8c9d0-1234-5678-90ab-cdef12345678"
	u.BadgeCode = "BADGE-42"

	data, err := xml.Marshal(u)
	assert.NoError(t, err)

	var result gen.UserConfirmed
	err = xml.Unmarshal(data, &result)
	assert.NoError(t, err)

	assert.Equal(t, "+32477000000", result.Phone)
	assert.Equal(t, gen.UUIDType("a3b8c9d0-1234-5678-90ab-cdef12345678"), result.CompanyId)
	assert.Equal(t, "BADGE-42", result.BadgeCode)
}

func TestUserConfirmed_AllRolesRoundTrip(t *testing.T) {
	roles := []gen.UserRoleType{
		gen.UserRoleTypeVISITOR,
		gen.UserRoleTypeCOMPANYCONTACT,
		gen.UserRoleTypeSPEAKER,
		gen.UserRoleTypeEVENTMANAGER,
		gen.UserRoleTypeCASHIER,
		gen.UserRoleTypeBARSTAFF,
		gen.UserRoleTypeADMIN,
	}

	for _, role := range roles {
		u := makeValidUser()
		u.Role = role

		data, err := xml.Marshal(u)
		assert.NoError(t, err, "marshal failed for role %s", role)

		var result gen.UserConfirmed
		err = xml.Unmarshal(data, &result)
		assert.NoError(t, err, "unmarshal failed for role %s", role)
		assert.Equal(t, role, result.Role, "role mismatch for %s", role)
	}
}

func TestUserConfirmed_GdprConsentFalse(t *testing.T) {
	u := makeValidUser()
	u.GdprConsent = false

	data, err := xml.Marshal(u)
	assert.NoError(t, err)

	var result gen.UserConfirmed
	err = xml.Unmarshal(data, &result)
	assert.NoError(t, err)
	assert.False(t, result.GdprConsent)
}

func TestUserConfirmed_IsActiveFalse(t *testing.T) {
	u := makeValidUser()
	u.IsActive = false

	data, err := xml.Marshal(u)
	assert.NoError(t, err)

	var result gen.UserConfirmed
	err = xml.Unmarshal(data, &result)
	assert.NoError(t, err)
	assert.False(t, result.IsActive)
}

func TestUserConfirmed_InvalidXML(t *testing.T) {
	var u gen.UserConfirmed
	err := xml.Unmarshal([]byte("<broken"), &u)
	assert.Error(t, err)
}

// test a wrong root tag
func TestUserConfirmed_WrongRootTag(t *testing.T) {

	xmlStr := `<SomethingElse><id>abc</id></SomethingElse>`

	var u gen.UserConfirmed

	err := xml.Unmarshal([]byte(xmlStr), &u)
	assert.Error(t, err)

	assert.Equal(t, gen.UUIDType(""), u.Id)
}
