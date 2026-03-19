package user

// TODO(nasr): replace with own validation

import (
	"github.com/go-playground/validator/v10"
	"encoding/xml"
)

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

