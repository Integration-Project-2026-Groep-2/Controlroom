package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/elastic/go-elasticsearch/v9"
	amqp "github.com/rabbitmq/amqp091-go"

	"integration-project-ehb/controlroom/internal/checkin"
	"integration-project-ehb/controlroom/internal/company"
	config "integration-project-ehb/controlroom/internal/cr_config"
	"integration-project-ehb/controlroom/internal/cr_logger"
	"integration-project-ehb/controlroom/internal/cr_rabbitmq"
	"integration-project-ehb/controlroom/internal/dashboard_sync"
	"integration-project-ehb/controlroom/internal/jarvis_metrics"
	"integration-project-ehb/controlroom/internal/heartbeat"
	"integration-project-ehb/controlroom/internal/k8retriever"
	"integration-project-ehb/controlroom/internal/mcp_server"
	"integration-project-ehb/controlroom/internal/statuscheck"
	"integration-project-ehb/controlroom/internal/summary"
	"integration-project-ehb/controlroom/internal/user"
	userack "integration-project-ehb/controlroom/internal/user_acknowledgment"
	"integration-project-ehb/controlroom/internal/watchdog"
	"integration-project-ehb/controlroom/pkg/logger"
)

// --------------------------------

// note(nasr): usefull consts for the main entry point
const weekly = 7 * 24 * time.Hour

const (
	initialBackoff = 1 * time.Second
	maxBackoff     = 60 * time.Second
	healthyAfter   = 10 * time.Second
)

const VERSION = "1.8.9"

var GlobalHttpClient = &http.Client{
	// note(nasr): if still not connected after 30 secodnds (should be enough for tls handshake, etc, etc) then fail w
	Timeout: 30 * time.Second,
}

// --------------------------------


func setup(ch *amqp.Channel) error {

	// refactor, sometimes we send teams messages from differnet hosts and we kind of don't know
	// if what we're testing is good or not. this is the solution i think
	{
		var err error
		config.Hostname, err = os.Hostname()
		if err != nil {
			config.Hostname = "localhost"
		}
	}

	logger.Log(logger.NewMessage(logger.DEBUG, logger.CONTROLROOM, "controlroom: declaring RabbitMQ exchanges and queues"))

	for _, def := range config.ConsumerDefinitions {

		if !def.Passive {

			if err := ch.ExchangeDeclare(def.Exchange.Name, def.Exchange.Kind, def.Exchange.Durable, false, false, false, nil); err != nil {
				return fmt.Errorf("exchange %s: %w", def.Exchange.Name, err)
			}

			logger.Log(logger.NewMessage(logger.DEBUG, logger.CONTROLROOM, fmt.Sprintf("controlroom: declared exchange %s (%s)", def.Exchange.Name, def.Exchange.Kind)))

		} else {
			//- skip the declaration of the exchange if the responsibility isn't ours
			continue
		}
	}

	if err := ch.ExchangeDeclare("controlroom.dlx", "direct", true, false, false, false, nil); err != nil {
		logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("controlroom: failed to declare dlx exchange: %v", err)))
		return fmt.Errorf("dlx: %w", err)
	}

	//- /////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
	//- note(steven): setup news producer
	if err := ch.ExchangeDeclare("news.topic", "topic", true, false, false, false, nil); err != nil {
		return fmt.Errorf("news.topic: %w", err)
	}

	if err := ch.ExchangeDeclare("ai.events", "topic", true, false, false, false, nil); err != nil {
		return fmt.Errorf("ai.events: %w", err)
	}

	if _, err := ch.QueueDeclare("mailing.news.warning", true, false, false, false, nil); err != nil {
		logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("controlroom: failed to declare queue mailing.news.warning: %v", err)))
	}

	if err := ch.QueueBind("mailing.news.warning", "news.warning", "news.topic", false, nil); err != nil {
		logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("controlroom: failed to bind queue mailing.news.warning: %v", err)))
	}

	//- /////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

	for _, def := range config.ConsumerDefinitions {
		if def.Passive {
			continue
		}

		if _, err := ch.QueueDeclare(def.Queue.Name, def.Queue.Durable, false, false, false, nil); err != nil {
			return fmt.Errorf("queue %s: %w", def.Queue.Name, err)
		}
		logger.Log(logger.NewMessage(logger.DEBUG, logger.CONTROLROOM, fmt.Sprintf("controlroom: declared queue %s", def.Queue.Name)))

		err := cr_rabbitmq.SetupDLQ(ch, def.DLQName)
		if err != nil {

			logger.Log(logger.NewMessage(logger.WARN, logger.CONTROLROOM, fmt.Sprintf("controlroom: failed to declare DLQ %s: %v", def.DLQName, err)))
		} else {

			logger.Log(logger.NewMessage(logger.DEBUG, logger.CONTROLROOM, fmt.Sprintf("controlroom: declared DLQ %s", def.DLQName)))

		}

		if err := ch.QueueBind(def.Queue.Name, def.Binding.Key, def.Exchange.Name, false, nil); err != nil {
			return fmt.Errorf("bind %s: %w", def.Queue.Name, err)
		}
		logger.Log(logger.NewMessage(logger.DEBUG, logger.CONTROLROOM, fmt.Sprintf("controlroom: bound %s -> %s (key: %s)", def.Queue.Name, def.Exchange.Name, def.Binding.Key)))

		dlqRoutingKey := def.Queue.Name + ".failed"
		if err := ch.QueueBind(def.DLQName, dlqRoutingKey, "controlroom.dlx", false, nil); err != nil {
			return fmt.Errorf("bind dlq %s: %w", def.DLQName, err)
		}
		logger.Log(logger.NewMessage(logger.DEBUG, logger.CONTROLROOM, fmt.Sprintf("controlroom: bound DLQ %s -> controlroom.dlx (key: %s)", def.DLQName, dlqRoutingKey)))
	}

	logger.Log(logger.NewMessage(logger.INFO, logger.CONTROLROOM, "controlroom: RabbitMQ topology setup complete"))
	return nil
}

