package heartbeat

import (
	"encoding/xml"

	"github.com/go-playground/validator/v10"
	"integration-project-ehb/controlroom/pkg/elastic"
)

type Heartbeat struct {
	XMLName   xml.Name `xml:"Heartbeat" json:"-"`
	ServiceID string   `xml:"serviceId" json:"service_id" validate:"required"`
	Timestamp string   `xml:"timestamp" json:"@timestamp" validate:"required"`
}

func Validate() error {
	for d := range msgsHeartbeat {
		var hb Heartbeat
		xml.Unmarshal(d.Body, &hb)
		if err := validate.Struct(hb); err == nil {
			log.Printf("Valid Heartbeat: %s\n", hb.ServiceID)
			SendToElastic(es, "heartbeats", hb)
		} else {
			log.Printf("Invalid Heartbeat received")
		}
	}
	return nil
}
