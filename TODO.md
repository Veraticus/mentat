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
- [X] Define all core interfaces - Phase 1
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

- [X] Create core types - Phase 2
  - Test: Verify types have proper zero values
  - Location: `internal/*/types.go`
  - Types: IncomingMessage, LLMResponse, QueuedMessage, MessageState, Priority
  - Acceptance: Types compile and have sensible defaults
  - Go idiom: Make zero values useful

- [X] Set up basic project structure - Phase 3
  - Test: `go mod tidy` runs without errors
  - Create all directories from architecture
  - Initialize go.mod with proper module name
  - Create Makefile with build/test/lint targets
  - Acceptance: `make build` creates empty binary

### Testing Infrastructure
- [X] Create mock implementations for all interfaces - Phase 4
  - Test: Mocks implement their interfaces correctly
  - Location: `internal/testing/mocks.go`
  - Include: MockLLM, MockMessenger, MockQueue, MockSessionManager
  - Acceptance: Mocks are usable in unit tests
  - Go idiom: Embed mutex in mocks for thread safety

- [X] Create test builders and helpers - Phase 5
  - Test: Builders create valid test data
  - Location: `internal/testing/builders.go`
  - Include: Message builders, response builders, scenario builders
  - Acceptance: Can build complex test scenarios fluently
  - Go idiom: Use functional options pattern for builders

- [X] Implement ScriptedLLM for deterministic testing - Phase 6
  - Test: ScriptedLLM returns expected responses in order
  - Location: `internal/testing/scripted_llm.go`
  - Features: Pattern matching, delays, error injection
  - Acceptance: Can script multi-turn conversations
  - Go idiom: Use table-driven tests

### Signal JSON-RPC Client
- [X] Implement JSON-RPC client - Phase 7
  - Test: Client handles connection errors gracefully
  - Location: `internal/signal/client.go`
  - Features: Connection pooling, timeout handling
  - Acceptance: Can make RPC calls with proper error handling
  - Go idiom: Return errors as last value

- [X] Implement Signal Messenger interface - Phase 8
  - Test: Send/Subscribe work with mock RPC client
  - Location: `internal/signal/messenger.go`
  - Features: Send, SendTypingIndicator, Subscribe
  - Acceptance: Messages flow through subscription channel
  - Go idiom: Close channels from sender side only

### Signal Handler
- [X] Implement Signal handler with queue integration - Phase 9
  - Test: Handler enqueues messages without blocking
  - Location: `internal/signal/handler.go`
  - Features: Subscribe loop, graceful shutdown
  - Acceptance: Ctrl+C cleanly shuts down
  - Go idiom: Use context for cancellation

- [X] Add typing indicator management - Phase 10
  - Test: Typing indicators refresh every 10 seconds
  - Location: `internal/signal/typing.go`
  - Features: Start/stop, automatic refresh
  - Acceptance: Indicators maintain during long operations
  - Go idiom: Use time.Ticker for periodic tasks

### Queue State Machine
- [X] Implement simple state machine - Phase 11
  - Test: Valid transitions succeed, invalid fail
  - Location: `internal/queue/state.go`
  - States: Queued, Processing, Validating, Completed, Failed, Retrying
  - Acceptance: State history is maintained
  - Go idiom: Use iota for enums

- [X] Add state transition validation - Phase 12
  - Test: Only allowed transitions are permitted
  - Features: Transition rules, reason tracking
  - Acceptance: Clear error messages for invalid transitions
  - Go idiom: Make invalid states unrepresentable

### Message Queue Core
- [X] Implement QueuedMessage with state tracking - Phase 13
  - Test: Messages track all state changes
  - Location: `internal/queue/message.go`
  - Features: State history, attempt counting, error tracking
  - Acceptance: Can query message history
  - Go idiom: Avoid mutations, prefer immutable updates

- [X] Implement ConversationQueue - Phase 14
  - Test: Messages maintain order within conversation
  - Location: `internal/queue/conversation.go`
  - Features: FIFO ordering, depth limits
  - Acceptance: Overflow returns clear error
  - Go idiom: Protect invariants with mutex

