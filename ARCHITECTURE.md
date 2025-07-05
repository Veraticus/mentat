# Mentat - Personal Assistant Bot Architecture

## Overview

A thin Go orchestration layer that transforms Claude Code into a reliable personal assistant accessible via Signal. The system leverages Claude's MCP (Model Context Protocol) integration for extensibility while adding proactive scheduling capabilities, conversation-aware queuing, and multi-agent validation to ensure reliable execution under load.

## Core Architecture Philosophy

- **Thin orchestration**: Go handles only Signal I/O, queuing, scheduling, and validation flow
- **Multi-agent reliability**: Use Claude to validate Claude, eliminating hallucinated successes
- **Resilient under load**: Queue-based architecture with rate limiting prevents overload
- **Conversation-first**: All user-facing messages generated naturally by Claude
- **Trust through verification**: Complex actions verified asynchronously after user response
- **Interface-driven**: All external dependencies behind interfaces for testability
- **Local-first privacy**: Everything runs on your infrastructure
- **Optimistic responses**: Send first, validate second - users get immediate feedback

## System Architecture

```
┌─────────────────────────────────────────────────────────┐
│                   Mentat Orchestrator                    │
│                                                          │
│  ┌─────────────────┐         ┌─────────────────────┐    │
│  │   Signal I/O    │         │  Proactive Jobs     │    │
│  │  (JSON-RPC)     │         │   (Cron Tasks)      │    │
│  └────────┬────────┘         └──────────┬──────────┘    │
│           │                              │               │
│           └──────────────┬───────────────┘               │
│                          │                               │
│        ┌─────────────────┴─────────────────┐            │
│        │     Command Queue & State Machine  │            │
│        │  ┌─────────┐ ┌─────────┐ ┌──────┐│            │
│        │  │ Queue   │ │ Workers │ │State ││            │
│        │  │ Manager │ │  Pool   │ │ Mgr  ││            │
│        │  └─────────┘ └─────────┘ └──────┘│            │
│        └─────────────────┬─────────────────┘            │
│                          │                               │
│           ┌──────────────┴──────────────┐               │
│           │    Multi-Agent Handler      │               │
│           │  ┌────────┐ ┌────────────┐ │               │
│           │  │Execute │ │  Validate   │ │               │
│           │  │ Agent  │ │   Agent     │ │               │
│           │  └────┬───┘ └─────┬──────┘ │               │
│           └───────┼───────────┼────────┘               │
│                   └─────┬─────┘                         │
│           ┌─────────────┴───────────────┐               │
│           │       Claude Code           │               │
│           │      (via CLI/SDK)          │               │
│           └─────────────┬───────────────┘               │
│                         │                               │
│        ┌────────────────┴────────────────┐              │
│        │        MCP Servers              │              │
│        │  ┌───────────┐ ┌────────────┐  │              │
│        │  │ Calendar  │ │   Memory   │  │              │
│        │  └───────────┘ └────────────┘  │              │
│        │  │   Gmail   │ │  Contacts  │  │              │
│        │  └───────────┘ └────────────┘  │              │
│        │  │  Todoist  │ │  Expensify │  │              │
│        │  └───────────┘ └────────────┘  │              │
│        └─────────────────────────────────┘              │
└─────────────────────────────────────────────────────────┘
```

## Core Interfaces

### 1. LLM Interface

```go
// LLM interface abstracts all Claude interactions
type LLM interface {
    Query(ctx context.Context, prompt string, sessionID string) (*LLMResponse, error)
}

type LLMResponse struct {
    Message   string            `json:"message"`
    ToolCalls []ToolCall       `json:"tool_calls,omitempty"` // For future tool call visibility
    Metadata  ResponseMetadata `json:"metadata"`
    // Structured response for progress tracking
    Progress  *ProgressInfo    `json:"progress,omitempty"`
}

// ProgressInfo provides structured progress tracking
type ProgressInfo struct {
    // Does the LLM need to continue processing?
    NeedsContinuation bool   `json:"needs_continuation"`
    // Why does it need to continue? (empty if complete)
    ContinuationReason string `json:"continuation_reason,omitempty"`
    // What's the next planned action?
    NextAction string `json:"next_action,omitempty"`
    // Is the user's intent fully satisfied?
    IntentSatisfied bool `json:"intent_satisfied"`
    // Does the LLM need clarification from user?
    NeedsClarification bool   `json:"needs_clarification,omitempty"`
    ClarificationPrompt string `json:"clarification_prompt,omitempty"`
    // Does this response need validation? (tool usage, complex operations)
    NeedsValidation bool `json:"needs_validation"`
    // Current status in natural language
    Status string `json:"status"`
    // Optional message for the user
    Message string `json:"message,omitempty"`
    // Estimated remaining continuations (0 if done)
    EstimatedRemaining int `json:"estimated_remaining"`
}

type ResponseMetadata struct {
    TokensUsed   int
    Latency      time.Duration
    ModelVersion string
}

// Production implementation
type ClaudeCLI struct {
    command    string
    mcpConfig  string
    systemPrompt string
}

func (c *ClaudeCLI) Query(ctx context.Context, prompt string, sessionID string) (*LLMResponse, error) {
    start := time.Now()
    cmd := exec.CommandContext(ctx, c.command,
        prompt,
        "--mcp-config", c.mcpConfig,
        "--session", sessionID,
        "--system-prompt", c.systemPrompt,
        "--format", "json", // Request JSON format from Claude
    )
    
    output, err := cmd.Output()
    if err != nil {
        return nil, fmt.Errorf("claude query failed: %w", err)
    }
    
    // Parse JSON response from Claude
    var response LLMResponse
    if err := json.Unmarshal(output, &response); err != nil {
        // Fallback for plain text responses
        return &LLMResponse{
            Message: string(output),
            Metadata: ResponseMetadata{
                Latency: time.Since(start),
            },
        }, nil
    }
    
    // Update metadata
    response.Metadata.Latency = time.Since(start)
    return &response, nil
}
```

### 2. Messenger Interface

```go
// Messenger abstracts Signal (and future channels)
type Messenger interface {
    Send(ctx context.Context, recipient string, message string) error
    SendTypingIndicator(ctx context.Context, recipient string) error
    Subscribe(ctx context.Context) (<-chan IncomingMessage, error)
}

type IncomingMessage struct {
    From      string
    Text      string
    Timestamp time.Time
}

// Production Signal implementation
type SignalClient struct {
    jsonRPC *JSONRPCClient
}

func (s *SignalClient) Send(ctx context.Context, recipient string, message string) error {
    return s.jsonRPC.Call(ctx, "send", map[string]interface{}{
        "recipient": recipient,
        "message":   message,
    })
}
```

### 3. Session Manager Interface

```go
// SessionManager handles conversation continuity
type SessionManager interface {
    GetOrCreateSession(identifier string) string
    GetSessionHistory(sessionID string) []Message
    ExpireSessions(before time.Time) int
    GetLastSessionID(identifier string) string
}

// Production implementation with 5-minute window
type ConversationManager struct {
    sessions   map[string]*Session
    mu         sync.RWMutex
    windowTime time.Duration
    clock      Clock  // For testability
}
```

### 4. Validation Strategy Interface

```go
// ValidationStrategy allows pluggable validation approaches
type ValidationStrategy interface {
    Validate(ctx context.Context, request, response string, llm LLM) ValidationResult
    ShouldRetry(result ValidationResult) bool
    GenerateRecovery(ctx context.Context, request, response string, result ValidationResult, llm LLM) string
}

type ValidationResult struct {
    Status      ValidationStatus // SUCCESS, PARTIAL, FAILED, UNCLEAR, INCOMPLETE_SEARCH
    Confidence  float64
    Issues      []string
    Suggestions []string
}

// Multi-agent validation with thoroughness checking
type MultiAgentValidator struct {
    maxAttempts int
}

func (v *MultiAgentValidator) Validate(ctx context.Context, request, response string, llm LLM) ValidationResult {
    validationResp, err := llm.Query(ctx, fmt.Sprintf(`
        Review this interaction:
        User asked: "%s"
        Assistant responded: "%s"
        
        1. Was the request fully completed?
        2. Did you check all relevant information sources?
        3. Could more complete information be found with different tools?
        
        Answer: SUCCESS, PARTIAL, FAILED, UNCLEAR, or INCOMPLETE_SEARCH
        
        Be strict - only SUCCESS if request is complete AND thorough.
    `, request, response), "validation")
    
    if err != nil {
        return ValidationResult{Status: UNCLEAR}
    }
    
    return v.parseValidationResponse(validationResp.Message)
}
```

### 5. Intent Enhancement Interface

```go
// IntentEnhancer provides gentle guidance without prescribing exact tools
type IntentEnhancer interface {
    Enhance(originalRequest string) string
    ShouldEnhance(request string) bool
}

type SmartIntentEnhancer struct {
    hints map[string]string
}

func NewSmartIntentEnhancer() *SmartIntentEnhancer {
    return &SmartIntentEnhancer{
        hints: map[string]string{
            "find_person": "Check all available sources for complete information about this person",
            "schedule": "Gather full context including availability and preferences before creating events",
            "remember": "Store important details that will be useful in future conversations",
            "email": "Ensure you have correct contact information and context before sending",
        },
    }
}

func (e *SmartIntentEnhancer) Enhance(originalRequest string) string {
    // Detect likely intents without being prescriptive
    for intent, hint := range e.hints {
        if e.matchesIntent(originalRequest, intent) {
            return fmt.Sprintf("%s\n\nNote: %s", originalRequest, hint)
        }
    }
    return originalRequest
}
```

### 6. Progressive Response Interface

