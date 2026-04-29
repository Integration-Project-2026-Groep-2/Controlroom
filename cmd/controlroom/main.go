package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/elastic/go-elasticsearch/v9"
	amqp "github.com/rabbitmq/amqp091-go"


	"integration-project-ehb/controlroom/internal/cr_rabbitmq"

	"integration-project-ehb/controlroom/internal/company"
	"integration-project-ehb/controlroom/internal/cr_logger"
	"integration-project-ehb/controlroom/internal/heartbeat"
	"integration-project-ehb/controlroom/internal/statuscheck"
	"integration-project-ehb/controlroom/internal/user"
	"integration-project-ehb/controlroom/pkg/logger"
	"integration-project-ehb/controlroom/pkg/watchdog"
)

type cr_consumer_t int8

const (
	HEARTBEAT   cr_consumer_t = iota
	LOGGER
	STATUSCHECK
	USER
	COMPANY
)

type ConsumerDef struct {

	Type      cr_consumer_t
	Exchange  cr_rabbitmq.ExchangeInfo
	Queue     cr_rabbitmq.QueueInfo
	Binding   cr_rabbitmq.BindingInfo
	DLQName   string
	Qos       int
}

var consumerDefinitions = []ConsumerDef{
	{
		Type:      HEARTBEAT,
		Exchange:  cr_rabbitmq.ExchangeInfo{Name: "heartbeat.direct", Kind: "direct", Durable: true},
		Queue:     cr_rabbitmq.QueueInfo{Name: "controlroom.heartbeat.queue", Durable: true},
		Binding:   cr_rabbitmq.BindingInfo{Key: "routing.heartbeat"},
		DLQName:   "controlroom.heartbeat.queue.dlq",
		Qos:       18,
	},
	{
		Type:      STATUSCHECK,
		Exchange:  cr_rabbitmq.ExchangeInfo{Name: "statuscheck.direct", Kind: "direct", Durable: true},
		Queue:     cr_rabbitmq.QueueInfo{Name: "controlroom.statuscheck.queue", Durable: true},
		Binding:   cr_rabbitmq.BindingInfo{Key: "routing.statuscheck"},
		DLQName:   "controlroom.statuscheck.queue.dlq",
		Qos:       5,
	},
	{
		Type:      STATUSCHECK,
		Exchange:  cr_rabbitmq.ExchangeInfo{Name: "contact.topic", Kind: "topic", Durable: true},
		Queue:     cr_rabbitmq.QueueInfo{Name: "crm.user.confirmed", Durable: true},
		Binding:   cr_rabbitmq.BindingInfo{Key: "crm.user.confirmed"},
		DLQName:   "crm.user.confirmed.dlq",
		Qos:       10,
	},
	{
		Type:      COMPANY,
		Exchange:  cr_rabbitmq.ExchangeInfo{Name: "contact.topic", Kind: "topic", Durable: true},
		Queue:     cr_rabbitmq.QueueInfo{Name: "crm.company.confirmed", Durable: true},
		Binding:   cr_rabbitmq.BindingInfo{Key: "crm.company.confirmed"},
		DLQName:   "crm.company.confirmed.dlq",
		Qos:       10,
	},
	{
		Type:      LOGGER,
		Exchange:  cr_rabbitmq.ExchangeInfo{Name: "logs.direct", Kind: "direct", Durable: true},
		Queue:     cr_rabbitmq.QueueInfo{Name: "controlroom.logs.queue", Durable: true},
		Binding:   cr_rabbitmq.BindingInfo{Key: "routing.log"},
		DLQName:   "controlroom.logs.queue.dlq",
		Qos:       5,
	},
}

func setupCRRQ(ch *amqp.Channel) error {
	for _, def := range consumerDefinitions {
		if err := ch.ExchangeDeclare(def.Exchange.Name, def.Exchange.Kind, def.Exchange.Durable, false, false, false, nil); err != nil {
			return fmt.Errorf("exchange %s: %w", def.Exchange.Name, err)
		}
	}

	if err := ch.ExchangeDeclare("controlroom.dlx", "direct", true, false, false, false, nil); err != nil {
		return fmt.Errorf("dlx: %w", err)
	}

	for _, def := range consumerDefinitions {
		if _, err := ch.QueueDeclare(def.Queue.Name, def.Queue.Durable, false, false, false, nil); err != nil {
			return fmt.Errorf("queue %s: %w", def.Queue.Name, err)
		}

		if _, err := ch.QueueDeclare(def.DLQName, true, false, false, false, nil); err != nil {
			return fmt.Errorf("dlq %s: %w", def.DLQName, err)
		}

		if err := ch.QueueBind(def.Queue.Name, def.Binding.Key, def.Exchange.Name, false, nil); err != nil {
			return fmt.Errorf("bind %s: %w", def.Queue.Name, err)
		}

		dlqRoutingKey := def.Queue.Name + ".failed"
		if err := ch.QueueBind(def.DLQName, dlqRoutingKey, "controlroom.dlx", false, nil); err != nil {
			return fmt.Errorf("bind dlq %s: %w", def.DLQName, err)
		}
	}

	return nil
}

