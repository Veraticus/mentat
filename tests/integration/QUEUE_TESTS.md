# Queue Integration Tests

This file documents the queue integration tests implemented in `queue_integration_test.go`.

## Test Coverage

### 1. Queue Behavioral Guarantees (`TestQueueBehavioralGuarantees`)

Table-driven tests verifying core queue behaviors:

- **message_ordering_within_conversation**: Ensures FIFO ordering is maintained for messages from the same user
- **concurrent_conversations_isolation**: Verifies multiple conversations can be processed in parallel without interference
- **rate_limiting_enforcement**: Tests burst limiting and rate limiting behavior
- **queue_fills_up**: Validates queue behavior when many messages are enqueued with slow processing
- **error_handling**: Ensures proper error handling when LLM queries fail
- **graceful_shutdown**: Verifies clean shutdown with no message loss

### 2. Queue Load Scenarios (`TestQueueLoadScenarios`)

Performance tests under various load conditions:

- **high_throughput_single_user**: 50 messages from one user with 5 workers
- **many_concurrent_users**: 20 users sending 5 messages each with 10 workers
- **burst_traffic**: 100 messages sent at once to test burst handling

### 3. Queue State Transitions (`TestQueueStateTransitions`)

Verifies proper state machine transitions through the message lifecycle.

### 4. Queue Metrics Accuracy (`TestQueueMetricsAccuracy`)

Ensures queue statistics accurately reflect the actual queue state.

## Key Test Patterns

1. **Table-Driven Tests**: All major test suites use table-driven patterns for maintainability
2. **Test Harness Integration**: Uses the existing `TestHarness` for realistic testing
3. **Configurable Scenarios**: Each test configures the harness appropriately for its scenario
4. **Comprehensive Verification**: Tests verify both expected outcomes and system invariants

## Running the Tests

```bash
# Run all queue integration tests
go test -tags integration -run TestQueue ./tests/integration

# Run specific test scenario
go test -tags integration -run TestQueueBehavioralGuarantees/rate_limiting_enforcement ./tests/integration
```

## Test Results

All tests pass with:
- ✓ Message ordering preserved within conversations
- ✓ Concurrent conversations processed independently  
- ✓ Rate limiting enforced correctly
- ✓ Error handling works as expected
- ✓ Graceful shutdown preserves messages
- ✓ Load scenarios complete successfully