```go
// ProgressReporter provides real-time user feedback during long operations
type ProgressReporter interface {
    // Report sends a progress update to the user
    Report(ctx context.Context, recipient string, stage ProgressStage, detail string) error
    // ReportError immediately sends error context to user
    ReportError(ctx context.Context, recipient string, err error, context string) error
    // ShouldContinue checks if the LLM needs to continue processing
    ShouldContinue(progress *ProgressInfo) bool
}

type ProgressStage int

const (
    StageStarting ProgressStage = iota
    StageSearching      // "Searching calendar..."
    StageProcessing     // "Found your events, checking details..."
    StageValidating     // "Verifying the information..."
    StageRetrying       // "Let me check that again..."
    StageCompleting     // "Almost done..."
)

// SignalProgressReporter sends natural language progress updates
type SignalProgressReporter struct {
    messenger Messenger
    llm       LLM  // Use Claude to generate natural progress messages
}

func (r *SignalProgressReporter) Report(ctx context.Context, recipient string, stage ProgressStage, detail string) error {
    // Generate natural progress message
    prompt := fmt.Sprintf(`
        Generate a brief, natural progress update for stage: %v
        Context: %s
        Keep it under 10 words and conversational.
        Examples: "Checking your calendar...", "Found it, getting details..."
    `, stage, detail)
    
    resp, err := r.llm.Query(ctx, prompt, "progress")
    if err != nil {
        // Fallback to simple message
        return r.messenger.Send(ctx, recipient, "Working on it...")
    }
    
    return r.messenger.Send(ctx, recipient, resp.Message)
}

func (r *SignalProgressReporter) ReportError(ctx context.Context, recipient string, err error, context string) error {
    // Let Claude explain the error naturally
    prompt := fmt.Sprintf(`
        The user asked: %s
        An error occurred: %v
        
        Explain this naturally and suggest what the user can do.
        Be helpful and conversational, not technical.
    `, context, err)
    
    resp, err := r.llm.Query(ctx, prompt, "error-recovery")
    if err != nil {
        return r.messenger.Send(ctx, recipient, 
            "I'm having trouble with that request right now. Could you try again in a moment?")
    }
    
    return r.messenger.Send(ctx, recipient, resp.Message)
}

func (r *SignalProgressReporter) ShouldContinue(progress *ProgressInfo) bool {
    if progress == nil {
        return true // Conservative default
    }
    
    return progress.NeedsContinuation && 
           !progress.NeedsClarification && 
           !progress.IntentSatisfied
}
```

### Enhanced Agent Handler Methods

```go
func (h *AgentHandler) continueWithProgress(
    ctx context.Context,
    msg IncomingMessage,
    previousResponse *LLMResponse,
    sessionID string,
) {
    // Send initial response
    h.messenger.Send(ctx, msg.From, previousResponse.Message)
    
    // LLM naturally reports what it's doing in its response
    
    maxContinuations := 5
    response := previousResponse
    
    for i := 0; i < maxContinuations && response.Progress.NeedsContinuation; i++ {
        // Continue processing with the reason
        continuationPrompt := fmt.Sprintf(`
            Continue processing the user's request.
            Previous status: %s
            Next action: %s
            
            Complete the action and provide an update.
            Include the same JSON progress structure in your response.
        `, response.Progress.ContinuationReason, response.Progress.NextAction)
        
        nextResponse, err := h.llm.Query(ctx, continuationPrompt, sessionID)
        if err != nil {
            h.progress.ReportError(ctx, msg.From, err, "continuation failed")
            break
        }
        
        // Send incremental update
        h.messenger.Send(ctx, msg.From, nextResponse.Message)
        
        // Check if we should continue
        if nextResponse.Progress == nil || !h.progress.ShouldContinue(nextResponse.Progress) {
            break
        }
        
        response = nextResponse
        
        // LLM naturally reports what it's doing - no separate progress messages
    }
    
    // Run validation on the final result
    go h.validateInBackground(ctx, msg, response, sessionID)
}
```

### 7. MCP Server Integration

MCP servers run as HTTP services via podman containers, eliminating subprocess startup overhead:

```go
// MCPConfig represents the Claude Code MCP configuration
type MCPConfig struct {
    Version string                   `json:"version"`
    Servers map[string]ServerConfig `json:"servers"`
}

type ServerConfig struct {
    Transport   string `json:"transport"`
    URL         string `json:"url"`
    Name        string `json:"name"`
    Description string `json:"description"`
}

// GenerateMCPConfig creates the configuration for Claude Code
func GenerateMCPConfig() MCPConfig {
    return MCPConfig{
        Version: "1.0",
        Servers: map[string]ServerConfig{
            "calendar": {
                Transport:   "http",
                URL:         "http://localhost:3000",
                Name:        "Google Calendar",
                Description: "Google Calendar integration",
            },
            "contacts": {
                Transport:   "http",
                URL:         "http://localhost:3001",
                Name:        "Google Contacts",
                Description: "Google Contacts integration",
            },
            "gmail": {
                Transport:   "http",
                URL:         "http://localhost:3002",
                Name:        "Gmail",
                Description: "Gmail integration",
            },
            "todoist": {
                Transport:   "http",
                URL:         "http://localhost:3003",
                Name:        "Todoist",
                Description: "Task management",
            },
            "memory": {
                Transport:   "http",
                URL:         "http://localhost:3004",
                Name:        "Memory",
                Description: "Persistent memory storage",
            },
            "expensify": {
                Transport:   "http",
                URL:         "http://localhost:3005",
                Name:        "Expensify",
                Description: "Expense tracking and reporting",
            },
        },
    }
}
```

### 7. Queue Management Interfaces

```go
// MessageQueue provides conversation-aware queuing
type MessageQueue interface {
    Enqueue(msg IncomingMessage) error
    // Workers call GetNext to receive work
    GetNext(workerID string) (*QueuedMessage, error)
    // Mark message state transitions
    UpdateState(msgID string, state MessageState, reason string) error
    // Get queue statistics
    Stats() QueueStats
}

// QueuedMessage represents a message in the queue
type QueuedMessage struct {
    ID             string
    ConversationID string
    From           string
    Text           string
    State          MessageState
    Priority       Priority
    QueuedAt       time.Time
    Attempts       int
    LastError      error
}

// MessageState represents the state machine states
type MessageState int

const (
    StateQueued MessageState = iota
    StateProcessing
    StateValidating
    StateCompleted
    StateFailed
    StateRetrying
)

// Simple priority levels
type Priority int

const (
    PriorityNormal Priority = iota
    PriorityHigh   // Scheduled tasks, retries
)

// Worker processes messages from the queue
type Worker interface {
    Start(ctx context.Context) error
    Stop() error
    ID() string
}

// RateLimiter provides per-conversation rate limiting
type RateLimiter interface {
    // Allow returns true if the conversation can proceed
    Allow(conversationID string) bool
    // Record marks a conversation as active
    Record(conversationID string)
}

// StateMachine manages message state transitions
type StateMachine interface {
    // CanTransition validates if a state transition is allowed
    CanTransition(from, to MessageState) bool
    // Transition updates the message state
    Transition(msg *QueuedMessage, to MessageState, reason string) error
    // GetAllowedTransitions returns valid next states
    GetAllowedTransitions(from MessageState) []MessageState
}
```

### 7. Queue Implementation Patterns

```go
// ConversationQueue ensures messages in a conversation process in order
type ConversationQueue struct {
    mu           sync.Mutex
    messages     []*QueuedMessage
    processing   bool
    lastActivity time.Time
}

// QueueManager coordinates multiple conversation queues
type QueueManager struct {
    conversations map[string]*ConversationQueue
    workers       []Worker
    rateLimiter   RateLimiter
    stateMachine  StateMachine
    maxDepth      int
    mu            sync.RWMutex
}

func (qm *QueueManager) Enqueue(msg IncomingMessage) error {
    qm.mu.Lock()
    defer qm.mu.Unlock()
    
    // Get conversation queue
    convID := qm.getConversationID(msg.From)
    queue, exists := qm.conversations[convID]
    if !exists {
        queue = &ConversationQueue{
            lastActivity: time.Now(),
        }
        qm.conversations[convID] = queue
    }
    
    // Check depth limit
    if len(queue.messages) >= qm.maxDepth {
        return fmt.Errorf("conversation queue full")
    }
    
    // Create queued message
    queuedMsg := &QueuedMessage{
        ID:             uuid.New().String(),
        ConversationID: convID,
        From:           msg.From,
        Text:           msg.Text,
        State:          StateQueued,
        Priority:       PriorityNormal,
        QueuedAt:       time.Now(),
    }
    
    queue.messages = append(queue.messages, queuedMsg)
    return nil
}

// WorkerPool manages a pool of workers
type WorkerPool struct {
    queue    MessageQueue
    handler  MessageHandler
    workers  []Worker
    wg       sync.WaitGroup
}

func (wp *WorkerPool) Start(ctx context.Context, numWorkers int) {
    for i := 0; i < numWorkers; i++ {
        w := &QueueWorker{
            id:      fmt.Sprintf("worker-%d", i),
            queue:   wp.queue,
            handler: wp.handler,
        }
        wp.workers = append(wp.workers, w)
        
        wp.wg.Add(1)
        go func() {
            defer wp.wg.Done()
            w.Start(ctx)
        }()
    }
}
```

## Queue Integration Overview

The queue system sits between Signal input and Agent processing, providing:

1. **Protection**: Rate limiting and queue depth limits prevent overload
2. **Reliability**: State tracking and automatic retries ensure message delivery
3. **Fairness**: Per-conversation queuing maintains order while allowing parallelism
4. **Observability**: Every state transition is tracked for debugging

Key integration points:
- Signal Handler enqueues messages instead of direct processing
- Workers pull from queue and invoke Agent Handler
- Typing indicators managed by workers during processing
- Scheduler can enqueue high-priority tasks
- All components remain testable through interfaces

## Response Streaming and Progressive Enhancement

The system provides real-time feedback during long operations to maintain user engagement and trust. This is critical given the multi-agent validation flow that can take 15-20 seconds.

### Progressive Response Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                   Progressive Response Flow                  │
│                                                              │
│  User Message                                                │
│       ↓                                                      │
│  "Checking..." (immediate)                                   │
│       ↓                                                      │
│  "Found your calendar..." (2s)                               │
│       ↓                                                      │
│  "I see 3 events today..." (4s)                             │
│       ↓                                                      │
│  [Full detailed response] (6-8s)                             │
│       ↓                                                      │
│  [Validation corrections if needed] (async)                  │
└─────────────────────────────────────────────────────────────┘
```

### Implementation Strategy

```go
// Enhanced AgentHandler with progress reporting
type ProgressiveAgentHandler struct {
    *AgentHandler
    progress ProgressReporter
}

