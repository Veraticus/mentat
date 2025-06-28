# Mentat Implementation TODO

## Overview
This TODO breaks down the Mentat implementation into bite-sized, testable chunks. Each task includes what to implement, tests to write first, acceptance criteria, and Go idioms to follow.

**Key Principles:**
- Test-driven development (TDD) - write tests first
- Interface-driven design - accept interfaces, return structs
- No custom error types - use `fmt.Errorf` with context
- No `interface{}` - use `any` or concrete types
- Build our own simple state machine
- Delete old code when replacing

## Foundation Layer

### Core Interfaces
- [ ] Define all core interfaces
  - Test: Verify interfaces compile and are mockable
  - Location: `internal/*/interfaces.go`
  - Files to create:
    - `internal/signal/interfaces.go` (Messenger)
    - `internal/claude/interfaces.go` (LLM) 
    - `internal/conversation/interfaces.go` (SessionManager)
    - `internal/agent/interfaces.go` (ValidationStrategy, IntentEnhancer)
    - `internal/queue/interfaces.go` (MessageQueue, Worker, RateLimiter, StateMachine)
  - Acceptance: Can create mock implementations for each interface
  - Go idiom: Keep interfaces small and focused (Interface Segregation Principle)

- [ ] Create core types
  - Test: Verify types have proper zero values
  - Location: `internal/*/types.go`
  - Types: IncomingMessage, LLMResponse, QueuedMessage, MessageState, Priority
  - Acceptance: Types compile and have sensible defaults
  - Go idiom: Make zero values useful

- [ ] Set up basic project structure
  - Test: `go mod tidy` runs without errors
  - Create all directories from architecture
  - Initialize go.mod with proper module name
  - Create Makefile with build/test/lint targets
  - Acceptance: `make build` creates empty binary

### Testing Infrastructure
- [ ] Create mock implementations for all interfaces
  - Test: Mocks implement their interfaces correctly
  - Location: `internal/testing/mocks.go`
  - Include: MockLLM, MockMessenger, MockQueue, MockSessionManager
  - Acceptance: Mocks are usable in unit tests
  - Go idiom: Embed mutex in mocks for thread safety

- [ ] Create test builders and helpers
  - Test: Builders create valid test data
  - Location: `internal/testing/builders.go`
  - Include: Message builders, response builders, scenario builders
  - Acceptance: Can build complex test scenarios fluently
  - Go idiom: Use functional options pattern for builders

- [ ] Implement ScriptedLLM for deterministic testing
  - Test: ScriptedLLM returns expected responses in order
  - Location: `internal/testing/scripted_llm.go`
  - Features: Pattern matching, delays, error injection
  - Acceptance: Can script multi-turn conversations
  - Go idiom: Use table-driven tests

### Signal JSON-RPC Client
- [ ] Implement JSON-RPC client
  - Test: Client handles connection errors gracefully
  - Location: `internal/signal/client.go`
  - Features: Connection pooling, timeout handling
  - Acceptance: Can make RPC calls with proper error handling
  - Go idiom: Return errors as last value

- [ ] Implement Signal Messenger interface
  - Test: Send/Subscribe work with mock RPC client
  - Location: `internal/signal/messenger.go`
  - Features: Send, SendTypingIndicator, Subscribe
  - Acceptance: Messages flow through subscription channel
  - Go idiom: Close channels from sender side only

### Signal Handler
- [ ] Implement Signal handler with queue integration
  - Test: Handler enqueues messages without blocking
  - Location: `internal/signal/handler.go`
  - Features: Subscribe loop, graceful shutdown
  - Acceptance: Ctrl+C cleanly shuts down
  - Go idiom: Use context for cancellation

- [ ] Add typing indicator management
  - Test: Typing indicators refresh every 10 seconds
  - Location: `internal/signal/typing.go`
  - Features: Start/stop, automatic refresh
  - Acceptance: Indicators maintain during long operations
  - Go idiom: Use time.Ticker for periodic tasks

### Queue State Machine
- [ ] Implement simple state machine
  - Test: Valid transitions succeed, invalid fail
  - Location: `internal/queue/state.go`
  - States: Queued, Processing, Validating, Completed, Failed, Retrying
  - Acceptance: State history is maintained
  - Go idiom: Use iota for enums

- [ ] Add state transition validation
  - Test: Only allowed transitions are permitted
  - Features: Transition rules, reason tracking
  - Acceptance: Clear error messages for invalid transitions
  - Go idiom: Make invalid states unrepresentable

