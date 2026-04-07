// Package logger new logging system that logs internal errors to kibana instead of to stdout
package logger

/*
import (
	"bytes"
	"log"
	"net/http"
	"time"

	// TODO(nasr): replace this no more json
	"encoding/json"
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
	Type      string    `json:"type"`
	Timestamp time.Time `json:"@timestamp"`
}

// logs to kibana only
func Log(message LogMessage) {

	// TODO(nasr): kibana implementation

	data, err := json.Marshal(message)
	if err != nil {
		log.Printf("error: %v\n", err)
	}

	req, err := http.NewRequest("POST", "", bytes.NewBuffer(data))
	if err != nil {
		log.Printf("error: %v\n", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
}

// logs to kibana only and to the stdout
func LogDebug(message LogMessage) {

	// TODO(nasr): kibana implementation

}

*/