func (h *ProgressiveAgentHandler) Process(ctx context.Context, msg IncomingMessage) error {
    sessionID := h.sessions.GetOrCreateSession(msg.From)
    
    // Immediate acknowledgment
    h.progress.Report(ctx, msg.From, StageStarting, msg.Text)
    
    // Enhance request
    enhancedRequest := h.enhancer.Enhance(msg.Text)
    
    // Execute with progress callbacks
    execution, err := h.executeWithProgress(ctx, enhancedRequest, sessionID, msg.From)
    
    if err != nil {
        // Immediate error reporting with context
        return h.progress.ReportError(ctx, msg.From, err, msg.Text)
    }
    
    // Send initial response immediately
    h.messenger.Send(ctx, msg.From, execution.Message)
    
    // Only validate if Claude indicates it's necessary
    if execution.Progress != nil && execution.Progress.NeedsValidation {
        // Async validation with corrections
        go h.validateAndCorrect(ctx, msg, execution, sessionID)
    }
    
    return nil
}

func (h *ProgressiveAgentHandler) executeWithProgress(
    ctx context.Context, 
    request string, 
    sessionID string,
    recipient string,
) (*LLMResponse, error) {
    // Create a wrapper LLM that reports progress based on tool usage
    progressLLM := &ProgressTrackingLLM{
        llm:      h.llm,
        progress: h.progress,
        recipient: recipient,
    }
    
    return progressLLM.Query(ctx, request, sessionID)
}
```

### Progress Tracking LLM Wrapper

```go
// ProgressTrackingLLM monitors tool usage and reports progress
type ProgressTrackingLLM struct {
    llm       LLM
    progress  ProgressReporter
    recipient string
}

func (p *ProgressTrackingLLM) Query(ctx context.Context, prompt string, sessionID string) (*LLMResponse, error) {
    // Set up tool usage monitoring
    monitorCtx, monitor := p.createToolMonitor(ctx)
    
    // Execute query with monitoring
    responseChan := make(chan *LLMResponse, 1)
    errChan := make(chan error, 1)
    
    go func() {
        resp, err := p.llm.Query(monitorCtx, prompt, sessionID)
        if err != nil {
            errChan <- err
        } else {
            responseChan <- resp
        }
    }()
    
    // Monitor progress
    for {
        select {
        case tool := <-monitor.ToolUsed:
            p.reportToolProgress(ctx, tool)
        case resp := <-responseChan:
            return resp, nil
        case err := <-errChan:
            return nil, err
        case <-ctx.Done():
            return nil, ctx.Err()
        }
    }
}

func (p *ProgressTrackingLLM) reportToolProgress(ctx context.Context, tool ToolUsage) {
    switch tool.Name {
    case "google-calendar":
        p.progress.Report(ctx, p.recipient, StageSearching, "calendar")
    case "memory":
        p.progress.Report(ctx, p.recipient, StageSearching, "memory")
    case "gmail":
        p.progress.Report(ctx, p.recipient, StageProcessing, "email")
    }
}
```

### Error Recovery Flow

When MCP tools fail or Claude encounters errors, the system immediately pivots to user-friendly error handling:

```go
func (h *ProgressiveAgentHandler) handleMCPError(ctx context.Context, err error, tool string, recipient string) error {
    // Check error type
    switch {
    case isConnectionError(err):
        return h.progress.ReportError(ctx, recipient, err, 
            fmt.Sprintf("I couldn't connect to %s", tool))
    
    case isPermissionError(err):
        return h.progress.ReportError(ctx, recipient, err,
            fmt.Sprintf("I need permission to access %s", tool))
    
    case isTimeoutError(err):
        return h.progress.ReportError(ctx, recipient, err,
            fmt.Sprintf("%s is taking too long to respond", tool))
    
    default:
        // Let Claude generate a natural explanation
        return h.progress.ReportError(ctx, recipient, err, 
            fmt.Sprintf("Issue with %s", tool))
    }
}
```

## Persistence Layer (SQLite)

The system uses SQLite as a robust persistence layer for state management, audit trails, and analytics. SQLite provides ACID guarantees while maintaining simplicity and embedded deployment.

### 8. Persistence Interface

```go
// Storage provides persistent state management across all components
type Storage interface {
    // Message lifecycle
    SaveMessage(msg *StoredMessage) error
    GetMessage(messageID string) (*StoredMessage, error)
    GetConversationHistory(userID string, limit int) ([]*StoredMessage, error)
    
    // Queue persistence
    SaveQueueItem(item *QueuedMessage) error
    UpdateQueueItemState(itemID string, state MessageState, metadata map[string]interface{}) error
    GetPendingQueueItems() ([]*QueuedMessage, error)
    GetQueueItemHistory(itemID string) ([]*StateTransition, error)
    
    // LLM interaction tracking
    SaveLLMCall(call *LLMCall) error
    GetLLMCallsForMessage(messageID string) ([]*LLMCall, error)
    GetLLMCostReport(userID string, start, end time.Time) (*CostReport, error)
    
    // Session management
    SaveSession(session *Session) error
    UpdateSessionActivity(sessionID string, lastActivity time.Time) error
    GetActiveSession(userID string) (*Session, error)
    ExpireSessions(before time.Time) (int, error)
    
    // System state
    SaveRateLimitState(userID string, state *RateLimitState) error
    GetRateLimitState(userID string) (*RateLimitState, error)
    SaveCircuitBreakerState(service string, state *CircuitBreakerState) error
    GetCircuitBreakerState(service string) (*CircuitBreakerState, error)
    
    // Analytics and monitoring
    RecordMetric(metric *SystemMetric) error
    GetMetricsReport(start, end time.Time) (*MetricsReport, error)
    
    // Maintenance
    PurgeOldData(before time.Time, dataType string) (int64, error)
    Vacuum() error
}

// Data structures for persistence
type StoredMessage struct {
    ID              string
    ConversationID  string
    From            string
    To              string
    Content         string
    Direction       MessageDirection // INBOUND, OUTBOUND
    Timestamp       time.Time
    ProcessingState string
    Metadata        json.RawMessage
}

type LLMCall struct {
    ID          string
    MessageID   string
    SessionID   string
    CallType    string // EXECUTION, VALIDATION, RECOVERY
    Prompt      string
    Response    string
    ToolCalls   json.RawMessage
    TokensUsed  int
    Latency     time.Duration
    Cost        float64
    Success     bool
    Error       string
    Timestamp   time.Time
}

type StateTransition struct {
    ID        string
    ItemID    string
    FromState MessageState
    ToState   MessageState
    Reason    string
    Metadata  json.RawMessage
    Timestamp time.Time
}

type RateLimitState struct {
    UserID      string
    Tokens      int
    LastRefill  time.Time
    WindowStart time.Time
    RequestCount int
}

type CircuitBreakerState struct {
    Service        string
    State          string // CLOSED, OPEN, HALF_OPEN
    Failures       int
    LastFailure    time.Time
    LastSuccess    time.Time
    NextRetry      time.Time
}
```

### SQLite Schema Design

```sql
-- Core message storage
CREATE TABLE messages (
    id TEXT PRIMARY KEY,
    conversation_id TEXT NOT NULL,
    from_number TEXT NOT NULL,
    to_number TEXT,
    content TEXT NOT NULL,
    direction TEXT NOT NULL CHECK (direction IN ('INBOUND', 'OUTBOUND')),
    processing_state TEXT,
    metadata TEXT, -- JSON
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_conversation (conversation_id, created_at),
    INDEX idx_from_number (from_number, created_at)
);

-- Queue items with state machine
CREATE TABLE queue_items (
    id TEXT PRIMARY KEY,
    message_id TEXT NOT NULL REFERENCES messages(id),
    conversation_id TEXT NOT NULL,
    state TEXT NOT NULL,
    priority INTEGER NOT NULL DEFAULT 0,
    attempts INTEGER NOT NULL DEFAULT 0,
    last_error TEXT,
    metadata TEXT, -- JSON
    queued_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_state (state, priority, queued_at),
    INDEX idx_conversation_queue (conversation_id, state, queued_at)
);

-- State transition audit log
CREATE TABLE state_transitions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    item_id TEXT NOT NULL REFERENCES queue_items(id),
    from_state TEXT NOT NULL,
    to_state TEXT NOT NULL,
    reason TEXT,
    metadata TEXT, -- JSON
    transitioned_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_item_transitions (item_id, transitioned_at)
);

-- LLM interaction tracking
CREATE TABLE llm_calls (
    id TEXT PRIMARY KEY,
    message_id TEXT NOT NULL REFERENCES messages(id),
    session_id TEXT NOT NULL,
    call_type TEXT NOT NULL, -- EXECUTION, VALIDATION, RECOVERY
    prompt TEXT NOT NULL,
    response TEXT NOT NULL,
    tool_calls TEXT, -- JSON array
    tokens_used INTEGER NOT NULL,
    latency_ms INTEGER NOT NULL,
    cost_cents INTEGER NOT NULL, -- Store as cents to avoid float precision
    success BOOLEAN NOT NULL,
    error TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_message_calls (message_id, created_at),
    INDEX idx_session_calls (session_id, created_at),
    INDEX idx_cost_tracking (created_at, cost_cents)
);

-- Session management
CREATE TABLE sessions (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    started_at TIMESTAMP NOT NULL,
    last_activity TIMESTAMP NOT NULL,
    expires_at TIMESTAMP NOT NULL,
    message_count INTEGER NOT NULL DEFAULT 0,
    metadata TEXT, -- JSON
    INDEX idx_user_sessions (user_id, expires_at),
    INDEX idx_active_sessions (expires_at)
);