### Queue Manager
- [X] Implement QueueManager coordinating conversations - Phase 15
  - Test: Parallel conversations don't interfere
  - Location: `internal/queue/manager.go`
  - Features: Conversation isolation, statistics
  - Acceptance: Can handle 100 concurrent conversations
  - Go idiom: Use sync.Map for concurrent access

- [X] Add GetNext logic for workers - Phase 16
  - Test: Workers get oldest unprocessed message
  - Features: Fair scheduling, conversation affinity
  - Acceptance: No starvation across conversations
  - Go idiom: Avoid holding locks during I/O

### Rate Limiting
- [X] Implement token bucket rate limiter - Phase 17
  - Test: Burst allows N rapid requests then limits
  - Location: `internal/queue/limiter.go`
  - Features: Per-conversation limits, refill rate
  - Acceptance: Rate limit errors are clear
  - Go idiom: Use time.Since for duration calculations

- [X] Integrate rate limiting with queue - Phase 18
  - Test: Rate-limited messages retry with backoff
  - Features: Automatic retry scheduling
  - Acceptance: Eventually processes all messages
  - Go idiom: Use exponential backoff

### Worker Pool
- [X] Implement Worker interface and QueueWorker - Phase 19
  - Test: Worker processes messages in order
  - Location: `internal/queue/worker.go`
  - Features: Graceful shutdown, error handling
  - Acceptance: No message loss on shutdown
  - Go idiom: Use sync.WaitGroup for coordination

- [X] Implement WorkerPool manager - Phase 20
  - Test: Pool scales workers up/down
  - Location: `internal/queue/pool.go`
  - Features: Dynamic sizing, health checks
  - Acceptance: Handles worker failures gracefully
  - Go idiom: Prefer channels over shared memory

### Queue Integration
- [X] Wire queue components together - Phase 21
  - Test: End-to-end message flow works
  - Features: All components integrate cleanly
  - Acceptance: Message goes from enqueue to completion
  - Go idiom: Use dependency injection

- [X] Add queue statistics and monitoring - Phase 22
  - Test: Stats accurately reflect queue state
  - Location: `internal/queue/stats.go`
  - Features: Queue depth, processing time, success rate
  - Acceptance: Can expose metrics endpoint
  - Go idiom: Use atomic operations for counters

### Basic Claude Client
- [X] Implement Claude CLI wrapper - Phase 23
  - Test: Handles CLI errors and timeouts
  - Location: `internal/claude/client.go`
  - Features: Command execution, timeout handling
  - Acceptance: Returns clear errors on failure
  - Go idiom: Use exec.CommandContext for timeouts

- [X] Add response parsing - Phase 24
  - Test: Extracts message from various outputs
  - Features: Handle multiline responses, errors
  - Acceptance: Never returns empty responses
  - Go idiom: Use strings.Builder for efficiency

### MVP End-to-End Testing (PRIORITY!)
- [X] Implement minimal AgentHandler - Phase 25
  - Test: Basic Process() method works
  - Location: `internal/agent/handler.go`
  - Features: Simple pass-through to Claude CLI
  - Acceptance: Returns Claude's response
  - Note: Skip validation, enhancement, sessions for MVP
  - Go idiom: Start simple, iterate

- [X] Wire minimal main.go - Phase 26
  - Test: Application starts and connects
  - Location: `cmd/mentat/main.go`
  - Features: Hardcoded config, basic wiring
  - Components: Signal client, Queue, Worker, AgentHandler
  - Acceptance: Can receive and process Signal message
  - Go idiom: Keep main small

- [X] Test real end-to-end flow - Phase 27
  - Test: Send Signal message, get Claude response
  - Features: Full Signal → Queue → Worker → Claude → Signal
  - Acceptance: Response arrives in Signal
  - Note: Use hardcoded phone number for testing
  - Go idiom: Test the whole system early