func startSession(ctx context.Context, client *elasticsearch.Client) error {
	logger.Log(logger.NewMessage(logger.INFO, logger.CONTROLROOM, "controlroom: dialing RabbitMQ"))
	conn, err := amqp.Dial(os.Getenv("RABBITMQ_URL"))
	if err != nil {
		logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("controlroom: failed to dial RabbitMQ: %v", err)))
		return fmt.Errorf("dial: %w", err)
	}
	defer conn.Close()
	logger.Log(logger.NewMessage(logger.INFO, logger.CONTROLROOM, "controlroom: connected to RabbitMQ"))

	closeCh := conn.NotifyClose(make(chan *amqp.Error, 1))

	setupCh, err := conn.Channel()

	if err != nil {
		logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("controlroom: failed to open setup channel: %v", err)))
		return fmt.Errorf("setup channel: %w", err)
	}
	if err := setup(setupCh); err != nil {
		logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("controlroom: RabbitMQ topology setup failed: %v", err)))
		setupCh.Close()
		return err
	}
	setupCh.Close()

	dlqCh, err := conn.Channel()
	if err != nil {
		logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("controlroom: failed to open DLQ channel: %v", err)))
		return fmt.Errorf("dlq channel: %w", err)
	}
	defer dlqCh.Close()

	for _, def := range config.ConsumerDefinitions {
		ch, err := conn.Channel()
		if err != nil {
			logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("controlroom: failed to open channel for %s: %v", def.Queue.Name, err)))
			return fmt.Errorf("channel for %s: %w", def.Queue.Name, err)
		}
		// defer ch.Close()

		msgs, err := ch.Consume(def.Queue.Name, fmt.Sprintf("controlroom-%d", os.Getpid()), false, false, false, false, nil)
		if err != nil {
			logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("controlroom: failed to start consumer for %s: %v", def.Queue.Name, err)))
			return fmt.Errorf("setup %s: %w", def.Queue.Name, err)
		}

		//		if err := ch.Qos(def.Qos, 0, false); err != nil {
		//			logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("failed to set QoS for %s: %v", def.Queue.Name, err)))
		//			return fmt.Errorf("qos %s: %w", def.Queue.Name, err)
		//		}

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
			wrapper := func(es *elasticsearch.Client, body []byte) error {
				if err := user.ProcessUser(es, body); err != nil {
					return err
				}
				if err := userack.ProcessCRMAck(es, body); err != nil {
					return err
				}
				return nil
			}
			go cr_rabbitmq.Consume(cfg, msgs, ctx, wrapper)
		case config.COMPANY:
			go cr_rabbitmq.Consume(cfg, msgs, ctx, company.ProcessCompany)
		case config.USER_ACK:
			go cr_rabbitmq.Consume(cfg, msgs, ctx, userack.ProcessControlroomAck)
		case config.CHECK_IN:
			go cr_rabbitmq.Consume(cfg, msgs, ctx, checkin.ProcessCheckin)
		}
		logger.Log(logger.NewMessage(logger.INFO, logger.CONTROLROOM, fmt.Sprintf("controlroom: %s consumer started (qos: %d)", def.Queue.Name, def.Qos)))
	}

	logger.Log(logger.NewMessage(logger.INFO, logger.CONTROLROOM, "controlroom: all consumers running, waiting for messages"))

	// TODO(nasr): fix the channel thing

	//- setup rabbitmq heartbeat
	//- publish a heartbeat and consume it
	//- we are sending it with the service name RMQ
	//- by doing this we also do a e2e test of the complete communication
	//- every second

	go func() {
		hbPubCh, err := conn.Channel()
		if err != nil {
			logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("failing to make a channel for the heartbaet publisher to rabbitmq: %v", err)))

		}
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				err := cr_rabbitmq.PublishHeartbeat(hbPubCh)
				if err != nil {
					logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("RabbitMQ publishing failed, is RabbitMQ alive? error: %v", err)))
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	//- watchdog stuff
	{
		wdch, err := conn.Channel()
		if err != nil {
			logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("controlroom: failed to open watchdog publish channel: %v", err)))
		} else {
			defer wdch.Close()
			watchdog.SetPubChannel(wdch)
			defer watchdog.SetPubChannel(nil)
		}

		if watchdog.TeamsWebhook == "" {
			logger.Log(logger.NewMessage(logger.WARN, logger.CONTROLROOM, "controlroom: TEAMS_WEBHOOK_URL is not configured"))
		} else {
			// note(nasr): fix nil pointer dereference when env variable is empty
			for _, svc := range watchdog.Services {
				watchdog.ServiceState[svc] = true
			}

			//- note(nasr): inlining of the previous process alert queues function
			go func() {
				for message := range watchdog.WatchdogQueue {
					watchdog.AlertTeams(message)
					time.Sleep(6 * time.Second)
				}

			}()

			//- deprecated now
			//- the content below is age restricted
			//- only read this if the user is older then 65
			//- because this code is handwritten :)
			//- go watchdog.ProcessAlertQueue()

			//-
			go func() {
				ticker := time.NewTicker(5 * time.Second)
				defer ticker.Stop()
				for {
					select {
					case <-ticker.C:
						watchdog.RunWatchdog(client, ctx, wdch)
					case <-ctx.Done():
						return
					}
				}
			}()
		}
	}

	// - summary initialization
	{

		summaryCh, err := conn.Channel()
		if err != nil {
			logger.Log(logger.NewMessage(logger.WARN, logger.CONTROLROOM, fmt.Sprintf("controlroom: failed to initialize the summary channel: %v", err)))
		}

		go func() {
			ticker := time.NewTicker(weekly)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					err := summary.Generate(ctx, client, summaryCh)
					if err != nil {
						logger.Log(logger.NewMessage(logger.WARN, logger.CONTROLROOM, fmt.Sprintf("controlroom: failed to gather Kubernetes resources: %v", err)))
					}
				case <-ctx.Done():
					return

				}
			}
		}()
	}

	select {
	case reason := <-closeCh:
		if reason == nil {
			logger.Log(logger.NewMessage(logger.WARN, logger.CONTROLROOM, "controlroom: RabbitMQ connection closed unexpectedly"))
			return fmt.Errorf("connection closed")
		}
		logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("controlroom: RabbitMQ connection lost: %v", reason)))
		return fmt.Errorf("connection lost: %w", reason)
	case <-ctx.Done():
		logger.Log(logger.NewMessage(logger.INFO, logger.CONTROLROOM, "controlroom: context cancelled, closing RabbitMQ session"))
		return ctx.Err()
	}
}

