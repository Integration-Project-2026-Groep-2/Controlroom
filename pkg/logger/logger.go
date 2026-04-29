package logger

import (
	"bytes"
	"context"
	"io"
	"os"
	"sync"
	"sync/atomic"
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
	dropped  uint64
)

// Dropped retourneert het aantal log-berichten dat is gedropt omdat de
// async-queue vol stond. Bedoeld voor monitoring / metrics.
func Dropped() uint64 { return atomic.LoadUint64(&dropped) }

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
			// dont block when the queu is full or down — tel drops zodat ze zichtbaar zijn
			select {
			case queue <- payload:
			default:
				atomic.AddUint64(&dropped, 1)
			}
		}
	}

	pool.Put(buf)

	// an external service shouldn't be able to crash controlroom on command
	if msg.service == CONTROLROOM && (msg.severity == FATAL || msg.severity == PANIC) {
		os.Exit(1)
	}
}

// IndexLogsQueue verbruikt de queue en stuurt logs in batches naar Elasticsearch via
// de _bulk API. Per worker wordt geflusht zodra batchSize bereikt is OF na
// flushInterval. Bulk-indexering verhoogt de drain-rate ~50× t.o.v. per-doc
// IndexRequest, waardoor de queue onder normale load niet meer vult.
func IndexLogsQueue() {
	const (
		batchSize     = 500
		flushInterval = 200 * time.Millisecond
	)

	var buf bytes.Buffer
	count := 0
	timer := time.NewTimer(flushInterval)
	if !timer.Stop() {
		<-timer.C
	}
	timerActive := false

	header := []byte(`{"index":{}}` + "\n")

	flush := func() {
		if count == 0 {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		res, err := (esapi.BulkRequest{
			Index: esIndex,
			Body:  bytes.NewReader(buf.Bytes()),
		}).Do(ctx, esClient)
		cancel()
		if err == nil {
			res.Body.Close()
		}
		buf.Reset()
		count = 0
	}

	for {
		select {
		case payload, ok := <-queue:
			if !ok {
				flush()
				return
			}
			buf.Write(header)
			buf.Write(payload)
			buf.WriteByte('\n')
			count++
			if !timerActive {
				timer.Reset(flushInterval)
				timerActive = true
			}
			if count >= batchSize {
				if !timer.Stop() {
					<-timer.C
				}
				timerActive = false
				flush()
			}
		case <-timer.C:
			timerActive = false
			flush()
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