### Claude Configuration
- [X] Implement MCP config generation - Phase 28
  - Test: Generates valid JSON config
  - Location: `internal/config/mcp.go`
  - Features: HTTP transport for all servers
  - Acceptance: Claude accepts generated config
  - Go idiom: Use struct tags for JSON

- [X] Add system prompt loading - Phase 29
  - Test: Loads and validates prompt
  - Location: `internal/config/prompt.go`
  - Features: File loading, validation
  - Acceptance: Missing prompt fails fast
  - Go idiom: Fail fast on configuration errors

### Integration Testing Framework
- [X] Create integration test harness - Phase 30
  - Test: Can run full conversation scenarios
  - Location: `tests/integration/harness_test.go`
  - Features: Setup/teardown, state verification
  - Acceptance: Tests are deterministic
  - Go idiom: Use subtests for scenarios

- [X] Write basic queue integration tests - Phase 31
  - Test: Queue handles load correctly
  - Scenarios: Overflow, rate limiting, ordering
  - Acceptance: All queue guarantees verified
  - Go idiom: Table-driven test cases

## Intelligence Layer

### Agent Handler Structure
- [X] Implement full AgentHandler with dependency injection - Phase 35
  - Test: Constructor validates required dependencies
  - Location: `internal/agent/handler.go`
  - Features: Option pattern, nil checking
  - Acceptance: Clear errors for missing deps
  - Go idiom: Functional options for construction

- [X] Add advanced Process method features - Phase 36
  - Test: Happy path processes successfully
  - Features: Session management, error handling, async validation support
  - Acceptance: Returns clear errors, validates asynchronously when needed
  - Go idiom: Handle errors explicitly

### Multi-Agent Validation Framework
- [X] Define ValidationStrategy interface implementations - Phase 37
  - Test: Each strategy has different behavior
  - Location: `internal/agent/validator.go`
  - Strategies: MultiAgent, Simple, Noop
  - Acceptance: Strategies are pluggable
  - Go idiom: Strategy pattern with interfaces

- [X] Implement validation result parsing - Phase 38
  - Test: Parses all status types correctly
  - Features: Confidence scores, issue extraction
  - Acceptance: Unknown responses default safely
  - Go idiom: Use switch for exhaustive matching

### Multi-Agent Validation Logic
- [X] Implement thoroughness checking - Phase 39
  - Test: Detects incomplete tool usage
  - Features: INCOMPLETE_SEARCH status
  - Acceptance: Catches missed memory checks
  - Go idiom: Be explicit about expectations

- [X] Add retry logic for incomplete searches - Phase 40  
  - Test: Retries lead to more complete results
  - Features: Guided retry prompts
  - Acceptance: Maximum 2 retry attempts
  - Go idiom: Fail gracefully after retries

### Recovery Generation
- [X] Implement natural recovery messages - Phase 41
  - Test: Recovery explains issues clearly
  - Location: `internal/agent/recovery.go`
  - Features: Context-aware explanations
  - Acceptance: Never exposes internal errors
  - Go idiom: User-friendly error messages

- [X] Add partial success handling - Phase 42
  - Test: Partial successes explained properly
  - Features: What worked, what didn't
  - Acceptance: Users understand state
  - Go idiom: Be honest about failures

### Intent Enhancement
- [X] Implement SmartIntentEnhancer - Phase 43
  - Test: Enhances without being prescriptive
  - Location: `internal/agent/enhancer.go`
  - Features: Pattern matching, hint mapping
  - Acceptance: Hints guide but don't dictate
  - Go idiom: Keep hints data-driven

- [X] Add intent detection logic - Phase 44
  - Test: Detects common request patterns
  - Patterns: Scheduling, finding people, memory
  - Acceptance: >90% accuracy on common intents
  - Go idiom: Prefer simple over clever

### Complex Request Detection
- [X] Implement complexity analyzer - Phase 45
  - Test: Identifies multi-step requests
  - Location: `internal/agent/complex.go`
  - Features: Step counting, dependency detection
  - Acceptance: Catches compound requests
  - Go idiom: Make detection configurable