func main() {

	log.Printf("[VERSION] %s\n", VERSION)

	if err := logger.Init(&config.ElasticConfig, "controlroom-logs", os.Stdout, 4); err != nil {
		fmt.Fprintf(os.Stderr, "logger init: %v\n", err)
		os.Exit(4)
	}
	defer logger.Shutdown()

	esClient, err := elasticsearch.NewClient(config.ElasticConfig)
	if err != nil {
		logger.Log(logger.NewMessage(logger.PANIC, logger.CONTROLROOM, fmt.Sprintf("controlroom: failed to create Elasticsearch client: %v", err)))
	}

	res, err := esClient.Info()
	if err != nil {
		logger.Log(logger.NewMessage(logger.PANIC, logger.CONTROLROOM, fmt.Sprintf("controlroom: failed to connect to Elasticsearch: %v", err)))
	}
	res.Body.Close()

	logger.Log(logger.NewMessage(logger.INFO, logger.CONTROLROOM, "controlroom: connected to Elasticsearch"))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	//- signal channel to handle proper exiting the software without stopping stuff in the middle
	//- keep the heartbeat publish loop separate so it can fail independently
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sc
		logger.Log(logger.NewMessage(logger.INFO, logger.CONTROLROOM, "controlroom: shutdown signal received, draining queues..."))
		cancel()
	}()

	//- mcp stuff
	{
		// NOTE(nasr): runs alongside the RabbitMQ session loop so mcp-master can initialized mcp server
		go func() {
			err := mcp.SetupMCP(esClient)
			if err != nil {
				logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("controlroom: MCP server exited: %v", err)))
			}
		}()
	}

	//- note(nasr): outside of the startSession function because this isn't dependent on rabbitmq
	//- gathering k8 resources. imrpovement over statuschecks. provide more accurate information
	//- because the services are running containerized
	//- if this doesn't work it should fail without breaking the rest
	{
		go func() {
			ticker := time.NewTicker(5 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					err := k8retriever.ProcessK8sData(esClient)
					if err != nil {
						logger.Log(logger.NewMessage(logger.WARN, logger.CONTROLROOM, fmt.Sprintf("controlroom: failed to gather Kubernetes resources: %v", err)))
					}
				case <-ctx.Done():
					return

				}
			}
		}()
	}

	//- not dependant on rabbitmq
	// note(nasr): toggle for debugging
	if true {
		// dynamic dashboards go go go
		{
			go func() {
				if config.KibanaConfig.DashboardUser == "" {
					logger.Log(logger.NewMessage(logger.WARN, logger.CONTROLROOM, "dashboard sync: DASHBOARD_USER is not set; dashboard auth will be disabled"))
				}

				logger.Log(logger.NewMessage(logger.INFO, logger.CONTROLROOM, "dashboard sync: starting dashboard synchronization loop"))

				ticker := time.NewTicker(5 * time.Second)

				for range ticker.C {
					sync.SyncLogsDashboard(esClient)
					sync.SyncHeartbeatDashboard(esClient)
				}
			}()
		}
	}


	// TODO(nasr & lars): should this communication happen over rabbitmq?
	// jarvis metrics
	{
		go jarvis_metrics.ProcessJarvisMetrics(ctx, esClient, GlobalHttpClient)
	}

	backoff := initialBackoff

	for {
		start := time.Now()
		err := startSession(ctx, esClient)

		if errors.Is(err, context.Canceled) {
			return
		}

		if time.Since(start) > healthyAfter {
			backoff = initialBackoff
		}

		logger.Log(logger.NewMessage(logger.WARN, logger.CONTROLROOM, fmt.Sprintf("controlroom: RabbitMQ session ended, redialing in %s: %v", backoff, err)))

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
