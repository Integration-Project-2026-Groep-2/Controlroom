// new logging system that logs internal errors to kibana instead of to stdout
package internal_logger

import (
	"bytes"
	"log"
	"net/http"
	"time"

	// for logging extra information to journald
	// the more debug endpoints we have the better
	"github.com/coreos/go-systemd/v22/journal"
)

type Severity int8

const (
	DEBUG Severity = iota
	INFO
	WARN
	ERROR
	FATAL
	PANIC
	TRACE
)

type LogMessage struct {
	Message   string    `json:"message"`
	Error     string    `json:"error,omitempty"`
	Service   string    `json:"service"`
	Severity  string    `json:"severity"`
	Timestamp time.Time `json:"@timestamp"`
}

// logs to elastic only
func Log(message LogMessage, err error, service string, sev Severity) {

	if strings.Compare(message, "") {
		message = err.Error
	}

	// if the severity is higher than a warning than we also log to journald
	if sev > WARN {

		// join the error to the messsage so we have one big one to send to journald
		message.Join(err.Error)

		journal.Send("Something happened", journal.PriInfo, map[string]string{
			"SERVICE": sev,
			"ERROR":   message,
		})

	}

	return LogMessage{
		Message:   message,
		Error:     err.Error(),
		Service:   service,
		Severity:  sev,
		Timestamp: time.Now(),
	}
}

// logs to elastic only to the stdout
func LogDebug(message LogMessage) error {

	if strings.Compare(message, "") {
		message = err.Error
	}

	message := LogMessage{
		Message:   message,
		Error:     err.Error(),
		Service:   service,
		Severity:  sev,
		Timestamp: time.Now(),
	}

	// log to stdout
	log.Printf("%+v\n", message)

	// return the message to log it to elastic
	return message

}
