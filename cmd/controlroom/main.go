package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/elastic/go-elasticsearch/v9"
	amqp "github.com/rabbitmq/amqp091-go"

	"integration-project-ehb/controlroom/internal/cr_rabbitmq"

	"integration-project-ehb/controlroom/cmd/config"
	"integration-project-ehb/controlroom/cmd/mcp"
	"integration-project-ehb/controlroom/internal/company"
	"integration-project-ehb/controlroom/internal/cr_logger"
	"integration-project-ehb/controlroom/internal/heartbeat"
	"integration-project-ehb/controlroom/internal/statuscheck"
	"integration-project-ehb/controlroom/internal/user"
	"integration-project-ehb/controlroom/internal/warning_producer"
	"integration-project-ehb/controlroom/pkg/logger"
	"integration-project-ehb/controlroom/pkg/watchdog"
)

func setup(ch *amqp.Channel) error {
	logger.Log(logger.NewMessage(logger.DEBUG, logger.CONTROLROOM, "declaring exchanges"))
	for _, def := range config.ConsumerDefinitions {
		if def.Passive {
			continue
		}

		if err := ch.ExchangeDeclare(def.Exchange.Name, def.Exchange.Kind, def.Exchange.Durable, false, false, false, nil); err != nil {
			return fmt.Errorf("exchange %s: %w", def.Exchange.Name, err)
		}
		logger.Log(logger.NewMessage(logger.DEBUG, logger.CONTROLROOM, fmt.Sprintf("declared exchange %s (%s)", def.Exchange.Name, def.Exchange.Kind)))
	}

	if err := ch.ExchangeDeclare("controlroom.dlx", "direct", true, false, false, false, nil); err != nil {
		return fmt.Errorf("dlx: %w", err)
	}

	//Setup producer
	if err := ch.ExchangeDeclare("news.topic", "topic", true, false, false, false, nil); err != nil {
		return fmt.Errorf("news.topic: %w", err)
	}

	// 2. FIX: Declare the Queue before binding it
	_, err := ch.QueueDeclare(
		"mailing.news.warning", // name
		true,                   // durable
		false,                  // auto-delete
		false,                  // exclusive
		false,                  // no-wait
		nil,                    // arguments
	)
	if err != nil {
		logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("failed to declare queue 'mailing.news.warning': %v", err)))
	}

	err = ch.QueueBind(
		"mailing.news.warning", // queue name
		"news.warning",         // routing key
		"news.topic",           // exchange
		false,
		nil,
	)

	if err != nil {
		logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("Error binding queue to exchange: %v", err)))
	}

	logger.Log(logger.NewMessage(logger.DEBUG, logger.CONTROLROOM, "declared DLX exchange"))

	for _, def := range config.ConsumerDefinitions {
		if def.Passive {
			continue
		}

		if _, err := ch.QueueDeclare(def.Queue.Name, def.Queue.Durable, false, false, false, nil); err != nil {
			return fmt.Errorf("queue %s: %w", def.Queue.Name, err)
		}
		logger.Log(logger.NewMessage(logger.DEBUG, logger.CONTROLROOM, fmt.Sprintf("declared queue %s", def.Queue.Name)))

		err := cr_rabbitmq.SetupDLQ(ch, def.DLQName)
		if err != nil {

			logger.Log(logger.NewMessage(logger.WARN, logger.CONTROLROOM, fmt.Sprintf("error declaring DLQ %v", err)))
		} else {

			logger.Log(logger.NewMessage(logger.DEBUG, logger.CONTROLROOM, fmt.Sprintf("declared DLQ %s", def.DLQName)))

		}

		if err := ch.QueueBind(def.Queue.Name, def.Binding.Key, def.Exchange.Name, false, nil); err != nil {
			return fmt.Errorf("bind %s: %w", def.Queue.Name, err)
		}
		logger.Log(logger.NewMessage(logger.DEBUG, logger.CONTROLROOM, fmt.Sprintf("bound %s -> %s (key: %s)", def.Queue.Name, def.Exchange.Name, def.Binding.Key)))

		dlqRoutingKey := def.Queue.Name + ".failed"
		if err := ch.QueueBind(def.DLQName, dlqRoutingKey, "controlroom.dlx", false, nil); err != nil {
			return fmt.Errorf("bind dlq %s: %w", def.DLQName, err)
		}
		logger.Log(logger.NewMessage(logger.DEBUG, logger.CONTROLROOM, fmt.Sprintf("bound DLQ %s -> controlroom.dlx (key: %s)", def.DLQName, dlqRoutingKey)))
	}

	logger.Log(logger.NewMessage(logger.INFO, logger.CONTROLROOM, "RabbitMQ topology setup complete"))
	return nil
}

