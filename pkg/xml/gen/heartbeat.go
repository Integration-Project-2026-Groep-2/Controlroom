package gen

import (
	"encoding/xml"
	"time"
)

type Heartbeat struct {
	XMLName   xml.Name  `xml:"Heartbeat" json:"heartbeat"`
	ServiceId string    `xml:"serviceId" json:"service_id"`
	Timestamp time.Time `xml:"timestamp" json:"timestamp"`
}