-- Rate limiting state
CREATE TABLE rate_limits (
    user_id TEXT PRIMARY KEY,
    tokens INTEGER NOT NULL,
    last_refill TIMESTAMP NOT NULL,
    window_start TIMESTAMP NOT NULL,
    request_count INTEGER NOT NULL,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Circuit breaker state
CREATE TABLE circuit_breakers (
    service TEXT PRIMARY KEY,
    state TEXT NOT NULL CHECK (state IN ('CLOSED', 'OPEN', 'HALF_OPEN')),
    failure_count INTEGER NOT NULL DEFAULT 0,
    last_failure TIMESTAMP,
    last_success TIMESTAMP,
    next_retry TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- System metrics for monitoring
CREATE TABLE metrics (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    metric_name TEXT NOT NULL,
    value REAL NOT NULL,
    tags TEXT, -- JSON
    recorded_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_metric_time (metric_name, recorded_at)
);

-- Create triggers for updated_at
CREATE TRIGGER update_messages_timestamp 
AFTER UPDATE ON messages
BEGIN
    UPDATE messages SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
END;

CREATE TRIGGER update_queue_items_timestamp 
AFTER UPDATE ON queue_items
BEGIN
    UPDATE queue_items SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
END;
```

### Storage Implementation

```go
type SQLiteStorage struct {
    db           *sql.DB
    maxRetries   int
    retryDelay   time.Duration
    queryTimeout time.Duration
}

func NewSQLiteStorage(dbPath string) (*SQLiteStorage, error) {
    // Use WAL mode for better concurrency
    db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
    if err != nil {
        return nil, err
    }
    
    // Set pragmas for performance and reliability
    pragmas := []string{
        "PRAGMA foreign_keys = ON",
        "PRAGMA synchronous = NORMAL",
        "PRAGMA cache_size = -64000", // 64MB cache
        "PRAGMA temp_store = MEMORY",
        "PRAGMA mmap_size = 30000000000", // 30GB mmap
    }
    
    for _, pragma := range pragmas {
        if _, err := db.Exec(pragma); err != nil {
            return nil, fmt.Errorf("failed to set pragma: %w", err)
        }
    }
    
    storage := &SQLiteStorage{
        db:           db,
        maxRetries:   3,
        retryDelay:   50 * time.Millisecond,
        queryTimeout: 5 * time.Second,
    }
    
    if err := storage.runMigrations(); err != nil {
        return nil, fmt.Errorf("failed to run migrations: %w", err)
    }
    
    return storage, nil
}

// Transaction helper with automatic retry on SQLITE_BUSY
func (s *SQLiteStorage) withTx(ctx context.Context, fn func(*sql.Tx) error) error {
    for attempt := 0; attempt < s.maxRetries; attempt++ {
        err := s.doTransaction(ctx, fn)
        if err == nil {
            return nil
        }
        
        // Check if it's a busy error
        if !strings.Contains(err.Error(), "database is locked") {
            return err
        }
        
        // Exponential backoff
        time.Sleep(s.retryDelay * time.Duration(1<<attempt))
    }
    
    return fmt.Errorf("transaction failed after %d attempts", s.maxRetries)
}

// Save complete message lifecycle
func (s *SQLiteStorage) SaveMessageLifecycle(
    msg *StoredMessage,
    queueItem *QueuedMessage,
    llmCalls []*LLMCall,
) error {
    return s.withTx(context.Background(), func(tx *sql.Tx) error {
        // Save message
        if err := s.saveMessageTx(tx, msg); err != nil {
            return err
        }
        
        // Save queue item
        if err := s.saveQueueItemTx(tx, queueItem); err != nil {
            return err
        }
        
        // Save all LLM calls
        for _, call := range llmCalls {
            if err := s.saveLLMCallTx(tx, call); err != nil {
                return err
            }
        }
        
        return nil
    })
}
```

### Integration with Existing Components

```go
// Enhanced QueueManager with persistence
type PersistentQueueManager struct {
    *QueueManager
    storage Storage
}

func (pqm *PersistentQueueManager) Enqueue(msg IncomingMessage) error {
    // Save to database first
    storedMsg := &StoredMessage{
        ID:             uuid.New().String(),
        ConversationID: msg.From,
        From:           msg.From,
        Content:        msg.Text,
        Direction:      INBOUND,
        Timestamp:      msg.Timestamp,
    }
    
    if err := pqm.storage.SaveMessage(storedMsg); err != nil {
        return fmt.Errorf("failed to persist message: %w", err)
    }
    
    // Create queue item
    queuedMsg := &QueuedMessage{
        ID:             uuid.New().String(),
        MessageID:      storedMsg.ID,
        ConversationID: msg.From,
        State:          StateQueued,
        Priority:       PriorityNormal,
        QueuedAt:       time.Now(),
    }
    
    // Save queue item and initial state
    if err := pqm.storage.SaveQueueItem(queuedMsg); err != nil {
        return fmt.Errorf("failed to persist queue item: %w", err)
    }
    
    // Enqueue in memory
    return pqm.QueueManager.Enqueue(msg)
}

// Enhanced Agent Handler with LLM call tracking
type PersistentAgentHandler struct {
    *AgentHandler
    storage Storage
}

func (pah *PersistentAgentHandler) Process(ctx context.Context, msg IncomingMessage) error {
    messageID := msg.ID // Assume message has ID from persistence
    
    // Wrap LLM to track calls
    trackingLLM := &TrackingLLM{
        llm:       pah.llm,
        storage:   pah.storage,
        messageID: messageID,
    }
    
    // Process with tracking
    return pah.AgentHandler.Process(ctx, msg)
}

// LLM wrapper that persists all interactions
type TrackingLLM struct {
    llm       LLM
    storage   Storage
    messageID string
}

func (t *TrackingLLM) Query(ctx context.Context, prompt string, sessionID string) (*LLMResponse, error) {
    start := time.Now()
    
    // Call underlying LLM
    response, err := t.llm.Query(ctx, prompt, sessionID)
    
    // Determine call type from prompt
    callType := t.inferCallType(prompt)
    
    // Save the call
    llmCall := &LLMCall{
        ID:         uuid.New().String(),
        MessageID:  t.messageID,
        SessionID:  sessionID,
        CallType:   callType,
        Prompt:     prompt,
        Response:   response.Message,
        TokensUsed: response.Metadata.TokensUsed,
        Latency:    time.Since(start),
        Cost:       t.calculateCost(response.Metadata.TokensUsed),
        Success:    err == nil,
        Timestamp:  start,
    }
    
    if err != nil {
        llmCall.Error = err.Error()
    }
    
    // Fire and forget - don't fail the request if storage fails
    go t.storage.SaveLLMCall(llmCall)
    
    return response, err
}
```

### Data Lifecycle Management

```go
// DataRetentionManager handles data cleanup
type DataRetentionManager struct {
    storage         Storage
    retentionRules  map[string]time.Duration
}

func NewDataRetentionManager(storage Storage) *DataRetentionManager {
    return &DataRetentionManager{
        storage: storage,
        retentionRules: map[string]time.Duration{
            "messages":     90 * 24 * time.Hour,  // 90 days
            "llm_calls":    30 * 24 * time.Hour,  // 30 days  
            "metrics":      7 * 24 * time.Hour,   // 7 days
            "transitions":  365 * 24 * time.Hour, // 1 year (audit trail)
        },
    }
}

func (drm *DataRetentionManager) RunCleanup(ctx context.Context) error {
    for dataType, retention := range drm.retentionRules {
        cutoff := time.Now().Add(-retention)
        
        deleted, err := drm.storage.PurgeOldData(cutoff, dataType)
        if err != nil {
            log.Printf("Failed to purge %s: %v", dataType, err)
            continue
        }
        
        log.Printf("Purged %d old %s records", deleted, dataType)
    }
    
    // Vacuum to reclaim space
    return drm.storage.Vacuum()
}
```

### Recovery and Startup

```go
// System startup with persistence recovery
func (app *MentatApp) Start(ctx context.Context) error {
    // Recover queue state
    pendingItems, err := app.storage.GetPendingQueueItems()
    if err != nil {
        return fmt.Errorf("failed to recover queue state: %w", err)
    }
    
    log.Printf("Recovering %d pending queue items", len(pendingItems))
    
    // Re-enqueue pending items
    for _, item := range pendingItems {
        // Check if item is stale
        if time.Since(item.QueuedAt) > 30*time.Minute {
            // Mark as failed
            app.storage.UpdateQueueItemState(item.ID, StateFailed, 
                map[string]interface{}{"reason": "stale after restart"})
            continue
        }
        
        // Re-enqueue for processing
        if err := app.queue.EnqueueRecovered(item); err != nil {
            log.Printf("Failed to re-enqueue %s: %v", item.ID, err)
        }
    }
    
    // Recover rate limit states
    // ... similar recovery for other stateful components
    
    return app.startWorkers(ctx)
}
```

## Core Components

### 1. Testable Agent Handler

```go
type AgentHandler struct {
    llm          LLM
    messenger    Messenger
    sessions     SessionManager
    validator    ValidationStrategy
    enhancer     IntentEnhancer
    storage      Storage
    clock        Clock  // For time-based testing
}

// Constructor with dependency injection
func NewAgentHandler(opts ...HandlerOption) *AgentHandler {
    h := &AgentHandler{
        clock: &RealClock{},
    }
    
    for _, opt := range opts {
        opt(h)
    }
    
    // Set defaults for any nil interfaces
    if h.validator == nil {
        h.validator = &MultiAgentValidator{}
    }
    if h.enhancer == nil {
        h.enhancer = NewSmartIntentEnhancer()
    }
    
    return h
}

// Option pattern for clean construction
type HandlerOption func(*AgentHandler)

func WithLLM(llm LLM) HandlerOption {
    return func(h *AgentHandler) { h.llm = llm }
}

func WithMessenger(m Messenger) HandlerOption {
    return func(h *AgentHandler) { h.messenger = m }
}

func WithValidator(v ValidationStrategy) HandlerOption {
    return func(h *AgentHandler) { h.validator = v }
}

func WithStorage(s Storage) HandlerOption {
    return func(h *AgentHandler) { h.storage = s }
}
```

### 2. Signal Handler with Queue Integration

```go
type SignalHandler struct {
    messenger Messenger
    queue     MessageQueue
    logger    StructuredLogger
}

func (h *SignalHandler) Start(ctx context.Context) error {
    // Subscribe to incoming messages
    messages, err := h.messenger.Subscribe(ctx)
    if err != nil {
        return fmt.Errorf("failed to subscribe: %w", err)
    }
    
    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        case msg := <-messages:
            if err := h.handleIncoming(ctx, msg); err != nil {
                h.logger.Error("failed to handle message", err,
                    Field("from", msg.From))
            }
        }
    }
}