### Message Queue Core
- [ ] Implement QueuedMessage with state tracking
  - Test: Messages track all state changes
  - Location: `internal/queue/message.go`
  - Features: State history, attempt counting, error tracking
  - Acceptance: Can query message history
  - Go idiom: Avoid mutations, prefer immutable updates

- [ ] Implement ConversationQueue
  - Test: Messages maintain order within conversation
  - Location: `internal/queue/conversation.go`
  - Features: FIFO ordering, depth limits
  - Acceptance: Overflow returns clear error
  - Go idiom: Protect invariants with mutex

### Queue Manager
- [ ] Implement QueueManager coordinating conversations
  - Test: Parallel conversations don't interfere
  - Location: `internal/queue/manager.go`
  - Features: Conversation isolation, statistics
  - Acceptance: Can handle 100 concurrent conversations
  - Go idiom: Use sync.Map for concurrent access

- [ ] Add GetNext logic for workers
  - Test: Workers get oldest unprocessed message
  - Features: Fair scheduling, conversation affinity
  - Acceptance: No starvation across conversations
  - Go idiom: Avoid holding locks during I/O

### Rate Limiting
- [ ] Implement token bucket rate limiter
  - Test: Burst allows N rapid requests then limits
  - Location: `internal/queue/limiter.go`
  - Features: Per-conversation limits, refill rate
  - Acceptance: Rate limit errors are clear
  - Go idiom: Use time.Since for duration calculations

- [ ] Integrate rate limiting with queue
  - Test: Rate-limited messages retry with backoff
  - Features: Automatic retry scheduling
  - Acceptance: Eventually processes all messages
  - Go idiom: Use exponential backoff

### Worker Pool
- [ ] Implement Worker interface and QueueWorker
  - Test: Worker processes messages in order
  - Location: `internal/queue/worker.go`
  - Features: Graceful shutdown, error handling
  - Acceptance: No message loss on shutdown
  - Go idiom: Use sync.WaitGroup for coordination

- [ ] Implement WorkerPool manager
  - Test: Pool scales workers up/down
  - Location: `internal/queue/pool.go`
  - Features: Dynamic sizing, health checks
  - Acceptance: Handles worker failures gracefully
  - Go idiom: Prefer channels over shared memory

### Queue Integration
- [ ] Wire queue components together
  - Test: End-to-end message flow works
  - Features: All components integrate cleanly
  - Acceptance: Message goes from enqueue to completion
  - Go idiom: Use dependency injection

- [ ] Add queue statistics and monitoring
  - Test: Stats accurately reflect queue state
  - Location: `internal/queue/stats.go`
  - Features: Queue depth, processing time, success rate
  - Acceptance: Can expose metrics endpoint
  - Go idiom: Use atomic operations for counters

### Basic Claude Client
- [ ] Implement Claude CLI wrapper
  - Test: Handles CLI errors and timeouts
  - Location: `internal/claude/client.go`
  - Features: Command execution, timeout handling
  - Acceptance: Returns clear errors on failure
  - Go idiom: Use exec.CommandContext for timeouts

- [ ] Add response parsing
  - Test: Extracts message from various outputs
  - Features: Handle multiline responses, errors
  - Acceptance: Never returns empty responses
  - Go idiom: Use strings.Builder for efficiency

### Claude Configuration
- [ ] Implement MCP config generation
  - Test: Generates valid JSON config
  - Location: `internal/config/mcp.go`
  - Features: HTTP transport for all servers
  - Acceptance: Claude accepts generated config
  - Go idiom: Use struct tags for JSON

- [ ] Add system prompt loading
  - Test: Loads and validates prompt
  - Location: `internal/config/prompt.go`
  - Features: File loading, validation
  - Acceptance: Missing prompt fails fast
  - Go idiom: Fail fast on configuration errors

### Integration Testing Framework
- [ ] Create integration test harness
  - Test: Can run full conversation scenarios
  - Location: `tests/integration/harness_test.go`
  - Features: Setup/teardown, state verification
  - Acceptance: Tests are deterministic
  - Go idiom: Use subtests for scenarios

- [ ] Write basic queue integration tests
  - Test: Queue handles load correctly
  - Scenarios: Overflow, rate limiting, ordering
  - Acceptance: All queue guarantees verified
  - Go idiom: Table-driven test cases

## Intelligence Layer

### Agent Handler Structure
- [ ] Implement AgentHandler with dependency injection
  - Test: Constructor validates required dependencies
  - Location: `internal/agent/handler.go`
  - Features: Option pattern, nil checking
  - Acceptance: Clear errors for missing deps
  - Go idiom: Functional options for construction

- [ ] Add basic Process method
  - Test: Happy path processes successfully
  - Features: Session management, error handling
  - Acceptance: Returns clear errors
  - Go idiom: Handle errors explicitly

