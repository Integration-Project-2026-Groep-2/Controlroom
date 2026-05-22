package logger

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/elastic/go-elasticsearch/v9/esapi"
	"github.com/mailru/easyjson"
)

// note(nasr): this is anoying because I liked to declare the debug mode in the globaal configuration but it was causing a 4 file import cylcle error ggrrrrrrr
var debugMode = strings.EqualFold(os.Getenv("DebugMode"), "true")

func IndexLogsQueue() {

	for payload := range queue {

		if debugMode {
			log.Println(payload)
		}

		data, err := easyjson.Marshal(payload)
		if err != nil {
			if debugMode {
				fmt.Printf("marshal error: %v\n", err)
			}
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
			if debugMode {
				fmt.Printf("index logs error: %v\n", err)
			}
			continue
		}

		res.Body.Close()

		if res.IsError() {
			if debugMode {
				fmt.Printf("index logs error response: %v\n", res.Status())
			}
			continue
		}
	}
}