- [X] Add gentle guidance system - Phase 46
  - Test: Guides without micromanaging
  - Features: Thoroughness hints
  - Acceptance: Claude remains autonomous
  - Go idiom: Trust but verify

### Session Management
- [X] Implement ConversationManager - Phase 47
  - Test: Sessions expire after 5 minutes
  - Location: `internal/conversation/manager.go`
  - Features: Sliding window, thread safety
  - Acceptance: No race conditions
  - Go idiom: Use RWMutex for read-heavy loads

- [X] Add session history tracking - Phase 48
  - Test: History maintains message order
  - Features: Message limit, context building
  - Acceptance: Old messages are pruned
  - Go idiom: Bounded data structures

### Session Cleanup
- [X] Implement periodic cleanup - Phase 49
  - Test: Expired sessions are removed
  - Location: `internal/conversation/cleanup.go`
  - Features: Background goroutine, graceful stop
  - Acceptance: No memory leaks
  - Go idiom: Always stop goroutines cleanly

- [X] Add session persistence interface - Phase 50
  - Test: Sessions can be saved/loaded
  - Features: JSON serialization
  - Acceptance: Restarts preserve context
  - Go idiom: Make persistence optional

### Full Agent Flow
- [X] Wire agent components together - Phase 51
  - Test: Full flow from request to response
  - Features: Enhancement → Execution → Optional Async Validation
  - Acceptance: Each step adds value, validation doesn't block response
  - Go idiom: Compose behaviors

- [X] Add comprehensive error handling - Phase 52
  - Test: All error paths return messages
  - Features: User-friendly errors
  - Acceptance: No error left unhandled
  - Go idiom: Errors are values

### Progressive Response Implementation
- [X] Create simplified ProgressReporter interface - Phase 53
  - Test: Error messages are natural language
  - Location: `internal/agent/progress.go`
  - Features: ReportError/ShouldContinue methods only
  - Acceptance: No staged progress, just error handling
  - Go idiom: Small focused interfaces

- [X] Implement error-focused ProgressReporter - Phase 54
  - Test: Falls back gracefully on LLM errors
  - Location: `internal/agent/progress_signal.go`
  - Features: LLM-generated error messages only
  - Acceptance: Clear error explanations
  - Go idiom: Fail gracefully with defaults

- [X] Add ProgressInfo to LLM response - Phase 55
  - Test: JSON progress block parsed correctly
  - Location: `internal/claude/types.go`
  - Features: ProgressInfo struct with continuation logic
  - Acceptance: LLM indicates when it's done vs needs to continue
  - Go idiom: Use struct tags for JSON parsing

- [X] Implement smart initial response system - Phase 56
  - Test: Simple queries complete in 3 seconds
  - Location: `internal/agent/handler.go`
  - Features: getInitialResponse with progress JSON
  - Acceptance: Chat-only queries don't continue processing
  - Go idiom: Early return optimization

- [X] Add continueWithProgress method - Phase 57
  - Test: Handles multi-step continuations correctly
  - Location: `internal/agent/handler.go`
  - Features: Loop until done, max 5 continuations
  - Acceptance: Complex tasks complete without timeout
  - Go idiom: Bounded iteration with context

- [ ] Update system prompt for JSON response format - Phase 58
  - Test: Claude returns structured JSON responses
  - Location: `internal/config/system-prompt.md`
  - Features: Instruct Claude to use JSON response format with message and progress fields
  - Acceptance: 100% of responses are valid JSON with separate message/progress
  - Go idiom: Documentation as code

### JSON Response Implementation
- [ ] Remove JSON extraction logic - Phase 59
  - Test: No embedded JSON parsing needed
  - Location: `internal/claude/parser.go`
  - Features: Remove extractProgressJSON function and related code
  - Acceptance: Parser only handles direct JSON response from Claude
  - Go idiom: Simplify by removing unnecessary code

- [ ] Update parser for structured JSON - Phase 60
  - Test: Parser correctly handles new JSON response format
  - Location: `internal/claude/parser.go`
  - Features: Parse JSON with message and progress as separate fields
  - Acceptance: Progress info directly accessible from response
  - Go idiom: Use standard library JSON unmarshaling

