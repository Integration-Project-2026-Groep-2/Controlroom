package gen

import (
	"encoding/xml"
	"time"
)

type Heartbeat struct {
	XMLName   xml.Name  `xml:"heartbeat" json:"heartbeat"`
	ServiceId string    `xml:"serviceId" json:"service_id" validate:"required"`
	Timestamp time.Time `xml:"timestamp" json:"timestamp" validate:"required"`
}
