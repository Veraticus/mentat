# Mentat Integration Test Harness

This directory contains a comprehensive integration test harness for the Mentat personal assistant bot. The harness provides a complete testing environment for verifying the full message flow from Signal reception through queue processing to Claude LLM responses.

## Overview

The test harness (`TestHarness`) provides a unified testing framework that:
- Works seamlessly for both simple and complex test scenarios
- Includes comprehensive message lifecycle tracking
- Monitors resource usage and performance metrics
- Detects common issues like goroutine leaks and queue stalls
- Supports deterministic testing with full control over timing and responses

## Quick Start

### Running Integration Tests

```bash
# Run all integration tests
make test

# Run only integration tests
make test-integration

# Run specific test
go test -tags integration -v ./tests/integration -run TestSimpleConversation

# Run with race detection
go test -tags integration -race ./tests/integration
```

### Basic Usage

```go
func TestMyFeature(t *testing.T) {
    RunScenario(t, "my_test", DefaultConfig(), func(t *testing.T, h *TestHarness) {
        // Configure LLM response
        h.SetLLMResponse("Hello from Claude!", nil)
        
        // Send message
        err := h.SendMessage("+1234567890", "Hello bot")
        if err != nil {
            t.Fatal(err)
        }
        
        // Wait for response
        response, err := h.WaitForMessage(2 * time.Second)
        if err != nil {
            t.Fatal(err)
        }
        
        // Verify response
        if response.Text != "Hello from Claude!" {
            t.Errorf("Got %q, want %q", response.Text, "Hello from Claude!")
        }
    })
}
```

### Enhanced Usage

```go
func TestAdvancedFeature(t *testing.T) {
    RunEnhancedScenario(t, "advanced", DefaultConfig(), func(t *testing.T, h *EnhancedTestHarness) {
        // Track complete message lifecycle
        err := h.SendMessage("+1234567890", "Track this")
        if err != nil {
            t.Fatal(err)
        }
        
        // Wait for specific message completion
        msgID := fmt.Sprintf("msg-%d", time.Now().UnixNano())
        err = h.WaitForMessageCompletion(msgID, 5*time.Second)
        if err != nil {
            t.Fatal(err)
        }
        
        // Get detailed statistics
        stats := h.GetMessageStats()
        t.Logf("Completed: %d, Failed: %d", stats.Completed, stats.Failed)
        
        // Check queue metrics
        metrics := h.GetQueueMetrics()
        t.Logf("Max queue depth: %d", metrics.MaxDepth)
    })
}
```

## Features

### Basic Test Harness

- **Component Initialization**: Sets up Signal handler, message queue, worker pool with mocked external dependencies
- **Message Flow Testing**: Send messages and verify responses
- **LLM Response Mocking**: Configure expected responses for different scenarios
- **Queue State Verification**: Check message counts and processing state
- **Error Collection**: Track unexpected errors during test execution
- **Graceful Shutdown**: Proper cleanup with timeout protection

### Enhanced Test Harness

All basic features plus:

#### 1. Message Lifecycle Tracking
- Complete state transition history (Queued → Processing → Completed/Failed)
- Processing time measurement
- Retry attempt tracking
- Error history with context
- Response correlation

#### 2. Performance Metrics
- Queue depth monitoring over time
- Message throughput calculation
- Processing latency tracking
- Worker utilization metrics
- Concurrent load testing support

#### 3. Resource Monitoring
- Goroutine leak detection
- Memory usage tracking
- Garbage collection statistics
- Resource growth analysis

#### 4. Advanced Testing Capabilities
- **Error Injection**: Simulate failures at specific points
- **Timing Control**: Deterministic time advancement for testing timeouts
- **Queue Backpressure**: Test overflow and rate limiting
- **Chaos Testing**: Random failure injection
- **Load Testing**: Concurrent user simulation

#### 5. Observability
- Queue alerts (high depth, stalled processing)
- Performance bottleneck detection
- Detailed state change tracking
- Comprehensive test reporting

## Configuration

### HarnessConfig Options

```go
config := HarnessConfig{
    BotPhoneNumber:  "+1234567890",  // Bot's phone number
    WorkerCount:     2,              // Number of worker threads
    QueueDepth:      10,             // Maximum queue size per conversation
    RateLimitTokens: 10,             // Rate limit bucket size
    RateLimitRefill: time.Second,    // Rate limit refill period
    DefaultTimeout:  5 * time.Second, // Default operation timeout
    EnableLogging:   false,          // Enable debug logging
}
```

## Test Scenarios

### 1. Simple Conversation
Tests basic request/response flow:
```go
RunScenario(t, "simple", DefaultConfig(), func(t *testing.T, h *TestHarness) {
    h.SetLLMResponse("Response text", nil)
    h.SendMessage(phone, "Request text")
    response := h.WaitForMessage(timeout)
    // Verify response
})
```

### 2. Error Handling
Tests various failure modes:
```go
// LLM errors
h.SetLLMResponse("", fmt.Errorf("LLM unavailable"))

// Empty responses
h.SetLLMResponse("", nil)

// Timeouts (with enhanced harness)
h.InjectDelay("pattern", 10*time.Second)
```

