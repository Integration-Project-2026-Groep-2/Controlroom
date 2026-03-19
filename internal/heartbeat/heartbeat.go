package heartbeat

import (
	"github.com/go-playground/validator/v10"
	"encoding/xml"
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
			sendToElastic(es, "heartbeats", hb)
		} else {
			log.Printf("Invalid Heartbeat received")
		}
	}
}