func (h *SignalHandler) handleIncoming(ctx context.Context, msg IncomingMessage) error {
    // Enqueue for processing
    if err := h.queue.Enqueue(msg); err != nil {
        // Queue full or rate limited - send user-friendly response
        if errors.Is(err, ErrQueueFull) {
            return h.messenger.Send(ctx, msg.From, 
                "I'm a bit overwhelmed right now. Please try again in a moment.")
        }
        return err
    }
    
    h.logger.Info("message queued",
        Field("from", msg.From),
        Field("length", len(msg.Text)))
    
    return nil
}
```

### 3. Queue Worker Implementation

```go
type QueueWorker struct {
    id           string
    queue        MessageQueue
    handler      *AgentHandler
    messenger    Messenger
    rateLimiter  RateLimiter
    stateMachine StateMachine
}

func (w *QueueWorker) Start(ctx context.Context) error {
    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        default:
            if err := w.processNext(ctx); err != nil {
                if !errors.Is(err, ErrNoMessages) {
                    w.logger.Error("worker error", err, Field("worker", w.id))
                }
                time.Sleep(100 * time.Millisecond)
            }
        }
    }
}

func (w *QueueWorker) processNext(ctx context.Context) error {
    // Get next message
    msg, err := w.queue.GetNext(w.id)
    if err != nil {
        return err
    }
    
    // Check rate limit
    if !w.rateLimiter.Allow(msg.ConversationID) {
        // Re-queue with backoff
        msg.State = StateRetrying
        return w.queue.UpdateState(msg.ID, StateRetrying, "rate limited")
    }
    
    // Update state to processing
    if err := w.queue.UpdateState(msg.ID, StateProcessing, "worker started"); err != nil {
        return err
    }
    
    // Start typing indicator
    typingCtx, cancelTyping := context.WithCancel(ctx)
    go w.maintainTypingIndicator(typingCtx, msg.From)
    
    // Process through agent handler
    inMsg := IncomingMessage{From: msg.From, Text: msg.Text, Timestamp: msg.QueuedAt}
    err = w.handler.Process(ctx, inMsg)
    
    // Stop typing
    cancelTyping()
    
    // Update final state
    if err != nil {
        msg.Attempts++
        msg.LastError = err
        
        if msg.Attempts >= 3 {
            return w.queue.UpdateState(msg.ID, StateFailed, fmt.Sprintf("max attempts: %v", err))
        }
        return w.queue.UpdateState(msg.ID, StateRetrying, fmt.Sprintf("attempt %d: %v", msg.Attempts, err))
    }
    
    return w.queue.UpdateState(msg.ID, StateCompleted, "success")
}

func (w *QueueWorker) maintainTypingIndicator(ctx context.Context, recipient string) {
    ticker := time.NewTicker(10 * time.Second)
    defer ticker.Stop()
    
    w.messenger.SendTypingIndicator(ctx, recipient)
    
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            w.messenger.SendTypingIndicator(ctx, recipient)
        }
    }
}
```

### 4. Multi-Agent Validation Process

```go
// ParallelValidationOrchestrator runs multiple validation strategies concurrently
type ParallelValidationOrchestrator struct {
    strategies []ValidationStrategy
    timeout    time.Duration
}

func (o *ParallelValidationOrchestrator) ValidateParallel(
    ctx context.Context, 
    request, response string, 
    llm LLM,
) ValidationResult {
    ctx, cancel := context.WithTimeout(ctx, o.timeout)
    defer cancel()
    
    results := make(chan ValidationResult, len(o.strategies))
    
    // Launch parallel validations
    for _, strategy := range o.strategies {
        go func(s ValidationStrategy) {
            results <- s.Validate(ctx, request, response, llm)
        }(strategy)
    }
    
    // Collect results and return most conservative
    var collected []ValidationResult
    for i := 0; i < len(o.strategies); i++ {
        select {
        case result := <-results:
            collected = append(collected, result)
            
            // Early exit on critical failures
            if result.Status == FAILED {
                return result
            }
        case <-ctx.Done():
            // Timeout - return best result so far
            return o.bestResult(collected)
        }
    }
    
    return o.bestResult(collected)
}

func (o *ParallelValidationOrchestrator) bestResult(results []ValidationResult) ValidationResult {
    // Return the most conservative (worst) result
    // Priority: FAILED > INCOMPLETE_SEARCH > PARTIAL > UNCLEAR > SUCCESS
    worst := ValidationResult{Status: SUCCESS}
    
    for _, r := range results {
        if r.Status < worst.Status { // Lower enum values = worse outcomes
            worst = r
        }
    }
    
    return worst
}

// Enhanced Process method with parallel validation
func (h *AgentHandler) Process(ctx context.Context, msg IncomingMessage) error {
    sessionID := h.sessions.GetOrCreateSession(msg.From)
    
    // Immediate acknowledgment/response (2-3 seconds)
    // This might be the ONLY response for simple queries
    ackResponse, err := h.getInitialResponse(ctx, msg, sessionID)
    if err != nil {
        return h.handleServiceError(ctx, err, msg.From)
    }
    
    // Send the initial response
    h.messenger.Send(ctx, msg.From, ackResponse.Message)
    
    // Check if we're done (no continuation needed)
    if ackResponse.Progress != nil && !ackResponse.Progress.NeedsContinuation {
        // Complete! No further processing needed
        return nil
    }
    
    // We need to continue - enhance request with gentle guidance
    enhancedRequest := h.enhancer.Enhance(msg.Text)
    
    // Continue with full execution since initial response indicated continuation needed
    execution, err := h.llm.Query(ctx, fmt.Sprintf(`
        %s
        
        After completing this request:
        - Report specific actions taken and tools used
        - Include any IDs, confirmations, or results
        - If you encounter issues, explain them naturally
        - Be transparent about partial successes
        
        IMPORTANT: Your response will be in JSON format with this structure:
        {
            "message": "Your natural language response to the user",
            "progress": {
                "needs_continuation": boolean,
                "continuation_reason": "why you need to continue (if applicable)",
                "next_action": "what you plan to do next (if continuing)",
                "intent_satisfied": boolean,
                "needs_clarification": boolean,
                "clarification_prompt": "what to ask the user (if clarification needed)"
            }
        }
        
        Set needs_continuation to true ONLY if:
        - You need to call more tools to complete the request
        - The current tool call is incomplete
        - You're gathering information across multiple sources
        
        Set needs_continuation to false if:
        - You've completed the user's request
        - You cannot access required tools
        - You need the user to clarify their intent
        - The task is impossible to complete
    `, enhancedRequest), sessionID)
    
    if err != nil {
        // Immediate error feedback to user
        if h.progress != nil {
            return h.progress.ReportError(ctx, msg.From, err, msg.Text)
        }
        return h.handleServiceError(ctx, err, msg.From)
    }
    
    // Check if LLM needs to continue
    if execution.Progress != nil && h.progress != nil {
        if !h.progress.ShouldContinue(execution.Progress) {
            // LLM is done - send final response
            if execution.Progress.NeedsClarification {
                // Include clarification prompt
                finalMsg := fmt.Sprintf("%s\n\n%s", 
                    execution.Message, 
                    execution.Progress.ClarificationPrompt)
                return h.messenger.Send(ctx, msg.From, finalMsg)
            }
            // Simple completion
            return h.messenger.Send(ctx, msg.From, execution.Message)
        }
        
        // LLM needs to continue - start progress tracking
        go h.continueWithProgress(ctx, msg, execution, sessionID)
    } else {
        // Fallback to original flow if no progress info
        h.messenger.Send(ctx, msg.From, execution.Message)
        go h.validateInBackground(ctx, msg, execution, sessionID)
    }
    
    return nil
}

func (h *AgentHandler) getInitialResponse(ctx context.Context, msg IncomingMessage, sessionID string) (*LLMResponse, error) {
    // Generate initial response in 2-3 seconds
    // This might be the complete response for simple queries!
    initialPrompt := fmt.Sprintf(`
        The user said: "%s"
        
        Respond naturally and conversationally. For simple questions or chat, 
        provide the complete answer. For complex requests that need tools,
        acknowledge and indicate you'll need to look things up.
        
        IMPORTANT: Your response will be in JSON format with this structure:
        {
            "message": "Your natural language response to the user",
            "progress": {
                "needs_continuation": boolean,
                "continuation_reason": "why you need to continue (if applicable)",
                "next_action": "what you plan to do next (if continuing)",
                "intent_satisfied": boolean,
                "needs_clarification": boolean,
                "clarification_prompt": "what to ask the user (if clarification needed)"
            }
        }
        
        Set needs_continuation to false if:
        - This is a simple question you can answer without tools
        - This is casual conversation
        - You need clarification from the user
        - The request is impossible to fulfill
        
        Set needs_continuation to true ONLY if:
        - You need to use tools (calendar, memory, etc)
        - You need to look up information
        - Multiple steps are required
    `, msg.Text)
    
    // Use short timeout for quick response
    ackCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
    defer cancel()
    
    resp, err := h.llm.Query(ackCtx, initialPrompt, sessionID)
    if err != nil {
        // Return error to be handled by caller
        return nil, err
    }
    
    return resp, nil
}