### 3. Concurrent Load
Tests system under concurrent load:
```go
for i := 0; i < 100; i++ {
    go h.SendMessage(fmt.Sprintf("+123456%04d", i), "Test")
}
// Verify completion rate and performance
```

### 4. Rate Limiting
Tests rate limiting behavior:
```go
config.RateLimitTokens = 2
config.RateLimitRefill = 5 * time.Second
// Send burst of messages
// Verify rate limiting applied
```

### 5. Message Ordering
Verifies FIFO within conversations:
```go
// Send multiple messages rapidly
for i := 0; i < 10; i++ {
    h.SendMessage(phone, fmt.Sprintf("Message %d", i))
}
// Verify responses maintain order
```

### 6. Resource Leak Detection
Ensures no goroutine or memory leaks:
```go
baseline := h.resourceMonitor.Sample()
// Run test workload
final := h.resourceMonitor.Sample()
// Verify no significant growth
```

## Mock Capabilities

### MockLLM
- Configure responses by session ID or prompt pattern
- Simulate errors and timeouts
- Track all LLM calls for verification
- Support for multi-turn conversations

### MockMessenger
- Simulate incoming Signal messages
- Track sent messages with full details
- Support multiple concurrent subscribers
- Typing indicator simulation

## Best Practices

1. **Use Enhanced Harness for Complex Tests**: The enhanced harness provides much better debugging and verification capabilities

2. **Configure Responses Before Sending**: Always set up expected LLM responses before sending messages

3. **Wait for Completion**: Use appropriate timeouts when waiting for responses

4. **Verify Complete State**: Check both responses and queue state for comprehensive verification

5. **Clean Up Resources**: The harness handles cleanup automatically, but always use defer for teardown

6. **Use Subtests**: Leverage Go's t.Run() for better test organization

7. **Check for Leaks**: In long-running tests, verify no resource leaks

## Debugging Failed Tests

### Common Issues

1. **"Response text = X, want Y"**
   - Check LLM response configuration
   - Verify session ID matching (signal-<phone>)

2. **"Timeout waiting for message"**
   - Increase timeout duration
   - Check if workers are processing
   - Verify queue isn't stalled

3. **"Completed messages = 0, want 1"**
   - Using basic harness without enhanced tracking
   - Message failed to process completely
   - Check error logs

4. **Resource leaks detected**
   - Goroutines not cleaning up properly
   - Context cancellation not propagating
   - Workers stuck in processing

### Debug Helpers

```go
// Get detailed state changes
changes := h.GetStateChanges()
for _, change := range changes {
    t.Logf("%s: %s -> %s", change.Component, change.Type, change.Details)
}

// Check queue alerts
alerts := h.queueMonitor.GetAlerts()
for _, alert := range alerts {
    t.Logf("Alert: %s - %s", alert.Type, alert.Description)
}

// Examine message history
msg, _ := h.messageTracker.GetMessage(msgID)
for _, transition := range msg.StateHistory {
    t.Logf("%s -> %s: %s", transition.From, transition.To, transition.Reason)
}
```

## Architecture

The test harness mirrors the production architecture:

```
Signal Handler → Message Queue → Worker Pool → LLM (Claude)
       ↓                ↓             ↓            ↓
    [Mocked]      [Real+Tracked]  [Real]      [Mocked]
```

Key components:
- **Signal**: Fully mocked with incoming message simulation
- **Queue**: Real implementation with comprehensive tracking
- **Workers**: Real worker pool processing
- **LLM**: Mocked with configurable responses
- **Tracking**: Additional layer for observability

## Extending the Harness

### Adding New Tracking

```go
type MyCustomTracker struct {
    // Custom tracking fields
}

func (h *EnhancedTestHarness) TrackCustomMetric(metric string, value int) {
    // Add to your custom tracker
}
```

### Custom Assertions

```go
func (h *TestHarness) AssertMessageProcessedWithin(d time.Duration) {
    // Custom assertion logic
}
```

### New Mock Behaviors

```go
// Add to mock messenger
func (m *MockMessengerWithIncoming) SimulateNetworkError() {
    // Simulate network issues
}
```

## Performance Considerations

- Integration tests are slower than unit tests
- Use `t.Parallel()` where possible
- Consider test data size for load tests
- Monitor test execution time
- Use `-short` flag to skip long tests

## Contributing

When adding new integration tests:

1. Decide if basic or enhanced harness is needed
2. Follow existing patterns for consistency
3. Add appropriate documentation
4. Ensure tests are deterministic
5. Avoid external dependencies
6. Keep tests focused and readable

## Future Enhancements

Planned improvements:
- [ ] MCP server mocking and testing
- [ ] Scheduler/cron job testing
- [ ] Database state verification
- [ ] Network condition simulation
- [ ] Distributed testing support
- [ ] Performance regression detection
- [ ] Automated test generation
- [ ] Visual test reporting