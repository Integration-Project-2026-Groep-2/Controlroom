package user

import (
	"github.com/go-playground/validator/v10"
	"encoding/xml"
)

type User struct {
	XMLName     xml.Name `xml:"User" json:"-"`
	ID          string   `xml:"id" json:"user_id" validate:"required"`
	Type        string   `xml:"type" json:"user_type" validate:"required,oneof=human service external"`
	Organisatie string   `xml:"organisatie" json:"organisatie" validate:"required"`
	Datum       string   `xml:"datum" json:"@timestamp" validate:"required"`
}


func Validate() error {
	for d := range msgsUser {
		var u User
		xml.Unmarshal(d.Body, &u)
		if err := validate.Struct(u); err == nil {
			log.Printf("Valid User: %s\n", u.ID)
			sendToElastic(es, "users", u)
			return nil
		} else {
			return fmt.Errorf("Invalid User received")
		}
	}
}

