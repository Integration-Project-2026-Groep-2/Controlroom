package heartbeat

// TODO(nasr): replace with own validation

import (
	"github.com/go-playground/validator/v10"
	"encoding/xml"
)

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
