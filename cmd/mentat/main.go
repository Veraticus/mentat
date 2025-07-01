// Package main provides the entry point for the Mentat personal assistant bot.
package main

import (
	"context"
	_ "embed"
	"log"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Veraticus/mentat/internal/claude"
	"github.com/Veraticus/mentat/internal/config"
	"github.com/Veraticus/mentat/internal/queue"
	signalpkg "github.com/Veraticus/mentat/internal/signal"
)

//go:embed system-prompt.md
var embeddedSystemPrompt string

// Hardcoded configuration for MVP.
const (
	// Signal configuration.
	signalSocketPath    = "/run/signal-cli/socket"
	phoneNumberFilePath = "/etc/signal-bot/phone-number"

	// Claude configuration.
	claudeCommand = "/home/joshsymonds/.npm-global/bin/claude"
	mcpConfigPath = "" // Empty means no MCP config
	claudeTimeout = 120 * time.Second

	// Worker pool configuration.
	initialWorkers = 2
	minWorkers     = 1
	maxWorkers     = 5

	// Rate limiting.
	rateLimitCapacity = 10
	rateLimitRefill   = 1
	rateLimitPeriod   = time.Minute
)

func main() {
	os.Exit(runMain())
}

func runMain() int {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("Shutting down gracefully...")
		cancel()
	}()

	if err := run(ctx); err != nil {
		log.Printf("Error: %v", err)
		return 1
	}
	return 0
}

func run(ctx context.Context) error {
	log.Println("Mentat starting...")

	// Initialize all components
	components, err := initializeComponents(ctx)
	if err != nil {
		return err
	}

	// Start all components
	if err := startComponents(ctx, components); err != nil {
		return err
	}

	log.Println("Mentat started successfully. Listening for messages.")

	// Wait for context cancellation
	<-ctx.Done()

	// Graceful shutdown with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	return shutdown(shutdownCtx, components)
}

// components holds all initialized components.
type components struct {
	signalHandler *signalpkg.Handler
	workerPool    *queue.DynamicWorkerPool
	queueManager  *queue.Manager
	wg            sync.WaitGroup
}

// messageEnqueuerAdapter adapts the queue.Manager to implement signal.MessageEnqueuer.
type messageEnqueuerAdapter struct {
	manager *queue.Manager
}

func (a *messageEnqueuerAdapter) Enqueue(msg signalpkg.IncomingMessage) error {
	// Convert IncomingMessage to queue.Message
	queueMsg := queue.NewMessage(
		generateMessageID(msg.Timestamp),
		msg.FromNumber, // Use phone number as conversation ID
		msg.From,       // Display name
		msg.FromNumber, // Phone number
		msg.Text,
	)

	return a.manager.Submit(queueMsg)
}

func generateMessageID(timestamp time.Time) string {
	return timestamp.Format("20060102150405.999999999")
}

// readPhoneNumber reads the bot's phone number from the config file.
func readPhoneNumber() (string, error) {
	data, err := os.ReadFile(phoneNumberFilePath)
	if err != nil {
		// If we can't read the system file, check for a local override
		// This is useful for development/testing
		if os.IsPermission(err) {
			log.Printf("Permission denied reading %s, checking for local override", phoneNumberFilePath)

			// Try reading from current directory
			localData, localErr := os.ReadFile("phone-number")
			if localErr == nil {
				phoneNumber := strings.TrimSpace(string(localData))
				if phoneNumber != "" {
					log.Printf("Using phone number from local file")
					return phoneNumber, nil
				}
			}

			// Try environment variable as last resort
			if envPhone := os.Getenv("SIGNAL_PHONE_NUMBER"); envPhone != "" {
				log.Printf("Using phone number from environment variable")
				return envPhone, nil
			}
		}
		return "", err
	}

	phoneNumber := strings.TrimSpace(string(data))
	if phoneNumber == "" {
		return "", os.ErrNotExist
	}

	return phoneNumber, nil
}