- [X] Add JSON response format tests - Phase 61
  - Test: Various JSON response formats handled correctly
  - Location: `internal/claude/parser_test.go`
  - Scenarios: Valid JSON, missing fields, extra fields, malformed JSON
  - Acceptance: Robust JSON parsing with clear error messages
  - Go idiom: Table-driven tests

### Async Validation Implementation
- [X] Update validation to use progress fields - Phase 62
  - Test: Validation checks progress.needs_validation field
  - Location: `internal/agent/handler.go`
  - Features: Use structured progress data instead of parsing
  - Acceptance: Clean validation decision based on JSON field
  - Go idiom: Use structured data over string parsing

- [X] Implement async validation launcher - Phase 63
  - Test: Validation runs without blocking response
  - Location: `internal/agent/handler.go`
  - Features: Check needs_validation, launch goroutine
  - Acceptance: User gets response before validation
  - Go idiom: Goroutines for async work

- [X] Create background validation context - Phase 64
  - Test: Validation continues after main request
  - Location: `internal/agent/validator_async.go`
  - Features: Independent context, proper cleanup
  - Acceptance: No goroutine leaks
  - Go idiom: Context for lifecycle management

- [X] Implement natural correction messages - Phase 65
  - Test: Follow-ups feel conversational
  - Location: `internal/agent/corrections.go`
  - Features: Templates for different validation results
  - Acceptance: Messages start with "Oops!" or similar
  - Go idiom: Keep messages in constants

### Selective Validation Logic
- [X] Skip validation for simple queries - Phase 66
  - Test: Greetings bypass validation entirely
  - Location: `internal/agent/handler.go`
  - Features: Check needs_validation before validating
  - Acceptance: "Hi" completes in <3 seconds
  - Go idiom: Early returns for fast paths

- [X] Add validation decision logging - Phase 67
  - Test: Can trace why validation ran/skipped
  - Location: `internal/agent/handler.go`
  - Features: Log validation decisions with reasons
  - Acceptance: Clear audit trail
  - Go idiom: Structured logging

### Follow-up Message Handling
- [ ] Implement validation result handler - Phase 68
  - Test: Appropriate follow-ups for each result
  - Location: `internal/agent/followup.go`
  - Features: Send corrections based on validation status
  - Acceptance: Natural conversation flow
  - Go idiom: Switch for exhaustive handling

- [ ] Add delay before validation messages - Phase 69
  - Test: User has time to read initial response
  - Location: `internal/agent/validator_async.go`
  - Features: 2-second delay before corrections
  - Acceptance: Messages don't feel rushed
  - Go idiom: time.Sleep in goroutines only

- [ ] Handle validation timeouts gracefully - Phase 70
  - Test: System continues if validation hangs
  - Location: `internal/agent/validator_async.go`
  - Features: Context timeout, skip corrections
  - Acceptance: No user-visible delays
  - Go idiom: Always use timeouts

### Integration Testing for JSON Response
- [ ] Test JSON response parsing - Phase 71
  - Test: All JSON response formats handled correctly
  - Location: `tests/integration/json_response_test.go`
  - Scenarios: Valid responses, missing fields, extra fields, errors
  - Acceptance: Robust handling of all Claude response variations
  - Go idiom: Test the unhappy paths

- [ ] Test async validation flow - Phase 72
  - Test: Validation doesn't block responses
  - Location: `tests/integration/async_validation_test.go`
  - Scenarios: Simple queries, tool usage, failures
  - Acceptance: Response times meet targets
  - Go idiom: Use channels to verify async behavior

- [ ] Test follow-up message timing - Phase 73
  - Test: Corrections arrive after appropriate delay
  - Location: `tests/integration/followup_timing_test.go`
  - Scenarios: Quick corrections, slow validation
  - Acceptance: Natural conversation pacing
  - Go idiom: time.After for timing assertions