func startSession(ctx context.Context, client *elasticsearch.Client) error {
	conn, err := amqp.Dial(os.Getenv("RABBITMQ_URL"))
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer conn.Close()
	logger.Log(logger.NewMessage(logger.INFO, logger.CONTROLROOM, "connected to RabbitMQ"))

	closeCh := conn.NotifyClose(make(chan *amqp.Error, 1))

	setupCh, err := conn.Channel()
	if err != nil {
		return fmt.Errorf("setup channel: %w", err)
	}
	if err := setupCRRQ(setupCh); err != nil {
		setupCh.Close()
		return err
	}
	setupCh.Close()

	dlqCh, err := conn.Channel()
	if err != nil {
		return fmt.Errorf("dlq channel: %w", err)
	}
	defer dlqCh.Close()

	for _, def := range consumerDefinitions {
		ch, err := conn.Channel()
		if err != nil {
			return fmt.Errorf("channel for %s: %w", def.Queue.Name, err)
		}
		defer ch.Close()

		msgs, err := cr_rabbitmq.SetupConsumer(ch, def.Exchange, def.Queue, def.Binding)

		if err != nil {
			return fmt.Errorf("setup %s: %w", def.Queue.Name, err)
		}

		if err := ch.Qos(def.Qos, 0, false); err != nil {
			return fmt.Errorf("qos %s: %w", def.Queue.Name, err)
		}

		cfg := &cr_rabbitmq.ConsumerConfig{
			DLQCh:   dlqCh,
			DLQName: def.DLQName,
		}

		switch def.Type {

		case HEARTBEAT:
			go cr_rabbitmq.Consume(cfg, msgs, ctx, heartbeat.ProcessHeartbeat)
		case  LOGGER:
			go cr_rabbitmq.Consume(cfg, msgs, ctx, cr_logger.ProcessLog)
		case  STATUSCHECK:
			go cr_rabbitmq.Consume(cfg, msgs, ctx, statuscheck.ProcessStatusCheck)
		case  USER:
			go cr_rabbitmq.Consume(cfg, msgs, ctx, user.ProcessUser)
		case  COMPANY:
			go cr_rabbitmq.Consume(cfg, msgs, ctx, company.ProcessCompany)

		}

		logger.Log(logger.NewMessage(logger.INFO, logger.CONTROLROOM, fmt.Sprintf("%s consumer started", def.Queue.Name)))
	}

	select {
	case reason := <-closeCh:
		if reason == nil {
			return fmt.Errorf("connection closed")
		}
		return fmt.Errorf("connection lost: %w", reason)
	case <-ctx.Done():
		return ctx.Err()
	}
}

func main() {
	cfg := elasticsearch.Config{
		Addresses: []string{os.Getenv("ELASTICSEARCH_URL")},
		Username:  os.Getenv("CONTROLROOM_ES_USER"),
		Password:  os.Getenv("CONTROLROOM_ES_PASS"),
	}

	if err := logger.Init(os.Getenv("ELASTICSEARCH_URL"), "controlroom-logs", os.Stdout, 4); err != nil {
		fmt.Fprintf(os.Stderr, "logger init: %v\n", err)
		os.Exit(1)
	}
	defer logger.Shutdown()

	client, err := elasticsearch.NewClient(cfg)
	if err != nil {
		logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("elasticsearch client config: %v", err)))
		os.Exit(1)
	}

	res, err := client.Info()
	if err != nil {
		logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("elasticsearch connect: %v", err)))
		os.Exit(1)
	}
	res.Body.Close()

	logger.Log(logger.NewMessage(logger.INFO, logger.CONTROLROOM, "connected to elasticsearch"))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		logger.Log(logger.NewMessage(logger.INFO, logger.CONTROLROOM, "shutdown signal received, draining queues..."))
		cancel()
	}()

	if watchdog.WDWebhook == "" {
		logger.Log(logger.NewMessage(logger.WARN, logger.CONTROLROOM, "CRITICAL: TEAMS_WEBHOOK_URL is niet ingesteld in de environment!"))
	}

	for _, svc := range watchdog.WDServices {
		watchdog.WDServiceState[svc] = true
	}

	go watchdog.ProcessAlertQueue()

	go func() {
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				watchdog.CheckHeartbeats(client)
			case <-ctx.Done():
				return
			}
		}
	}()

	const (
		initialBackoff = 1 * time.Second
		maxBackoff     = 60 * time.Second
		healthyAfter   = 10 * time.Second
	)

	backoff := initialBackoff

	for {
		start := time.Now()
		err := startSession(ctx, client)

		if errors.Is(err, context.Canceled) {
			return
		}

		if time.Since(start) > healthyAfter {
			backoff = initialBackoff
		}

		logger.Log(logger.NewMessage(logger.WARN, logger.CONTROLROOM, fmt.Sprintf("rabbit session ended, redialing in %s: %v", backoff, err)))

		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			return
		}

		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}
