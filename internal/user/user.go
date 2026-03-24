package user

import (
	"integration-project-ehb/controlroom/pkg/elastic"

	"github.com/go-playground/validator/v10"

	"encoding/xml"
	"fmt"
)

func ValidateAndSend(es *elasticsearch.Client) error {
	for d := range msgsUser {
		var u User
		xml.Unmarshal(d.Body, &u)
		if err := validate.Struct(u); err == nil {
			log.Printf("Valid User: %s\n", u.ID)
			SendToElastic(es, "users", u)
			return nil
		} else {
			return fmt.Errorf("Invalid User received")
		}
	}

	return nil
}
