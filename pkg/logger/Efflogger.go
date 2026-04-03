package logger

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/coreos/go-systemd/v22/journal"
	"github.com/elastic/go-elasticsearch/v9"
	"github.com/elastic/go-elasticsearch/v9/esapi"
)

// --- Severity ---

type Severity int8

const (
	DEBUG Severity = 1
	INFO  Severity = 2
	WARN  Severity = 3
	ERROR Severity = 4
	FATAL Severity = 5
	PANIC Severity = 6
)

func (s Severity) string() string {
	switch s {
	case DEBUG:
		return "DEBUG"
	case INFO:
		return "INFO"
	case WARN:
		return "WARN"
	case ERROR:
		return "ERROR"
	case FATAL:
		return "FATAL"
	case PANIC:
		return "PANIC"
	default:
		return "UNKNOWN"
	}
}

func (s Severity) journalPriority() journal.Priority {
	switch s {
	case DEBUG:
		return journal.PriDebug
	case INFO:
		return journal.PriInfo
	case WARN:
		return journal.PriWarning
	case ERROR:
		return journal.PriErr
	case FATAL, PANIC:
		return journal.PriCrit
	default:
		return journal.PriInfo
	}
}

// --- Field ---

type fieldKind uint8

const (
	kindString fieldKind = iota
	kindInt
)

// Field is a typed key-value pair for structured logging.
type Field struct {
	key    string
	strVal string
	intVal int64
	kind   fieldKind
}

// String returns a Field with a string value.
func String(key, val string) Field {
	return Field{key: key, strVal: val, kind: kindString}
}

// Int returns a Field with an integer value.
func Int(key string, val int) Field {
	return Field{key: key, intVal: int64(val), kind: kindInt}
}

// --- Logger ---

var pool = sync.Pool{
	New: func() any { return new(bytes.Buffer) },
}

// Logger writes JSON log lines to an io.Writer without intermediate heap
// allocations. For severity above WARN it also writes to journald. If es is
// set, logs are sent asynchronously to Elasticsearch.
type Logger struct {
	w       io.Writer
	service string
	es      *elasticsearch.Client
	esIndex string
	wg      sync.WaitGroup // tracks in-flight Elasticsearch goroutines
}

// New returns a Logger that writes to w.
func New(w io.Writer, service string) *Logger {
	return &Logger{w: w, service: service}
}

// NewWithElastic returns a Logger that writes to w and indexes logs into Elasticsearch.
func NewWithElastic(w io.Writer, service string, es *elasticsearch.Client, index string) *Logger {
	return &Logger{w: w, service: service, es: es, esIndex: index}
}

func (l *Logger) Debug(msg string, fields ...Field) {
	l.log(DEBUG, msg, nil, fields)
}

func (l *Logger) Info(msg string, fields ...Field) {
	l.log(INFO, msg, nil, fields)
}

func (l *Logger) Warn(msg string, fields ...Field) {
	l.log(WARN, msg, nil, fields)
}

func (l *Logger) Error(msg string, err error, fields ...Field) {
	l.log(ERROR, msg, err, fields)
}

func (l *Logger) Fatal(msg string, err error, fields ...Field) {
	l.log(FATAL, msg, err, fields)
	l.wg.Wait()
	os.Exit(1)
}

func (l *Logger) Panic(msg string, err error, fields ...Field) {
	l.log(PANIC, msg, err, fields)
	l.wg.Wait()
	panic(msg)
}

func (l *Logger) log(sev Severity, msg string, err error, fields []Field) {
	buf := pool.Get().(*bytes.Buffer)
	buf.Reset()

	buf.WriteString(`{"level":"`)
	buf.WriteString(sev.string())
	buf.WriteString(`","@timestamp":"`)
	var tmp [35]byte
	buf.Write(time.Now().UTC().AppendFormat(tmp[:0], time.RFC3339Nano))
	buf.WriteString(`","service":"`)
	writeEscaped(buf, l.service)
	buf.WriteString(`","msg":"`)
	writeEscaped(buf, msg)
	buf.WriteByte('"')

	if err != nil {
		buf.WriteString(`,"error":"`)
		writeEscaped(buf, err.Error())
		buf.WriteByte('"')
	}

	for _, f := range fields {
		buf.WriteString(`,"`)
		buf.WriteString(f.key)
		buf.WriteString(`":`)
		switch f.kind {
		case kindString:
			buf.WriteByte('"')
			writeEscaped(buf, f.strVal)
			buf.WriteByte('"')
		case kindInt:
			writeInt(buf, f.intVal)
		}
	}

	buf.WriteString("}\n")
	l.w.Write(buf.Bytes()) //nolint:errcheck

	// also send to journald for severity above WARN
	if sev > WARN {
		vars := map[string]string{
			"SERVICE":  l.service,
			"SEVERITY": sev.string(),
		}
		if err != nil {
			vars["ERROR"] = err.Error()
		}
		journal.Send(msg, sev.journalPriority(), vars) //nolint:errcheck
	}

	// send to Elasticsearch asynchronously if client is set
	if l.es != nil {
		if n := buf.Len() - 1; n > 0 { // exclude trailing \n; ES expects raw JSON
			payload := make([]byte, n)
			copy(payload, buf.Bytes())
			l.wg.Add(1)
			go l.indexToElastic(payload, time.Now())
		}
	}

	pool.Put(buf)
}

func (l *Logger) indexToElastic(payload []byte, t time.Time) {
	defer l.wg.Done()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req := esapi.IndexRequest{
		Index:      l.esIndex,
		DocumentID: fmt.Sprintf("%s-%d", l.service, t.UnixNano()),
		Body:       bytes.NewReader(payload),
		Refresh:    "false",
	}
	res, err := req.Do(ctx, l.es)
	if err != nil {
		return
	}
	res.Body.Close()
}

// --- Helpers ---

// writeEscaped writes s into buf with JSON string escaping per RFC 8259.
// All control characters (0x00–0x1F) and the two structural characters
// " and \ are escaped. No heap allocations.
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
			// Remaining control characters: emit \u00XX
			buf.WriteByte('\\')
			buf.WriteByte('u')
			buf.WriteByte('0')
			buf.WriteByte('0')
			buf.WriteByte(hex[c>>4])
			buf.WriteByte(hex[c&0xF])
		default:
			buf.WriteByte(c)
		}
	}
}

// writeInt writes an int64 into buf without fmt or strconv allocations.
func writeInt(buf *bytes.Buffer, v int64) {
	if v == 0 {
		buf.WriteByte('0')
		return
	}
	if v < 0 {
		buf.WriteByte('-')
		v = -v
	}
	var tmp [20]byte
	pos := len(tmp)
	for v > 0 {
		pos--
		tmp[pos] = byte('0' + v%10)
		v /= 10
	}
	buf.Write(tmp[pos:])
}