func (h *AgentHandler) validateAndCorrect(
    ctx context.Context, 
    msg IncomingMessage, 
    execution *LLMResponse,
    sessionID string,
) {
    // Give the user time to read the response before validation
    time.Sleep(2 * time.Second)
    
    // Create a background context that won't be cancelled by the main request
    bgCtx := context.Background()
    
    // Run validation asynchronously
    validation := h.validator.Validate(bgCtx, msg.Text, execution.Message, sessionID, h.llm)
    
    // Only send follow-up if validation found issues
    switch validation.Status {
    case SUCCESS:
        // Original response was correct, no action needed
        return
        
    case INCOMPLETE_SEARCH:
        // Claude didn't use all relevant tools
        followUp := fmt.Sprintf(
            "Oops! I just realized I didn't check all available information sources. "+
            "Let me look at %s to give you a more complete answer...",
            validation.Metadata["missing_tools"])
        
        h.messenger.Send(bgCtx, msg.From, followUp)
        
        // Get additional information
        thoroughRetry, err := h.llm.Query(bgCtx, fmt.Sprintf(`
            The user asked: "%s"
            
            You provided an initial response but missed checking: %s
            Please use those tools now and provide the additional information found.
            Start your response with what new information you discovered.
        `, msg.Text, validation.Metadata["missing_tools"]), sessionID)
        
        if err == nil {
            h.messenger.Send(bgCtx, msg.From, thoroughRetry.Message)
        }
        
    case PARTIAL, FAILED:
        // Response had errors or was incomplete
        h.messenger.Send(bgCtx, msg.From, 
            "Wait, I need to correct something about my previous response...")
        
        // Generate natural correction
        recovery := h.validator.GenerateRecovery(bgCtx, msg.Text, execution.Message, validation, h.llm)
        if recovery != "" {
            h.messenger.Send(bgCtx, msg.From, recovery)
        }
    }
}
```

## Async Validation Architecture

### Validation Decision Flow

The system uses Claude's self-awareness to determine when validation is necessary:

```go
// Claude indicates validation need in progress JSON
{
  "progress": {
    "needs_continuation": false,
    "status": "complete",
    "needs_validation": true,  // Set when tools were used or complex operations performed
    "message": "Created calendar event and sent invitation"
  }
}
```

### When Validation is Needed

Claude sets `needs_validation: true` for:
- **Tool usage**: Any MCP tool invocation (calendar, email, memory, etc.)
- **Multi-step operations**: Complex requests requiring multiple actions
- **Scheduling operations**: Creating/modifying calendar events
- **Data modifications**: Adding memories, creating tasks, sending emails
- **Uncertain completions**: When Claude isn't confident about success

Claude sets `needs_validation: false` for:
- **Simple greetings**: "Hi, what's your name?"
- **Information queries**: "What can you help me with?"
- **Chat conversations**: General discussion without tool usage
- **Status checks**: "How are you doing?"
- **Clarification requests**: Asking for more information

### Async Validation Workflow

```go
func (h *AgentHandler) Process(ctx context.Context, msg IncomingMessage) error {
    // 1. Execute request and get initial response
    response, err := h.llm.Query(ctx, msg.Text, sessionID)
    if err != nil {
        return h.handleError(ctx, msg, err)
    }
    
    // 2. Send response immediately to user
    h.messenger.Send(ctx, msg.From, response.Message)
    
    // 4. Check if validation is needed
    if response.Progress != nil && response.Progress.NeedsValidation {
        // Launch async validation - doesn't block user response
        go h.validateAndCorrect(ctx, msg, response, sessionID)
    }
    
    return nil
}
```

### JSON Response Format

Claude returns a properly structured JSON response:

```go
// Example Claude response:
{
    "message": "I found your calendar and you have 3 meetings today. Let me get the details...",
    "progress": {
        "needs_continuation": true,
        "continuation_reason": "Need to fetch meeting details",
        "next_action": "Query calendar for specific event information",
        "intent_satisfied": false,
        "needs_clarification": false
    },
    "tool_calls": [
        {
            "tool": "google-calendar",
            "action": "list_events",
            "timestamp": "2024-01-15T10:30:00Z"
        }
    ]
}
```

This approach eliminates the need for parsing embedded JSON from the message text.

### Validation Correction Messages

When validation detects issues, natural follow-up messages are sent:

```yaml
incomplete_search:
  template: "Oops! I just realized I didn't check {missing_tool}. Let me look at that for you..."
  action: query_with_specific_tool
  
partial_success:
  template: "Wait, I need to correct something about my previous response..."
  action: send_correction
  
tool_failure:
  template: "I noticed {tool} didn't respond properly. Here's what I found instead..."
  action: alternative_approach
```

## Testing Architecture

### 1. Scriptable Test LLM

```go
// ScriptedLLM for deterministic testing
type ScriptedLLM struct {
    responses []ScriptedResponse
    callCount int
    mu        sync.Mutex
}

type ScriptedResponse struct {
    ForPromptContaining string
    Response            *LLMResponse
    Error               error
    Delay               time.Duration
}

func (s *ScriptedLLM) Query(ctx context.Context, prompt string, sessionID string) (*LLMResponse, error) {
    s.mu.Lock()
    defer s.mu.Unlock()
    
    if s.callCount >= len(s.responses) {
        return nil, errors.New("unexpected LLM call")
    }
    
    resp := s.responses[s.callCount]
    s.callCount++
    
    // Simulate processing time
    if resp.Delay > 0 {
        select {
        case <-time.After(resp.Delay):
        case <-ctx.Done():
            return nil, ctx.Err()
        }
    }
    
    // Progress info is already parsed from JSON response
    // No need to extract from message text
    
    return resp.Response, resp.Error
}

// extractProgress is no longer needed as progress info comes in structured JSON
```

### 2. Conversation Test Harness

```go
type ConversationTestHarness struct {
    handler  *AgentHandler
    llm      *ScriptedLLM
    messages *MockMessenger
    clock    *MockClock
}

func (h *ConversationTestHarness) SimulateConversation(t *testing.T, turns []ConversationTurn) {
    for i, turn := range turns {
        // Set up expected LLM responses
        h.llm.SetResponses(turn.ExpectedLLMCalls)
        
        // Advance time if specified
        if turn.TimeAdvance > 0 {
            h.clock.Advance(turn.TimeAdvance)
        }
        
        // Send message
        msg := IncomingMessage{
            From:      turn.From,
            Text:      turn.Message,
            Timestamp: h.clock.Now(),
        }
        
        err := h.handler.Process(context.Background(), msg)
        
        // Verify expectations
        if turn.ExpectError {
            assert.Error(t, err, "Turn %d should have errored", i)
        } else {
            assert.NoError(t, err, "Turn %d should not have errored", i)
        }
        
        // Custom assertions
        if turn.Verify != nil {
            turn.Verify(t, h)
        }
    }
}
```

### 3. Complex Scenario Testing

```go
func TestComplexSchedulingWithPartialFailure(t *testing.T) {
    // Setup
    llm := &ScriptedLLM{
        responses: []ScriptedResponse{
            // First call: Execution attempt
            {
                ForPromptContaining: "Schedule meeting with Sarah",
                Response: &LLMResponse{
                    Message: "I found Sarah's contact info and checked your calendar. I see you're free Wednesday at 2pm, but I couldn't create the event due to a calendar permission issue.",
                },
            },
            // Second call: Validation
            {
                ForPromptContaining: "Was the request fully completed?",
                Response: &LLMResponse{Message: "PARTIAL"},
            },
            // Third call: Recovery explanation
            {
                ForPromptContaining: "Have a natural conversation about",
                Response: &LLMResponse{
                    Message: "I found Sarah's email (sarah@example.com) and confirmed you're free Wednesday at 2pm. However, I couldn't create the calendar event - it seems I need updated permissions. Would you like me to send Sarah an email instead to propose the meeting time?",
                },
            },
        },
    }
    
    messenger := &MockMessenger{}
    sessions := NewInMemorySessionManager()
    handler := NewAgentHandler(
        WithLLM(llm),
        WithMessenger(messenger),
        WithSessions(sessions),
    )
    
    // Execute
    msg := IncomingMessage{
        From: "+1234567890",
        Text: "Schedule meeting with Sarah tomorrow at 2pm",
    }
    
    err := handler.Process(context.Background(), msg)
    
    // Verify
    require.NoError(t, err)
    assert.Equal(t, 3, llm.callCount)
    assert.Equal(t, 1, len(messenger.SentMessages))
    assert.Contains(t, messenger.SentMessages[0].Text, "couldn't create the calendar event")
    assert.Contains(t, messenger.SentMessages[0].Text, "send Sarah an email instead")
}

func TestIncompletSearchRetry(t *testing.T) {
    // Test when Claude misses checking memory for person info
    llm := &ScriptedLLM{
        responses: []ScriptedResponse{
            // First attempt - only checks contacts
            {
                ForPromptContaining: "What's Sarah's phone number",
                Response: &LLMResponse{
                    Message: "Sarah's phone number is (555) 123-4567.",
                },
            },
            // Validation detects incomplete search
            {
                ForPromptContaining: "Did you check all relevant information sources?",
                Response: &LLMResponse{Message: "INCOMPLETE_SEARCH"},
            },
            // Retry with more thorough search
            {
                ForPromptContaining: "additional tools could provide more complete information",
                Response: &LLMResponse{
                    Message: "I found Sarah's information:\n- Phone: (555) 123-4567 (from contacts)\n- Email: sarah@example.com\n- Note from memory: Prefers texts after 6pm, allergic to shellfish\n- Last contact: Discussed project timeline last Tuesday",
                },
            },
            // Final validation confirms thoroughness
            {
                ForPromptContaining: "Was the request fully completed?",
                Response: &LLMResponse{Message: "SUCCESS"},
            },
        },
    }
    
    handler := NewAgentHandler(WithLLM(llm))
    msg := IncomingMessage{
        From: "+1234567890", 
        Text: "What's Sarah's phone number?",
    }
    
    err := handler.Process(context.Background(), msg)
    
    require.NoError(t, err)
    assert.Equal(t, 4, llm.callCount) // Initial + validation + retry + final validation
    assert.Contains(t, handler.LastResponse(), "Prefers texts after 6pm")
}
```

### 4. Test Scenario Builder

```go
// Fluent API for building test scenarios
func TestSchedulingScenario(t *testing.T) {
    NewScenario("Complex scheduling with retry").
        WithMessage("+123", "Schedule lunch with Alice next week").
        ExpectingLLMCall("Schedule lunch", "Checking calendar and finding Alice's contact...").
        ExpectingValidation("PARTIAL").
        ExpectingRecovery("I found several time slots but need to know your preference").
        WithMessage("+123", "Tuesday at noon would be great").
        ExpectingLLMCall("Tuesday at noon", "Perfect! I've scheduled lunch with Alice for Tuesday at noon.").
        ExpectingValidation("SUCCESS").
        ShouldResultIn(func(t *testing.T, result *TestResult) {
            assert.Equal(t, 2, result.ConversationTurns)
            assert.Equal(t, 5, result.TotalLLMCalls)  // 3 for first turn, 2 for second
            assert.Contains(t, result.FinalMessage, "scheduled lunch with Alice")
        }).
        Run(t)
}
```

### 5. Queue Testing Patterns

```go
// Mock queue for testing
type MockQueue struct {
    messages []QueuedMessage
    mu       sync.Mutex
    blocked  bool
}