### Parallel Validation Implementation
- [ ] Move validation strategies to async - Phase 74
  - Test: Strategies work with new async flow
  - Location: `internal/agent/validator_parallel.go`
  - Features: Adapt existing strategies
  - Acceptance: Same validation quality
  - Go idiom: Preserve interfaces

- [ ] Optimize validation performance - Phase 75
  - Test: Validation completes quickly
  - Location: `internal/agent/validator.go`
  - Features: Cache validation results
  - Acceptance: <3s validation time
  - Go idiom: Measure before optimizing

### Error Recovery Enhancement
- [ ] Implement immediate error reporting - Phase 76
  - Test: MCP errors trigger natural explanations
  - Location: `internal/agent/error_handler.go`
  - Features: Error classification, Claude-generated messages
  - Acceptance: No raw errors shown to users
  - Go idiom: Wrap errors with context

- [ ] Add progressive error recovery - Phase 77
  - Test: Errors reported as they occur
  - Features: Connection/permission/timeout detection
  - Acceptance: User understands what went wrong
  - Go idiom: Error type assertions

### End-to-End Testing
- [ ] Build end-to-end latency tests - Phase 78
  - Test: Initial ack in 3s, response in 8s
  - Location: `tests/integration/latency_test.go`
  - Metrics: Time to ack, time to response, time to validation
  - Acceptance: P95 latency meets targets
  - Go idiom: Benchmark tests

- [ ] Test continuation flow - Phase 79
  - Test: Multi-step requests complete correctly
  - Location: `tests/integration/continuation_test.go`
  - Scenarios: 0, 1, 5 continuations needed
  - Acceptance: All steps execute with progress updates
  - Go idiom: State machine testing

- [ ] Test smart initial response - Phase 80
  - Test: Simple queries complete without continuation
  - Location: `tests/integration/quick_response_test.go`
  - Scenarios: Chat queries, simple questions, clarifications
  - Acceptance: Response in 3s with needs_continuation=false
  - Go idiom: Timeout-based assertions

### Agent Testing Scenarios
- [ ] Build complex test scenarios - Phase 81
  - Test: Multi-turn conversations work
  - Location: `tests/scenarios/complex_requests.go`
  - Scenarios: Scheduling, partial failures
  - Acceptance: Scenarios are reusable
  - Go idiom: Data-driven test cases

- [ ] Add failure mode testing - Phase 82
  - Test: System handles all failure types
  - Location: `tests/scenarios/failure_modes.go`
  - Modes: Timeout, invalid response, tool errors
  - Acceptance: Graceful degradation
  - Go idiom: Test the unhappy path

### MCP Configuration Integration
- [ ] Create MCP config generator - Phase 83
  - Test: Config matches Claude's format
  - Location: `internal/config/generator.go`
  - Features: All 5 MCP servers configured
  - Acceptance: Claude accepts config
  - Go idiom: Generate, don't template

- [ ] Add config validation - Phase 84
  - Test: Invalid configs fail fast
  - Features: URL validation, server checks
  - Acceptance: Clear error messages
  - Go idiom: Validate at boundaries

### MCP Health Checking
- [ ] Implement MCP server health checks - Phase 85
  - Test: Detects when servers are down
  - Location: `internal/mcp/health.go`
  - Features: HTTP health endpoints
  - Acceptance: Quick detection (<5s)
  - Go idiom: Use context with timeout

- [ ] Add startup verification - Phase 86
  - Test: Won't start without MCP servers
  - Features: Retry logic, clear errors
  - Acceptance: Helpful error messages
  - Go idiom: Fail fast and loud

### Intelligence Layer Integration Testing
- [ ] Create end-to-end test scenarios - Phase 87
  - Test: Real Claude integration works
  - Location: `tests/integration/e2e_test.go`
  - Tag: `// +build integration`
  - Acceptance: Tests can run locally
  - Go idiom: Skip if Claude unavailable

- [ ] Add load testing - Phase 88
  - Test: System handles 50 concurrent users
  - Location: `tests/integration/load_test.go`
  - Features: Queue overflow, rate limiting
  - Acceptance: Degrades gracefully
  - Go idiom: Measure, don't guess