func startSession(ctx context.Context, client *elasticsearch.Client) error {
	logger.Log(logger.NewMessage(logger.INFO, logger.CONTROLROOM, "dialing RabbitMQ"))
	conn, err := amqp.Dial(os.Getenv("RABBITMQ_URL"))
	if err != nil {
		logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("failed to dial RabbitMQ: %v", err)))
		return fmt.Errorf("dial: %w", err)
	}
	defer conn.Close()
	logger.Log(logger.NewMessage(logger.INFO, logger.CONTROLROOM, "connected to RabbitMQ"))

	closeCh := conn.NotifyClose(make(chan *amqp.Error, 1))

	setupCh, err := conn.Channel()

	if err != nil {
		logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("failed to open setup channel: %v", err)))
		return fmt.Errorf("setup channel: %w", err)
	}
	if err := setup(setupCh); err != nil {
		logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("topology setup failed: %v", err)))
		setupCh.Close()
		return err
	}
	setupCh.Close()

	dlqCh, err := conn.Channel()
	if err != nil {
		logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("failed to open DLQ channel: %v", err)))
		return fmt.Errorf("dlq channel: %w", err)
	}
	defer dlqCh.Close()

	for _, def := range config.ConsumerDefinitions {
		ch, err := conn.Channel()
		if err != nil {
			logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("failed to open channel for %s: %v", def.Queue.Name, err)))
			return fmt.Errorf("channel for %s: %w", def.Queue.Name, err)
		}
		defer ch.Close()

		msgs, err := ch.Consume(def.Queue.Name, fmt.Sprintf("controlroom-%d", os.Getpid()), false, false, false, false, nil)

		if err != nil {
			logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("failed to setup consumer for %s: %v", def.Queue.Name, err)))
			return fmt.Errorf("setup %s: %w", def.Queue.Name, err)
		}

		if err := ch.Qos(def.Qos, 0, false); err != nil {
			logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("failed to set QoS for %s: %v", def.Queue.Name, err)))
			return fmt.Errorf("qos %s: %w", def.Queue.Name, err)
		}

		cfg := &cr_rabbitmq.ConsumerConfig{
			Client:  client,
			DLQCh:   dlqCh,
			DLQName: def.DLQName,
		}

		switch def.Type {
		case config.HEARTBEAT:
			go cr_rabbitmq.Consume(cfg, msgs, ctx, heartbeat.ProcessHeartbeat)
		case config.LOGGER:
			go cr_rabbitmq.Consume(cfg, msgs, ctx, cr_logger.ProcessLog)
		case config.STATUSCHECK:
			go cr_rabbitmq.Consume(cfg, msgs, ctx, statuscheck.ProcessStatusCheck)
		case config.USER:
			go cr_rabbitmq.Consume(cfg, msgs, ctx, user.ProcessUser)
		case config.COMPANY:
			go cr_rabbitmq.Consume(cfg, msgs, ctx, company.ProcessCompany)
		}
		logger.Log(logger.NewMessage(logger.INFO, logger.CONTROLROOM, fmt.Sprintf("%s consumer started (qos: %d)", def.Queue.Name, def.Qos)))
	}

	logger.Log(logger.NewMessage(logger.INFO, logger.CONTROLROOM, "all consumers running, waiting for messages"))

	pubCh, err := conn.Channel()
	if err != nil {
		logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("producer channel: %v", err)))
	}
	defer pubCh.Close()
	go warning_producer.RunWarningProducer(client, ctx, pubCh)

	logger.Log(logger.NewMessage(logger.INFO, logger.CONTROLROOM, "news producer started (interval: 120s)"))

	select {
	case reason := <-closeCh:
		if reason == nil {
			logger.Log(logger.NewMessage(logger.WARN, logger.CONTROLROOM, "RabbitMQ connection closed unexpectedly"))
			return fmt.Errorf("connection closed")
		}
		logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("RabbitMQ connection lost: %v", reason)))
		return fmt.Errorf("connection lost: %w", reason)
	case <-ctx.Done():
		logger.Log(logger.NewMessage(logger.INFO, logger.CONTROLROOM, "context cancelled, closing RabbitMQ session"))
		return ctx.Err()
	}
}

func main() {
	if err := logger.Init(&config.ElasticConfig, "controlroom-logs", os.Stdout, 4); err != nil {
		fmt.Fprintf(os.Stderr, "logger init: %v\n", err)
		os.Exit(4)
	}
	defer logger.Shutdown()

	client, err := elasticsearch.NewClient(config.ElasticConfig)
	if err != nil {
		logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("elasticsearch client config: %v", err)))
		os.Exit(5)
	}

	res, err := client.Info()
	if err != nil {
		log.Printf("error here: %v\n", err)
		logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("elasticsearch connect: %v", err)))
		os.Exit(6)
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

	// initialized mcp server
	// NOTE(nasr): runs alongside the RabbitMQ session loop so mcp-master can
	// reach our tools over SSE without blocking the consumer goroutines.
	go func() {
		err := mcp.SetupMCP(client)
		if err != nil {
			logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("mcp server exited: %v", err)))
		}
	}()

	if watchdog.WDWebhook == "" {
		logger.Log(logger.NewMessage(logger.WARN, logger.CONTROLROOM, "CRITICAL: TEAMS_WEBHOOK_URL is niet ingesteld in de environment!"))
	}

	for _, svc := range watchdog.WDServices {
		watchdog.WDServiceState[svc] = true
	}

	go watchdog.ProcessAlertQueue()

	go func() {
		ticker := time.NewTicker(5 * time.Second)
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
