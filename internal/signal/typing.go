package signal

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

const (
	// DefaultTypingIndicatorInterval is the default interval at which typing indicators are refreshed.
	DefaultTypingIndicatorInterval = 10 * time.Second
)

// TypingIndicatorManager manages typing indicators for multiple recipients.
type TypingIndicatorManager interface {
	// Start begins sending typing indicators for a recipient.
	Start(ctx context.Context, recipient string) error

	// Stop stops sending typing indicators for a recipient.
	Stop(recipient string)

	// StopAll stops all active typing indicators.
	StopAll()
}

// typingIndicator represents an active typing indicator.
type typingIndicator struct {
	cancel context.CancelFunc
}

// typingManager implements TypingIndicatorManager.
type typingManager struct {
	messenger  Messenger
	indicators map[string]*typingIndicator
	mu         sync.RWMutex
	interval   time.Duration
}

// NewTypingIndicatorManager creates a new typing indicator manager with default interval.
func NewTypingIndicatorManager(messenger Messenger) TypingIndicatorManager {
	return NewTypingIndicatorManagerWithInterval(messenger, DefaultTypingIndicatorInterval)
}

// NewTypingIndicatorManagerWithInterval creates a new typing indicator manager with custom interval.
func NewTypingIndicatorManagerWithInterval(messenger Messenger, interval time.Duration) TypingIndicatorManager {
	return &typingManager{
		messenger:  messenger,
		indicators: make(map[string]*typingIndicator),
		interval:   interval,
	}
}

// Start begins sending typing indicators for a recipient.
func (m *typingManager) Start(ctx context.Context, recipient string) error {
	if recipient == "" {
		return fmt.Errorf("recipient cannot be empty")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if already active
	if _, exists := m.indicators[recipient]; exists {
		return fmt.Errorf("typing indicator already active for recipient %s", recipient)
	}

	// Create cancelable context
	indicatorCtx, cancel := context.WithCancel(ctx)

	// Store indicator
	m.indicators[recipient] = &typingIndicator{
		cancel: cancel,
	}

	// Start goroutine
	go m.runIndicator(indicatorCtx, recipient)

	return nil
}

// Stop stops sending typing indicators for a recipient.
func (m *typingManager) Stop(recipient string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if indicator, exists := m.indicators[recipient]; exists {
		indicator.cancel()
		delete(m.indicators, recipient)
	}
}

// StopAll stops all active typing indicators.
func (m *typingManager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for recipient, indicator := range m.indicators {
		indicator.cancel()
		delete(m.indicators, recipient)
	}
}

// runIndicator sends typing indicators periodically.
func (m *typingManager) runIndicator(ctx context.Context, recipient string) {
	// Send initial typing indicator
	if err := m.messenger.SendTypingIndicator(ctx, recipient); err != nil {
		logger := slog.Default()
		logger.ErrorContext(ctx, "Failed to send initial typing indicator",
			slog.String("recipient", recipient),
			slog.Any("error", err))
		// Continue anyway - don't fail the whole operation
	}

	// Create ticker for periodic updates
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// Context canceled, just return
			// The Stop() method already handles cleanup
			return

		case <-ticker.C:
			// Send periodic typing indicator
			if err := m.messenger.SendTypingIndicator(ctx, recipient); err != nil {
				logger := slog.Default()
				logger.ErrorContext(ctx, "Failed to send typing indicator",
					slog.String("recipient", recipient),
					slog.Any("error", err))
				// Don't stop on error - the recipient might come back online
			}
		}
	}
}