func initializeComponents(ctx context.Context) (*components, error) {
	// 0. Read bot phone number from config
	botPhoneNumber, err := readPhoneNumber()
	if err != nil {
		if os.IsPermission(err) {
			log.Printf("Permission denied reading phone number. Try one of:")
			log.Printf("  1. Run as sudo or signal-cli user")
			log.Printf("  2. Create a 'phone-number' file in current directory")
			log.Printf("  3. Set SIGNAL_PHONE_NUMBER environment variable")
			return nil, err
		}
		return nil, err
	}
	log.Printf("Using bot phone number: %s", botPhoneNumber)

	// 1. Initialize Signal transport and client
	transport, err := signalpkg.NewUnixSocketTransport(signalSocketPath)
	if err != nil {
		return nil, err
	}

	signalClient := signalpkg.NewClient(transport)
	messenger := signalpkg.NewMessenger(signalClient, botPhoneNumber)

	// 2. Validate embedded system prompt
	if validateErr := config.ValidateSystemPrompt(embeddedSystemPrompt); validateErr != nil {
		return nil, validateErr
	}
	log.Printf("Using embedded system prompt (%d characters)", len(embeddedSystemPrompt))

	// 3. Initialize Claude client
	claudeConfig := claude.Config{
		Command:       claudeCommand,
		MCPConfigPath: mcpConfigPath,
		SystemPrompt:  embeddedSystemPrompt,
		Timeout:       claudeTimeout,
	}

	claudeClient, err := claude.NewClient(claudeConfig)
	if err != nil {
		return nil, err
	}

	// 4. Initialize support services
	messageQueue := queue.NewMessageQueue()
	rateLimiter := queue.NewRateLimiter(rateLimitCapacity, rateLimitRefill, rateLimitPeriod)

	// 5. Initialize queue manager
	queueManager := queue.NewManager(ctx)

	// 6. Initialize worker pool
	poolConfig := queue.PoolConfig{
		InitialSize:  initialWorkers,
		MinSize:      minWorkers,
		MaxSize:      maxWorkers,
		LLM:          claudeClient,
		Messenger:    messenger,
		QueueManager: queueManager,
		MessageQueue: messageQueue,
		RateLimiter:  rateLimiter,
		PanicHandler: nil, // Use default panic handler
	}

	workerPool, err := queue.NewDynamicWorkerPool(poolConfig)
	if err != nil {
		return nil, err
	}

	// 7. Initialize Signal handler
	enqueuer := &messageEnqueuerAdapter{manager: queueManager}
	signalHandler, err := signalpkg.NewHandler(messenger, enqueuer)
	if err != nil {
		return nil, err
	}

	return &components{
		signalHandler: signalHandler,
		workerPool:    workerPool,
		queueManager:  queueManager,
	}, nil
}

func startComponents(ctx context.Context, c *components) error {
	// Start queue manager first (before workers start requesting messages)
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		log.Println("Starting queue manager...")
		c.queueManager.Start()
		log.Println("Queue manager stopped")
	}()

	// Start worker pool
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		log.Println("Starting worker pool...")
		if err := c.workerPool.Start(ctx); err != nil {
			log.Printf("Worker pool error: %v", err)
		}
	}()

	// Start signal handler (subscribes to messages)
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		log.Println("Starting Signal handler...")
		if err := c.signalHandler.Start(ctx); err != nil {
			log.Printf("Signal handler error: %v", err)
		}
	}()

	// Components are started asynchronously and will initialize in the background

	return nil
}

func shutdown(ctx context.Context, c *components) error {
	log.Println("Shutting down components...")

	// Wait for all components to finish
	done := make(chan struct{})
	go func() {
		c.wg.Wait()
		close(done)
	}()

	// Wait for graceful shutdown or timeout
	select {
	case <-done:
		log.Println("All components stopped successfully")
	case <-ctx.Done():
		log.Println("Shutdown timeout exceeded")
	}

	log.Println("Shutdown complete")
	return nil
}
