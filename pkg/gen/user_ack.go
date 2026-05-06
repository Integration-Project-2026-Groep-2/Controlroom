package gen

import (
	"encoding/xml"
	"time"
)

// supressing unused-import errors before goimports is run.
var _ = xml.Name{}
var _ = time.Time{}

// UserAck representeert het user_ack.xsd contract
type UserAck struct {
	XMLName xml.Name  `xml:"UserDoc" json:"-"`
	UserId  UUIDType  `xml:"id" json:"id" validate:"required"`
	Indexed time.Time `xml:"indexed" json:"indexed" validate:"required"`
	Service string    `xml:"service" json:"service" validate:"required"`
}