### Multi-Agent Validation Framework
- [ ] Define ValidationStrategy interface implementations
  - Test: Each strategy has different behavior
  - Location: `internal/agent/validator.go`
  - Strategies: MultiAgent, Simple, Noop
  - Acceptance: Strategies are pluggable
  - Go idiom: Strategy pattern with interfaces

- [ ] Implement validation result parsing
  - Test: Parses all status types correctly
  - Features: Confidence scores, issue extraction
  - Acceptance: Unknown responses default safely
  - Go idiom: Use switch for exhaustive matching

### Multi-Agent Validation Logic
- [ ] Implement thoroughness checking
  - Test: Detects incomplete tool usage
  - Features: INCOMPLETE_SEARCH status
  - Acceptance: Catches missed memory checks
  - Go idiom: Be explicit about expectations

- [ ] Add retry logic for incomplete searches  
  - Test: Retries lead to more complete results
  - Features: Guided retry prompts
  - Acceptance: Maximum 2 retry attempts
  - Go idiom: Fail gracefully after retries

### Recovery Generation
- [ ] Implement natural recovery messages
  - Test: Recovery explains issues clearly
  - Location: `internal/agent/recovery.go`
  - Features: Context-aware explanations
  - Acceptance: Never exposes internal errors
  - Go idiom: User-friendly error messages

- [ ] Add partial success handling
  - Test: Partial successes explained properly
  - Features: What worked, what didn't
  - Acceptance: Users understand state
  - Go idiom: Be honest about failures

### Intent Enhancement
- [ ] Implement SmartIntentEnhancer
  - Test: Enhances without being prescriptive
  - Location: `internal/agent/enhancer.go`
  - Features: Pattern matching, hint mapping
  - Acceptance: Hints guide but don't dictate
  - Go idiom: Keep hints data-driven

- [ ] Add intent detection logic
  - Test: Detects common request patterns
  - Patterns: Scheduling, finding people, memory
  - Acceptance: >90% accuracy on common intents
  - Go idiom: Prefer simple over clever

### Complex Request Detection
- [ ] Implement complexity analyzer
  - Test: Identifies multi-step requests
  - Location: `internal/agent/complex.go`
  - Features: Step counting, dependency detection
  - Acceptance: Catches compound requests
  - Go idiom: Make detection configurable

- [ ] Add gentle guidance system
  - Test: Guides without micromanaging
  - Features: Thoroughness hints
  - Acceptance: Claude remains autonomous
  - Go idiom: Trust but verify

### Session Management
- [ ] Implement ConversationManager
  - Test: Sessions expire after 5 minutes
  - Location: `internal/conversation/manager.go`
  - Features: Sliding window, thread safety
  - Acceptance: No race conditions
  - Go idiom: Use RWMutex for read-heavy loads

- [ ] Add session history tracking
  - Test: History maintains message order
  - Features: Message limit, context building
  - Acceptance: Old messages are pruned
  - Go idiom: Bounded data structures

### Session Cleanup
- [ ] Implement periodic cleanup
  - Test: Expired sessions are removed
  - Location: `internal/conversation/cleanup.go`
  - Features: Background goroutine, graceful stop
  - Acceptance: No memory leaks
  - Go idiom: Always stop goroutines cleanly

- [ ] Add session persistence interface
  - Test: Sessions can be saved/loaded
  - Features: JSON serialization
  - Acceptance: Restarts preserve context
  - Go idiom: Make persistence optional

### Full Agent Flow
- [ ] Wire agent components together
  - Test: Full flow from request to response
  - Features: Enhancement → Execution → Validation
  - Acceptance: Each step adds value
  - Go idiom: Compose behaviors

- [ ] Add comprehensive error handling
  - Test: All error paths return messages
  - Features: User-friendly errors
  - Acceptance: No error left unhandled
  - Go idiom: Errors are values

### Agent Testing Scenarios
- [ ] Build complex test scenarios
  - Test: Multi-turn conversations work
  - Location: `tests/scenarios/complex_requests.go`
  - Scenarios: Scheduling, partial failures
  - Acceptance: Scenarios are reusable
  - Go idiom: Data-driven test cases

- [ ] Add failure mode testing
  - Test: System handles all failure types
  - Location: `tests/scenarios/failure_modes.go`
  - Modes: Timeout, invalid response, tool errors
  - Acceptance: Graceful degradation
  - Go idiom: Test the unhappy path

### MCP Configuration Integration
- [ ] Create MCP config generator
  - Test: Config matches Claude's format
  - Location: `internal/config/generator.go`
  - Features: All 5 MCP servers configured
  - Acceptance: Claude accepts config
  - Go idiom: Generate, don't template