func (m *MockQueue) Enqueue(msg IncomingMessage) error {
    m.mu.Lock()
    defer m.mu.Unlock()
    
    if m.blocked {
        return ErrQueueFull
    }
    
    queuedMsg := QueuedMessage{
        ID:             uuid.New().String(),
        ConversationID: msg.From,
        From:           msg.From,
        Text:           msg.Text,
        State:          StateQueued,
        QueuedAt:       time.Now(),
    }
    
    m.messages = append(m.messages, queuedMsg)
    return nil
}

// Test queue behavior under load
func TestQueueOverload(t *testing.T) {
    queue := NewQueueManager(QueueConfig{
        MaxDepth:       10,
        MaxConcurrent:  2,
        WorkerCount:    3,
    })
    
    // Simulate rapid messages from same user
    user := "+1234567890"
    for i := 0; i < 15; i++ {
        msg := IncomingMessage{
            From: user,
            Text: fmt.Sprintf("Message %d", i),
        }
        
        err := queue.Enqueue(msg)
        if i < 10 {
            assert.NoError(t, err, "First 10 messages should succeed")
        } else {
            assert.ErrorIs(t, err, ErrQueueFull, "Queue should be full after 10")
        }
    }
    
    // Verify queue stats
    stats := queue.Stats()
    assert.Equal(t, 10, stats.TotalQueued)
    assert.Equal(t, 1, stats.ConversationCount)
}

// Test conversation ordering
func TestConversationOrdering(t *testing.T) {
    queue := NewQueueManager(QueueConfig{
        MaxConcurrent: 1, // Force sequential processing
    })
    
    worker := &MockWorker{
        processedOrder: []string{},
    }
    
    // Enqueue messages from same conversation
    for i := 0; i < 3; i++ {
        queue.Enqueue(IncomingMessage{
            From: "+123",
            Text: fmt.Sprintf("Message %d", i),
        })
    }
    
    // Process all messages
    ctx := context.Background()
    worker.ProcessAll(ctx, queue)
    
    // Verify order preserved
    assert.Equal(t, []string{"Message 0", "Message 1", "Message 2"}, 
        worker.processedOrder)
}

// Test rate limiting
func TestRateLimiting(t *testing.T) {
    limiter := NewRateLimiter(RateLimiterConfig{
        BurstSize:    3,
        RefillRate:   time.Minute,
    })
    
    convID := "conv-123"
    
    // First 3 should succeed
    for i := 0; i < 3; i++ {
        assert.True(t, limiter.Allow(convID))
    }
    
    // 4th should fail
    assert.False(t, limiter.Allow(convID))
    
    // Simulate time passing
    mockClock.Advance(time.Minute)
    
    // Should allow again
    assert.True(t, limiter.Allow(convID))
}

// Test state machine transitions
func TestStateMachine(t *testing.T) {
    sm := NewStateMachine()
    msg := &QueuedMessage{State: StateQueued}
    
    // Valid transitions
    assert.NoError(t, sm.Transition(msg, StateProcessing, "worker claimed"))
    assert.Equal(t, StateProcessing, msg.State)
    
    assert.NoError(t, sm.Transition(msg, StateValidating, "processing complete"))
    assert.Equal(t, StateValidating, msg.State)
    
    assert.NoError(t, sm.Transition(msg, StateCompleted, "validation passed"))
    assert.Equal(t, StateCompleted, msg.State)
    
    // Invalid transition from terminal state
    err := sm.Transition(msg, StateProcessing, "invalid")
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "invalid transition")
    
    // Verify history
    assert.Len(t, msg.StateHistory, 3)
    assert.Equal(t, StateQueued, msg.StateHistory[0].From)
    assert.Equal(t, StateProcessing, msg.StateHistory[0].To)
}
```

## System Prompt Engineering

Critical for reliable tool use and efficient processing:

```markdown
# Mentat System Prompt

You are Mentat, a personal assistant with access to various tools via MCP.

## Critical Rules for Tool Usage:

1. **NEVER claim success without using the appropriate tool**
   - Scheduling requires google-calendar tool
   - Sending emails requires gmail tool  
   - Remembering information requires memory tool
   - Finding contacts requires contacts tool

2. **When you cannot access a required tool, be explicit**:
   - "I need to schedule this but cannot access the calendar tool"
   - "I tried to save this to memory but the tool isn't responding"

3. **After using tools, confirm with specific details**:
   - ✓ "I've scheduled 'Meeting with Sarah' for Wednesday 2pm (ID: evt_123)"
   - ✗ "Done!" (too vague)
   - ✗ "I'll schedule that" (implies future action)

4. **For multi-step requests, report progress**:
   - "Found Sarah's email: sarah@example.com"
   - "Checking your calendar for Wednesday..."
   - "Created event at 2pm with Sarah as attendee"

5. **Be transparent about failures and partial successes**:
   - "I found Sarah's contact but couldn't access your calendar"
   - "The event is created but I couldn't send the invitation"

## Tool Usage Best Practices:

1. **For people/contacts**: Always check BOTH memory and contacts tools
   - Memory may have preferences, notes, or personal context
   - Contacts may have current phone/email information
   - Combine both sources for a complete picture

2. **For scheduling**: Gather full context before creating events
   - Check calendar for availability
   - Check memory for scheduling preferences
   - Check contacts for attendee information

3. **Be thorough**: Better to check multiple sources than miss information
   - Users value complete answers over fast answers
   - If unsure where information might be, check multiple tools
   - Report which sources you checked

4. **Intelligent tool selection**: Use your judgment about which tools to consult
   - Not every request needs every tool
   - But when in doubt, be comprehensive
   - Learn from patterns about what works best

## Memory Usage:

- Always check memory for context about people and preferences
- Save important information from conversations
- Use memory to track patterns and relationships
- Remember that memory complements other tools, not replaces them

## Response Efficiency:

**ALWAYS include the progress JSON structure in EVERY response** to indicate:
- Whether you need to continue processing
- Why continuation is needed (if applicable)
- What your next action will be (if continuing)
- Whether the user's intent is satisfied

This allows the system to:
- Complete simple queries in 2-3 seconds
- Avoid unnecessary processing
- Provide immediate value to users
- Scale processing based on request complexity
```

## MCP Server Deployment

MCP servers run as HTTP services using podman containers, managed by NixOS:

### Container Services

```nix
# In the flake's NixOS module
virtualisation.oci-containers = {
  backend = "podman";
  containers = {
    mcp-google-calendar = {
      image = "ghcr.io/nspady/google-calendar-mcp:latest";
      ports = ["127.0.0.1:3000:3000"];
      environment = {
        TRANSPORT = "http";
        HOST = "0.0.0.0";
        PORT = "3000";
      };
      volumes = [
        "/home/assistant/.config/mentat/google-credentials.json:/creds/google.json:ro"
      ];
      extraOptions = ["--restart=always"];
    };
    
    mcp-google-contacts = {
      image = "ghcr.io/rayanzaki/mcp-google-contacts-server:latest";
      ports = ["127.0.0.1:3001:3001"];
      environment = {
        TRANSPORT = "http";
        HOST = "0.0.0.0";
        PORT = "3001";
      };
      volumes = [
        "/home/assistant/.config/mentat/google-credentials.json:/creds/google.json:ro"
      ];
    };
    
    mcp-gmail = {
      image = "ghcr.io/gongrzhe/gmail-mcp-server:latest";
      ports = ["127.0.0.1:3002:3002"];
      environment = {
        TRANSPORT = "http";
        HOST = "0.0.0.0";
        PORT = "3002";
      };
      volumes = [
        "/home/assistant/.config/mentat/gmail-credentials.json:/creds/gmail.json:ro"
      ];
    };
    
    mcp-todoist = {
      image = "ghcr.io/doist/todoist-mcp:latest";
      ports = ["127.0.0.1:3003:3003"];
      environment = {
        TRANSPORT = "http";
        HOST = "0.0.0.0";
        PORT = "3003";
        TODOIST_API_KEY_FILE = "/creds/todoist-key";
      };
      volumes = [
        "/home/assistant/.config/mentat/todoist-api-key:/creds/todoist-key:ro"
      ];
    };
    
    mcp-memory = {
      # Build custom image since official may not support HTTP
      imageFile = pkgs.dockerTools.buildImage {
        name = "mcp-memory";
        tag = "latest";
        contents = [
          pkgs.nodejs_20
          (pkgs.writeTextDir "app/server.js" ''
            // HTTP wrapper for memory MCP server
            const { createServer } = require('@modelcontextprotocol/server-memory');
            // ... implementation
          '')
        ];
      };
      ports = ["127.0.0.1:3004:3004"];
      environment = {
        MEMORY_PATH = "/data/memory.json";
      };
      volumes = [
        "/var/lib/mentat/memory:/data"
      ];
    };
    
    mcp-expensify = {
      image = "ghcr.io/expensify/mcp-expensify:latest";
      ports = ["127.0.0.1:3005:3005"];
      environment = {
        TRANSPORT = "http";
        HOST = "0.0.0.0";
        PORT = "3005";
      };
      volumes = [
        "/home/assistant/.config/mentat/expensify-credentials.json:/creds/expensify.json:ro"
      ];
      extraOptions = ["--restart=always"];
    };
  };
};
```

### Claude Code Configuration

```nix
# Generate Claude's MCP configuration
environment.etc."mentat/claude-mcp-config.json".text = builtins.toJSON {
  version = "1.0";
  servers = {
    calendar = {
      transport = "http";
      url = "http://localhost:3000";
      name = "Google Calendar";
    };
    contacts = {
      transport = "http";
      url = "http://localhost:3001";
      name = "Google Contacts";
    };
    gmail = {
      transport = "http";
      url = "http://localhost:3002";
      name = "Gmail";
    };
    todoist = {
      transport = "http";
      url = "http://localhost:3003";
      name = "Todoist";
    };
    memory = {
      transport = "http";
      url = "http://localhost:3004";
      name = "Memory";
    };
    expensify = {
      transport = "http";
      url = "http://localhost:3005";
      name = "Expensify";
    };
  };
};

