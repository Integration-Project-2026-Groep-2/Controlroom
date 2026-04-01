// Package internal_logger is een minimale zero-allocation JSON logger
// die logt naar stdout (voor Kibana/Elastic via log shipper) en naar
// journald voor severity hoger dan WARN.
package internal_logger

import (
	"bytes"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/coreos/go-systemd/v22/journal"
	"github.com/elastic/go-elasticsearch/v9"
	"github.com/elastic/go-elasticsearch/v9/esapi"
)

// Severity geeft het log-niveau aan.
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
	case TRACE:
		return "TRACE"
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

// Field is een getypeerd sleutel-waardepaar voor structured logging.
type Field struct {
	key    string
	strVal string
	intVal int64
	kind   fieldKind
}

type fieldKind uint8

const (
	kindString fieldKind = iota
	kindInt
)

func String(key, val string) Field {
	return Field{key: key, strVal: val, kind: kindString}
}

func Int(key string, val int) Field {
	return Field{key: key, intVal: int64(val), kind: kindInt}
}

var pool = sync.Pool{
	New: func() any { return new(bytes.Buffer) },
}

// Logger schrijft JSON log-regels naar een io.Writer zonder tussenliggende
// heap-allocaties. Voor severity hoger dan WARN wordt ook naar journald
// gestuurd. Als es ingesteld is worden logs asynchroon naar Elasticsearch
// gestuurd.
type Logger struct {
	w       io.Writer
	service string
	es      *elasticsearch.Client
	esIndex string
}

func New(w io.Writer, service string) *Logger {
	return &Logger{w: w, service: service}
}

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

	// bij severity hoger dan WARN ook naar journald sturen
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

	// asynchroon naar Elasticsearch sturen als client ingesteld is
	if l.es != nil {
		payload := make([]byte, buf.Len()-1) // zonder trailing \n
		copy(payload, buf.Bytes())
		go l.indexToElastic(payload, time.Now())
	}

	pool.Put(buf)
}

func (l *Logger) indexToElastic(payload []byte, t time.Time) {
	req := esapi.IndexRequest{
		Index:      l.esIndex,
		DocumentID: fmt.Sprintf("%s-%d", l.service, t.UnixNano()),
		Body:       bytes.NewReader(payload),
		Refresh:    "false",
	}
	res, err := req.Do(nil, l.es)
	if err != nil {
		return
	}
	res.Body.Close()
}

// writeEscaped schrijft s naar buf met minimale JSON-escaping (alleen " en \).
func writeEscaped(buf *bytes.Buffer, s string) {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '"' {
			buf.WriteString(`\"`)
		} else if c == '\\' {
			buf.WriteString(`\\`)
		} else {
			buf.WriteByte(c)
		}
	}
}

// writeInt schrijft een int64 naar buf zonder fmt of strconv allocaties.
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
