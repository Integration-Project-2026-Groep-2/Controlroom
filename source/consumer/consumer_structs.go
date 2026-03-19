package consumer


type consumerType struct {
	XMLName   xml.Name `xml:"Heartbeat" json:"-"`
	ServiceID string   `xml:"serviceId" json:"service_id" validate:"required"`
	Timestamp string   `xml:"timestamp" json:"@timestamp" validate:"required"`
}