# Point Claude to use this configuration
systemd.services.mentat.environment = {
  CLAUDE_MCP_CONFIG = "/etc/mentat/claude-mcp-config.json";
};
```

### Secret Management

```bash
# User setup script
mkdir -p ~/.config/mentat

# Google OAuth credentials (for Calendar, Contacts, Gmail)
# Download from https://console.cloud.google.com/apis/credentials
cp ~/Downloads/client_secret_*.json ~/.config/mentat/google-credentials.json

# Gmail might need separate credentials
cp ~/Downloads/gmail_client_secret.json ~/.config/mentat/gmail-credentials.json

# Todoist API key from https://todoist.com/app/settings/integrations
echo "your-todoist-api-key" > ~/.config/mentat/todoist-api-key

# Expensify credentials
cp ~/Downloads/expensify_credentials.json ~/.config/mentat/expensify-credentials.json

# Ensure proper permissions
chmod 600 ~/.config/mentat/*
```

### Health Monitoring

```nix
# Systemd health checks for MCP services
systemd.services.mcp-health-check = {
  description = "MCP Services Health Check";
  after = [ "podman.service" ];
  
  serviceConfig = {
    Type = "oneshot";
    ExecStart = pkgs.writeShellScript "check-mcp-health" ''
      for port in 3000 3001 3002 3003 3004 3005; do
        if ! ${pkgs.curl}/bin/curl -sf http://localhost:$port/health; then
          echo "MCP service on port $port is not healthy"
          exit 1
        fi
      done
    '';
  };
};

systemd.timers.mcp-health-check = {
  wantedBy = [ "timers.target" ];
  timerConfig = {
    OnBootSec = "5min";
    OnUnitActiveSec = "5min";
  };
};
```

## Configuration

```yaml
# config/config.yaml
app:
  owner_phone: "+1234567890"
  allowed_numbers:
    - "+1234567890"
    
signal:
  socket: "/var/run/signal-cli/socket"
  typing_refresh_interval: 10s
  
claude:
  command: "claude"
  mcp_config_path: "./config/mcp-config.json"
  system_prompt_path: "./config/system-prompt.md"
  max_retries: 3
  timeout: 30s
  
conversation:
  session_window: 5m
  cleanup_interval: 30m
  
validation:
  enable_multi_agent: true
  max_validation_calls: 4
  
scheduler:
  timezone: "America/New_York"
  
queue:
  max_depth_per_conversation: 10
  max_concurrent_processing: 3
  worker_count: 3
  rate_limit:
    burst_size: 5
    refill_interval: 1m
  retry:
    max_attempts: 3
    backoff_base: 2s

storage:
  type: "sqlite"
  path: "/var/lib/mentat/mentat.db"
  wal_mode: true
  busy_timeout: 5000
  cache_size_mb: 64
  retention:
    messages_days: 90
    llm_calls_days: 30
    metrics_days: 7
    audit_logs_days: 365
  cleanup_schedule: "0 3 * * *"  # Daily at 3 AM
```

## Implementation Timeline

### Week 1: Foundation
1. **Day 1-2**: Core interfaces, Storage layer with SQLite, and migrations
2. **Day 3**: Signal-cli JSON-RPC integration with persistence
3. **Day 4-5**: Message queue with state machine, rate limiting, and SQLite backing
4. **Day 6**: Basic Claude Code wrapper with LLM call tracking
5. **Day 7**: Queue and storage integration testing

### Week 2: Intelligence Layer  
1. **Day 1-2**: Multi-agent validation framework with test harness
2. **Day 3-4**: Complex request detection and intent enhancement
3. **Day 5-6**: Conversation continuity with comprehensive tests
4. **Day 7**: MCP configuration and integration testing

### Week 3: Production Ready
1. **Day 1-2**: Proactive scheduler with validated responses
2. **Day 3-4**: End-to-end testing and load testing
3. **Day 5-6**: NixOS packaging and deployment
4. **Day 7**: Production monitoring and alerting setup

## Project Structure

```
cmd/mentat/
    main.go              # Wire everything together

internal/
    signal/
        client.go        # JSON-RPC client implementation
        handler.go       # Message handling + typing indicators
        types.go         # Signal message types
        interfaces.go    # Messenger interface definition
    
    claude/
        client.go        # Claude CLI wrapper
        types.go         # Request/response types
        interfaces.go    # LLM interface definition
    
    agent/
        handler.go       # Multi-agent orchestration
        validator.go     # Validation strategies
        enhancer.go      # Intent enhancement logic
        complex.go       # Complex request handling
        interfaces.go    # Core handler interfaces
    
    conversation/
        manager.go       # Session continuity
        cleanup.go       # Expired session cleanup
        interfaces.go    # SessionManager interface
    
    queue/
        manager.go       # Queue orchestration
        worker.go        # Worker pool implementation
        state.go         # State machine logic
        limiter.go       # Rate limiting per conversation
        interfaces.go    # Queue interfaces
    
    storage/
        sqlite.go        # SQLite implementation
        migrations.go    # Schema migrations
        tracking.go      # LLM call tracking wrapper
        retention.go     # Data retention management
        recovery.go      # Startup recovery logic
        interfaces.go    # Storage interface definition
    
    scheduler/
        scheduler.go     # Cron job management
        jobs/
            briefing.go  # Morning briefing
            patterns.go  # Pattern analysis
            reminders.go # Proactive reminders
            cleanup.go   # Data retention cleanup
    
    config/
        config.go        # Configuration management
        mcp.go          # MCP config handling
    
    testing/
        mocks.go        # Common test mocks
        harness.go      # Test harness utilities
        builders.go     # Scenario builders

config/
    config.yaml          # Main configuration
    system-prompt.md     # Claude system prompt

# Generated by Nix
/etc/mentat/
    claude-mcp-config.json  # Claude Code MCP configuration

# User-managed secrets
~/.config/mentat/
    google-credentials.json  # Google OAuth credentials
    gmail-credentials.json   # Gmail-specific credentials
    todoist-api-key         # Todoist API key
    expensify-credentials.json  # Expensify API credentials

# Runtime data (created by application)
/var/lib/mentat/
    mentat.db               # SQLite database with WAL files
    mentat.db-wal           # Write-ahead log
    mentat.db-shm           # Shared memory file

tests/
    integration/
        scheduling_test.go    # End-to-end scheduling tests
        conversation_test.go  # Multi-turn conversation tests
        validation_test.go    # Validation strategy tests
    scenarios/
        complex_requests.go   # Complex scenario definitions
        failure_modes.go      # Various failure scenarios

scripts/
    setup-signal.sh      # Signal-cli setup helper
    test-integration.sh  # Integration test runner
```

## Key Architectural Decisions

1. **Interface-driven design**: All external dependencies behind interfaces for complete testability

2. **Multi-agent validation over parsing**: Use Claude's intelligence to verify Claude's actions

3. **Smart delegation over micromanagement**: Provide gentle hints about thoroughness while trusting Claude's tool selection

4. **Thoroughness validation**: Check not just success but also completeness of information gathering

5. **Natural conversation over templates**: Every user-facing message generated by Claude based on context

6. **Session continuity**: 5-minute window balances context preservation with avoiding confusion

7. **Typing indicators**: Maintain user engagement during longer Claude processing

8. **Graceful degradation**: If validation fails, send the original response rather than error messages

9. **Comprehensive testing**: Every component testable with scripted scenarios and deterministic outcomes

10. **Conversation-aware queuing**: Messages process sequentially within conversations while allowing parallel processing across conversations

11. **State machine for reliability**: Every message tracks its state from queued through completion with automatic retries

12. **Rate limiting protection**: Multi-level rate limiting prevents system overload while maintaining fair access

13. **MCP servers as HTTP services**: Run MCP servers as always-on HTTP services via podman to eliminate startup overhead

14. **Container-based MCP deployment**: Use podman containers for MCP servers to avoid complex Node.js dependency management

15. **SQLite for persistence**: Embedded database provides ACID guarantees with zero operational overhead, perfect for single-instance deployment

16. **Comprehensive audit trail**: Every LLM interaction, state transition, and system decision is persisted for debugging and compliance

17. **Progressive response streaming**: Users receive immediate feedback with incremental updates during long operations, maintaining engagement throughout the 15-20 second validation process

18. **Parallel validation strategies**: Multiple validation approaches run concurrently with fast-fail semantics, reducing validation latency while maintaining thoroughness

19. **Immediate error recovery**: MCP and system errors trigger immediate LLM-generated explanations rather than exposing internal error messages to users

20. **Optimistic response pattern**: Initial responses sent immediately with background validation and async corrections, balancing speed with accuracy

21. **Structured progress tracking**: LLM responses include JSON metadata indicating whether continuation is needed, avoiding unnecessary processing when requests are complete

22. **Intent-aware completion**: System understands when user intent is satisfied, tool access has failed, or clarification is needed, terminating processing appropriately

23. **Smart initial response**: First LLM call determines if continuation is needed, enabling 2-3 second completion for simple queries while maintaining full capability for complex requests

24. **Self-aware validation needs**: Claude indicates in progress JSON whether validation is required, eliminating unnecessary validation for simple queries like greetings

25. **Async validation corrections**: Validation runs after user receives response, with natural follow-up messages ("Oops! I just realized...") only when issues are detected

26. **Structured JSON responses**: Claude returns properly formatted JSON responses with separate message and progress fields, eliminating the need for parsing embedded JSON

## Future Enhancements

- Voice integration via Home Assistant
- Web dashboard for conversation history
- Multi-user support with access controls
- Automatic learning of user preferences
- Integration with more MCP servers as they become available
- Performance optimization based on real usage patterns

## Success Metrics

- **Reliability**: < 1% hallucinated success rate
- **Response time**: < 10s for standard requests
- **Queue latency**: < 500ms from enqueue to processing start
- **Message ordering**: 100% in-order delivery within conversations
- **Retry success**: > 95% success rate within 3 attempts
- **Availability**: 99.9% uptime
- **User trust**: No manual verification needed after first week
- **Proactive value**: > 5 useful proactive messages per week
- **Test coverage**: > 80% code coverage with comprehensive scenario tests