package logger

import (
	"bytes"
	"context"
	"io"
	"os"
	"sync"
	"time"

	"github.com/coreos/go-systemd/v22/journal"
	"github.com/elastic/go-elasticsearch/v9"
	"github.com/elastic/go-elasticsearch/v9/esapi"
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
	esClient *elasticsearch.Client
	esIndex  string
	out      io.Writer
	queue    chan []byte
)

var pool = sync.Pool{New: func() any { return new(bytes.Buffer) }}

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

func Init(addr, idx string, writer io.Writer, workers int) error {
	c, err := elasticsearch.NewClient(elasticsearch.Config{Addresses: []string{addr}})
	if err != nil {
		return err
	}

	esClient, esIndex, out, queue = c, idx, writer, make(chan []byte, 512)

	for range workers {
		go indexQueue()
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
	buf := pool.Get().(*bytes.Buffer)
	buf.Reset()

	var tmp [35]byte
	buf.WriteString(`{"level":"`)
	buf.WriteString(string(msg.severity))
	buf.WriteString(`","@timestamp":"`)
	buf.Write(time.Now().UTC().AppendFormat(tmp[:0], time.RFC3339Nano))
	buf.WriteString(`","service":"`)
	writeEscaped(buf, string(msg.service))
	buf.WriteString(`","msg":"`)
	writeEscaped(buf, string(msg.data))
	buf.WriteString("\"}\n")

	out.Write(buf.Bytes())

	switch msg.severity {
	case ERROR, FATAL, PANIC:
		journal.Send(msg.data, journalPriorities[msg.severity], map[string]string{
			"SERVICE":  string(msg.service),
			"SEVERITY": string(msg.severity),
		})
	}

	if esClient != nil {
		if n := buf.Len() - 1; n > 0 {
			payload := make([]byte, n)
			copy(payload, buf.Bytes())
			// dont block when the queu is full or down
			select {
			case queue <- payload:
			default:
			}
		}
	}

	pool.Put(buf)

	// an external service shouldn't be able to crash controlroom on command
	if msg.service == CONTROLROOM && (msg.severity == FATAL || msg.severity == PANIC) {
		os.Exit(1)
	}
}

func indexQueue() {
	for payload := range queue {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		res, err := (esapi.IndexRequest{
			Index:   esIndex,
			Body:    bytes.NewReader(payload),
			Refresh: "false",
		}).Do(ctx, esClient)
		cancel()
		if err == nil {
			res.Body.Close()
		}
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