## Production Layer

### Scheduler Framework
- [ ] Implement cron scheduler - Phase 89
  - Test: Jobs run at specified times
  - Location: `internal/scheduler/scheduler.go`
  - Features: Cron syntax, timezone support
  - Acceptance: Accurate to the minute
  - Go idiom: Use well-tested libraries

- [ ] Add job registration system - Phase 90
  - Test: Jobs can be added/removed
  - Features: Dynamic scheduling
  - Acceptance: No duplicate runs
  - Go idiom: Make scheduling declarative

### Proactive Jobs
- [ ] Implement morning briefing job - Phase 91
  - Test: Generates useful briefing
  - Location: `internal/scheduler/jobs/briefing.go`
  - Features: Calendar, weather, tasks
  - Acceptance: Runs at 7am daily
  - Go idiom: Keep jobs independent

- [ ] Add reminder job framework - Phase 92
  - Test: Reminders are contextual
  - Location: `internal/scheduler/jobs/reminders.go`
  - Features: Smart timing, relevance
  - Acceptance: Not annoying
  - Go idiom: User control over frequency

### Main Application
- [ ] Wire full production main.go - Phase 93
  - Test: Application starts cleanly
  - Location: `cmd/mentat/main.go`
  - Features: Flag parsing, config loading
  - Acceptance: --help is helpful
  - Go idiom: Keep main small

- [ ] Add graceful shutdown - Phase 94
  - Test: Ctrl+C stops cleanly
  - Features: Drain queues, close connections
  - Acceptance: No message loss
  - Go idiom: Use signal handling

### Configuration Management
- [ ] Implement config loading - Phase 95
  - Test: Validates all required fields
  - Location: `internal/config/config.go`
  - Features: YAML parsing, env override
  - Acceptance: Clear errors for bad config
  - Go idiom: Explicit over implicit

- [ ] Add configuration hot reload - Phase 96
  - Test: Changes apply without restart
  - Features: File watching, validation
  - Acceptance: Bad changes are rejected
  - Go idiom: Make it optional

### Observability
- [ ] Add structured logging - Phase 97
  - Test: Logs are parseable
  - Location: `internal/logging/logger.go`
  - Features: Context fields, levels
  - Acceptance: Can trace requests
  - Go idiom: Log actionable information

- [ ] Implement metrics collection - Phase 98
  - Test: Metrics are accurate
  - Location: `internal/metrics/collector.go`
  - Metrics: Queue depth, latency, success rate
  - Acceptance: Prometheus compatible
  - Go idiom: Use standard metrics

### Monitoring Endpoints
- [ ] Add health check endpoint - Phase 99
  - Test: Returns 200 when healthy
  - Location: `internal/api/health.go`
  - Features: Dependency checks
  - Acceptance: Useful for monitoring
  - Go idiom: Be specific about health

- [ ] Add metrics endpoint - Phase 100
  - Test: Exposes Prometheus metrics
  - Location: `internal/api/metrics.go`
  - Features: All key metrics exposed
  - Acceptance: Grafana can read it
  - Go idiom: Follow Prometheus conventions

### Deployment Preparation
- [ ] Create systemd service file - Phase 101
  - Test: Service starts on boot
  - Location: `scripts/mentat.service`
  - Features: Restart policy, logging
  - Acceptance: Survives reboot
  - Go idiom: Log to stdout/stderr

- [ ] Add Docker/Podman support - Phase 102
  - Test: Container runs correctly
  - Location: `Dockerfile`
  - Features: Multi-stage build
  - Acceptance: Image under 50MB
  - Go idiom: FROM scratch when possible

### Security Hardening
- [ ] Add authentication for Signal numbers - Phase 103
  - Test: Only allowed numbers work
  - Location: `internal/auth/validator.go`
  - Features: Whitelist checking
  - Acceptance: Clear rejection messages
  - Go idiom: Fail secure

- [ ] Implement secrets management - Phase 104
  - Test: No secrets in logs/config
  - Features: File-based secrets
  - Acceptance: Secrets are protected
  - Go idiom: Never log secrets