- [ ] Add config validation
  - Test: Invalid configs fail fast
  - Features: URL validation, server checks
  - Acceptance: Clear error messages
  - Go idiom: Validate at boundaries

### MCP Health Checking
- [ ] Implement MCP server health checks
  - Test: Detects when servers are down
  - Location: `internal/mcp/health.go`
  - Features: HTTP health endpoints
  - Acceptance: Quick detection (<5s)
  - Go idiom: Use context with timeout

- [ ] Add startup verification
  - Test: Won't start without MCP servers
  - Features: Retry logic, clear errors
  - Acceptance: Helpful error messages
  - Go idiom: Fail fast and loud

### Intelligence Layer Integration Testing
- [ ] Create end-to-end test scenarios
  - Test: Real Claude integration works
  - Location: `tests/integration/e2e_test.go`
  - Tag: `// +build integration`
  - Acceptance: Tests can run locally
  - Go idiom: Skip if Claude unavailable

- [ ] Add load testing
  - Test: System handles 50 concurrent users
  - Location: `tests/integration/load_test.go`
  - Features: Queue overflow, rate limiting
  - Acceptance: Degrades gracefully
  - Go idiom: Measure, don't guess

## Production Layer

### Scheduler Framework
- [ ] Implement cron scheduler
  - Test: Jobs run at specified times
  - Location: `internal/scheduler/scheduler.go`
  - Features: Cron syntax, timezone support
  - Acceptance: Accurate to the minute
  - Go idiom: Use well-tested libraries

- [ ] Add job registration system
  - Test: Jobs can be added/removed
  - Features: Dynamic scheduling
  - Acceptance: No duplicate runs
  - Go idiom: Make scheduling declarative

### Proactive Jobs
- [ ] Implement morning briefing job
  - Test: Generates useful briefing
  - Location: `internal/scheduler/jobs/briefing.go`
  - Features: Calendar, weather, tasks
  - Acceptance: Runs at 7am daily
  - Go idiom: Keep jobs independent

- [ ] Add reminder job framework
  - Test: Reminders are contextual
  - Location: `internal/scheduler/jobs/reminders.go`
  - Features: Smart timing, relevance
  - Acceptance: Not annoying
  - Go idiom: User control over frequency

### Main Application
- [ ] Wire everything in main.go
  - Test: Application starts cleanly
  - Location: `cmd/mentat/main.go`
  - Features: Flag parsing, config loading
  - Acceptance: --help is helpful
  - Go idiom: Keep main small

- [ ] Add graceful shutdown
  - Test: Ctrl+C stops cleanly
  - Features: Drain queues, close connections
  - Acceptance: No message loss
  - Go idiom: Use signal handling

### Configuration Management
- [ ] Implement config loading
  - Test: Validates all required fields
  - Location: `internal/config/config.go`
  - Features: YAML parsing, env override
  - Acceptance: Clear errors for bad config
  - Go idiom: Explicit over implicit

- [ ] Add configuration hot reload
  - Test: Changes apply without restart
  - Features: File watching, validation
  - Acceptance: Bad changes are rejected
  - Go idiom: Make it optional

### Observability
- [ ] Add structured logging
  - Test: Logs are parseable
  - Location: `internal/logging/logger.go`
  - Features: Context fields, levels
  - Acceptance: Can trace requests
  - Go idiom: Log actionable information

- [ ] Implement metrics collection
  - Test: Metrics are accurate
  - Location: `internal/metrics/collector.go`
  - Metrics: Queue depth, latency, success rate
  - Acceptance: Prometheus compatible
  - Go idiom: Use standard metrics

### Monitoring Endpoints
- [ ] Add health check endpoint
  - Test: Returns 200 when healthy
  - Location: `internal/api/health.go`
  - Features: Dependency checks
  - Acceptance: Useful for monitoring
  - Go idiom: Be specific about health

- [ ] Add metrics endpoint
  - Test: Exposes Prometheus metrics
  - Location: `internal/api/metrics.go`
  - Features: All key metrics exposed
  - Acceptance: Grafana can read it
  - Go idiom: Follow Prometheus conventions

### Deployment Preparation
- [ ] Create systemd service file
  - Test: Service starts on boot
  - Location: `scripts/mentat.service`
  - Features: Restart policy, logging
  - Acceptance: Survives reboot
  - Go idiom: Log to stdout/stderr

- [ ] Add Docker/Podman support
  - Test: Container runs correctly
  - Location: `Dockerfile`
  - Features: Multi-stage build
  - Acceptance: Image under 50MB
  - Go idiom: FROM scratch when possible

