package logger

import (
	"bytes"
	"context"
	"fmt"
	"integration-project-ehb/controlroom/pkg/gen"
	"io"
	"os"
	"time"

	"github.com/coreos/go-systemd/v22/journal"
	"github.com/elastic/go-elasticsearch/v9"
	"github.com/elastic/go-elasticsearch/v9/esapi"
	"github.com/mailru/easyjson"
)

type Severity string
type Service string

const (
	DEBUG Severity = "DEBUG"
	INFO  Severity = "INFO"
	WARN  Severity = "WARN"
	ERROR Severity = "ERROR"
	FATAL Severity = "FATAL"
	PANIC Severity = "PANIC"
)

// TODO(nasr): replace with environment variables
const (
	CONTROLROOM Service = "CONTROLROOM"
	CRM         Service = "CRM"
	KASSA       Service = "KASSA"
	FACTURATIE  Service = "FACTURATIE"
	MAILING     Service = "MAILING"
	FRONTEND    Service = "FRONTEND"
	PLANNING    Service = "PLANNING"
)

var (
	Client *elasticsearch.Client
	Index  string
	out    io.Writer
	queue  chan gen.LogDoc
)

var journalPriorities = map[Severity]journal.Priority{
	DEBUG: journal.PriDebug,
	INFO:  journal.PriInfo,
	WARN:  journal.PriWarning,
	ERROR: journal.PriErr,
	FATAL: journal.PriCrit,
	PANIC: journal.PriCrit,
}

func NewMessage(sev Severity, svc Service, data string) Message {
	return Message{severity: sev, service: svc, data: data}
}

func Init(config *elasticsearch.Config, idx string, writer io.Writer, workers int) error {

	c, err := elasticsearch.NewClient(*config)

	if err != nil {
		return err
	}

	Client, Index, out, queue = c, idx, writer, make(chan gen.LogDoc, 512)

	for range workers {
		go IndexLogsQueue()
	}

	return nil
}

func Shutdown() { close(queue) }

type Message struct {
	severity Severity
	service  Service
	data     string
}

func Log(msg Message) {

	data := gen.LogDoc{
		Level:     gen.SeverityType(msg.severity),
		Timestamp: time.Now().UTC(),
		Service:   string(msg.service),
		Data:      string(msg.data),
		Indexed:   time.Now().UTC(),
	}

	switch msg.severity {
	case ERROR, FATAL, PANIC:
		journal.Send(msg.data, journalPriorities[msg.severity], map[string]string{
			"SERVICE":  string(msg.service),
			"SEVERITY": string(msg.severity),
		})
	}

	if Client != nil {
		select {
		case queue <- data:
		default:
		}
	}

	// an external service shouldn't be able to crash controlroom on command
	if msg.service == CONTROLROOM && (msg.severity == FATAL || msg.severity == PANIC) {
		os.Exit(1)
	}
}

func writeEscaped(buf *bytes.Buffer, s string) {
	const hex = "0123456789abcdef"
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '"':
			buf.WriteString(`\"`)
		case c == '\\':
			buf.WriteString(`\\`)
		case c == '\n':
			buf.WriteString(`\n`)
		case c == '\r':
			buf.WriteString(`\r`)
		case c == '\t':
			buf.WriteString(`\t`)
		case c < 0x20:
			buf.WriteString(`\u00`)
			buf.WriteByte(hex[c>>4])
			buf.WriteByte(hex[c&0xF])
		default:
			buf.WriteByte(c)
		}
	}
}

func IndexLogsQueue() {
	for payload := range queue {
		data, err := easyjson.Marshal(payload)
		if err != nil {
			fmt.Printf("marshal error: %v\n", err)
			continue
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

		req := esapi.IndexRequest{
			Index:   Index,
			Body:    bytes.NewReader(data),
			Refresh: "false",
		}

		res, err := req.Do(ctx, Client)
		cancel()

		if err != nil {
			fmt.Printf("index logs error: %v\n", err)
			continue
		}

		res.Body.Close()

		if res.IsError() {
			fmt.Printf("index logs error response: %v\n", res.Status())
			continue
		}
	}
}