### NixOS Module
- [ ] Create NixOS module - Phase 105
  - Test: Module evaluates correctly
  - Location: `nix/module.nix`
  - Features: Service configuration
  - Acceptance: nixos-rebuild works
  - Go idiom: Declarative configuration

- [ ] Add flake with all dependencies - Phase 106
  - Test: nix build succeeds
  - Location: `flake.nix`
  - Features: Reproducible builds
  - Acceptance: Works on fresh system
  - Go idiom: Pin all dependencies

### MCP Container Setup
- [ ] Configure MCP containers in Nix - Phase 107
  - Test: All containers start
  - Features: Health checks, restart policies
  - Acceptance: Survives host reboot
  - Go idiom: One container per service

- [ ] Add secrets mounting - Phase 108
  - Test: Containers can read secrets
  - Features: Read-only mounts
  - Acceptance: Secure permissions
  - Go idiom: Principle of least privilege

### Documentation
- [ ] Write operations guide - Phase 109
  - Test: Can follow to deploy
  - Location: `docs/operations.md`
  - Sections: Install, configure, monitor
  - Acceptance: New user can deploy
  - Go idiom: Show, don't just tell

- [ ] Create troubleshooting guide - Phase 110
  - Test: Covers common issues
  - Location: `docs/troubleshooting.md`
  - Issues: Queue full, MCP down, auth fails
  - Acceptance: Solutions work
  - Go idiom: Real examples

### Performance Testing
- [ ] Run load tests - Phase 111
  - Test: Handle 100 msgs/minute
  - Features: Queue behavior under load
  - Acceptance: <10s p99 latency
  - Go idiom: Measure real workloads

- [ ] Profile and optimize hot paths - Phase 112
  - Test: No obvious bottlenecks
  - Tools: pprof, trace
  - Acceptance: CPU <10% idle
  - Go idiom: Optimize the measured

### Final Integration
- [ ] Full system test - Phase 113
  - Test: All features work together
  - Scenarios: Morning briefing, scheduling, memory
  - Acceptance: No regressions
  - Go idiom: Test the whole system

- [ ] Deploy to production - Phase 114
  - Test: Real Signal messages work
  - Features: All MCP servers connected
  - Acceptance: Responds within 10s
  - Go idiom: Start with monitoring

### Handoff
- [ ] Create runbook - Phase 115
  - Test: Can handle incidents
  - Location: `docs/runbook.md`
  - Scenarios: Outages, performance issues
  - Acceptance: Ops team approved
  - Go idiom: Automate solutions

- [ ] Final cleanup - Phase 116
  - Test: No TODOs in code
  - Tasks: Remove debug code, update README
  - Acceptance: Code is production ready
  - Go idiom: Leave it better

## Success Criteria

### Foundation Layer Success Metrics
- [ ] All interfaces defined and mockable - Phase 117
- [ ] Queue processes messages with proper state transitions - Phase 118
- [ ] Signal messages flow through system - Phase 119
- [ ] Basic Claude integration works - Phase 120
- [ ] Integration tests pass - Phase 121

### Intelligence Layer Success Metrics
- [ ] Multi-agent validation catches failures - Phase 122
- [ ] Complex requests handled gracefully - Phase 123
- [ ] Session continuity maintains context - Phase 124
- [ ] All MCP servers integrated - Phase 125
- [ ] Load tests pass - Phase 126

### Production Layer Success Metrics
- [ ] Deploys cleanly with Nix - Phase 127
- [ ] Handles 100 concurrent conversations - Phase 128
- [ ] P99 latency under 10 seconds - Phase 129
- [ ] Zero message loss under load - Phase 130
- [ ] Production monitoring active - Phase 131

## Testing Checklist

For each component:
- [ ] Unit tests with >80% coverage - Phase 132
- [ ] Integration tests for happy path - Phase 133
- [ ] Failure mode tests - Phase 134
- [ ] Concurrent access tests - Phase 135
- [ ] Benchmark for hot paths - Phase 136

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