### Security Hardening
- [ ] Add authentication for Signal numbers
  - Test: Only allowed numbers work
  - Location: `internal/auth/validator.go`
  - Features: Whitelist checking
  - Acceptance: Clear rejection messages
  - Go idiom: Fail secure

- [ ] Implement secrets management
  - Test: No secrets in logs/config
  - Features: File-based secrets
  - Acceptance: Secrets are protected
  - Go idiom: Never log secrets

### NixOS Module
- [ ] Create NixOS module
  - Test: Module evaluates correctly
  - Location: `nix/module.nix`
  - Features: Service configuration
  - Acceptance: nixos-rebuild works
  - Go idiom: Declarative configuration

- [ ] Add flake with all dependencies
  - Test: nix build succeeds
  - Location: `flake.nix`
  - Features: Reproducible builds
  - Acceptance: Works on fresh system
  - Go idiom: Pin all dependencies

### MCP Container Setup
- [ ] Configure MCP containers in Nix
  - Test: All containers start
  - Features: Health checks, restart policies
  - Acceptance: Survives host reboot
  - Go idiom: One container per service

- [ ] Add secrets mounting
  - Test: Containers can read secrets
  - Features: Read-only mounts
  - Acceptance: Secure permissions
  - Go idiom: Principle of least privilege

### Documentation
- [ ] Write operations guide
  - Test: Can follow to deploy
  - Location: `docs/operations.md`
  - Sections: Install, configure, monitor
  - Acceptance: New user can deploy
  - Go idiom: Show, don't just tell

- [ ] Create troubleshooting guide
  - Test: Covers common issues
  - Location: `docs/troubleshooting.md`
  - Issues: Queue full, MCP down, auth fails
  - Acceptance: Solutions work
  - Go idiom: Real examples

### Performance Testing
- [ ] Run load tests
  - Test: Handle 100 msgs/minute
  - Features: Queue behavior under load
  - Acceptance: <10s p99 latency
  - Go idiom: Measure real workloads

- [ ] Profile and optimize hot paths
  - Test: No obvious bottlenecks
  - Tools: pprof, trace
  - Acceptance: CPU <10% idle
  - Go idiom: Optimize the measured

### Final Integration
- [ ] Full system test
  - Test: All features work together
  - Scenarios: Morning briefing, scheduling, memory
  - Acceptance: No regressions
  - Go idiom: Test the whole system

- [ ] Deploy to production
  - Test: Real Signal messages work
  - Features: All MCP servers connected
  - Acceptance: Responds within 10s
  - Go idiom: Start with monitoring

### Handoff
- [ ] Create runbook
  - Test: Can handle incidents
  - Location: `docs/runbook.md`
  - Scenarios: Outages, performance issues
  - Acceptance: Ops team approved
  - Go idiom: Automate solutions

- [ ] Final cleanup
  - Test: No TODOs in code
  - Tasks: Remove debug code, update README
  - Acceptance: Code is production ready
  - Go idiom: Leave it better

## Success Criteria

### Foundation Layer Success Metrics
- [ ] All interfaces defined and mockable
- [ ] Queue processes messages with proper state transitions
- [ ] Signal messages flow through system
- [ ] Basic Claude integration works
- [ ] Integration tests pass

### Intelligence Layer Success Metrics
- [ ] Multi-agent validation catches failures
- [ ] Complex requests handled gracefully
- [ ] Session continuity maintains context
- [ ] All MCP servers integrated
- [ ] Load tests pass

### Production Layer Success Metrics
- [ ] Deploys cleanly with Nix
- [ ] Handles 100 concurrent conversations
- [ ] P99 latency under 10 seconds
- [ ] Zero message loss under load
- [ ] Production monitoring active

## Testing Checklist

For each component:
- [ ] Unit tests with >80% coverage
- [ ] Integration tests for happy path
- [ ] Failure mode tests
- [ ] Concurrent access tests
- [ ] Benchmark for hot paths

## Go Idioms Reminder

1. **Accept interfaces, return structs**
2. **Make zero values useful**
3. **Return error as last value**
4. **No custom error types** - use `fmt.Errorf("context: %w", err)`
5. **Keep interfaces small**
6. **Channels for coordination, mutexes for state**
7. **`defer` for cleanup**
8. **Context for cancellation**
9. **Table-driven tests**
10. **Fail fast on configuration**

## Notes

- If stuck, step back and simplify
- When testing seems hard, the design might be wrong
- Delete old code when replacing - no compatibility layers
- Run linters (`make lint`) after each task
- Keep the main package minimal
- Document why, not what
- Optimize only after measuring