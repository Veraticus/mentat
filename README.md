# Mentat - AI Personal Assistant via Signal

Mentat is a personal assistant bot that brings Claude's capabilities to your Signal messenger. It combines Claude Code's intelligence with MCP (Model Context Protocol) servers to provide a powerful, privacy-focused AI assistant that can manage your calendar, emails, tasks, and more - all through natural conversation.

## What is Mentat?

Mentat acts as a thin orchestration layer that:
- **Connects Signal to Claude**: Receive and respond to messages via Signal messenger
- **Manages your digital life**: Access calendar, email, tasks, contacts through MCP servers
- **Validates its own work**: Uses multi-agent validation to ensure reliable responses
- **Works proactively**: Sends morning briefings and reminders via scheduled jobs
- **Respects your privacy**: Runs entirely on your infrastructure

## Key Features

### 🤖 Natural Conversation
- Chat with Claude through Signal like messaging a friend
- All responses are generated naturally by Claude
- Maintains conversation context within 5-minute windows
- Shows typing indicators during processing

### 📅 Integrated Services (via MCP)
- **Google Calendar**: Check schedule, create events, find free time
- **Gmail**: Read emails, search messages, send responses  
- **Google Contacts**: Look up contact information
- **Todoist**: Manage tasks and projects
- **Expensify**: Submit expenses and receipts
- **Memory**: Persistent conversation memory across sessions

### 🛡️ Reliable Execution
- **Multi-agent validation**: Claude validates Claude's responses to catch errors
- **Queue-based architecture**: Handles multiple conversations without dropping messages
- **Rate limiting**: Prevents overload while maintaining fairness
- **Graceful degradation**: Continues working even if some MCP servers are down

### ⏰ Proactive Assistance
- **Morning briefings**: Daily summary of calendar, weather, and tasks
- **Smart reminders**: Context-aware notifications throughout the day
- **Scheduled reports**: Weekly expense summaries, meeting prep, etc.

## Architecture Overview

```
Signal Message → Queue → Worker → Claude → MCP Servers → Response
                           ↓
                     Validation
                           ↓
                     Corrections
```

1. **Signal Integration**: Receives messages via signal-cli JSON-RPC
2. **Message Queue**: Ensures reliable processing with per-conversation ordering
3. **Worker Pool**: Processes messages with configurable concurrency
4. **Claude Integration**: Queries Claude with full MCP context
5. **Async Validation**: Validates responses in background, sends corrections if needed
6. **Response Delivery**: Sends natural language responses back via Signal

## Technical Highlights

- **Language**: Go (for reliability and performance)
- **AI Brain**: Claude Code via SDK/CLI
- **Message Protocol**: Signal messenger via signal-cli
- **Data Access**: MCP servers running in containers
- **Database**: SQLite for message history and state
- **Deployment**: NixOS module with systemd service

## Getting Started

### Prerequisites
- Go 1.23+
- signal-cli configured with your phone number
- Claude Code installed (`pip install claude-code`)
- Docker/Podman for MCP servers
- API credentials for integrated services

### Quick Start

```bash
# Clone the repository
git clone https://github.com/yourusername/mentat
cd mentat

# Build the binary
make build

# Configure signal-cli
signal-cli link -n "Mentat Assistant"

# Start MCP servers
docker-compose up -d

# Run Mentat
./mentat --config config.yaml
```

## Example Interactions

**You**: "What's on my calendar today?"

**Mentat**: "You have 3 meetings today:
- 10:00 AM - Team standup (30 min)
- 2:00 PM - Client presentation (1 hour) 
- 4:30 PM - 1:1 with Sarah (30 min)

Your afternoon is pretty packed! The client presentation is your longest meeting - would you like me to pull up the related emails or documents?"

**You**: "Schedule lunch with John next week when we're both free"

**Mentat**: "I'll find a time when you and John are both available for lunch next week.

I found these options when you're both free:
- Tuesday 12:00-1:00 PM
- Wednesday 12:30-1:30 PM  
- Thursday 12:00-1:00 PM

Would you like me to send John a calendar invite for one of these times?"

## Configuration

Mentat uses a YAML configuration file:

```yaml
signal:
  phone_number: "+1234567890"
  
claude:
  model: "claude-3-5-sonnet"
  
mcp_servers:
  - name: "google-calendar"
    url: "http://localhost:3000"
  - name: "gmail"
    url: "http://localhost:3001"
    
scheduler:
  morning_briefing: "0 7 * * *"  # 7 AM daily
```

## Development

```bash
# Run tests
make test

# Run linters
make lint

# Run integration tests
make test-integration

# Format code
make fmt
```

## Project Structure

```
mentat/
├── cmd/mentat/         # Main application entry point
├── internal/           # Private application code
│   ├── agent/         # Multi-agent validation system
│   ├── claude/        # Claude Code SDK integration
│   ├── queue/         # Message queue and workers
│   ├── signal/        # Signal messenger integration
│   └── conversation/  # Session management
├── tests/             # Integration tests
└── docs/              # Additional documentation
```

For detailed architecture information, see [ARCHITECTURE.md](ARCHITECTURE.md).

## License

This project is licensed under the MIT License - see the LICENSE file for details.

## Acknowledgments

- Built on [Claude Code](https://claude.ai/code) by Anthropic
- Uses [signal-cli](https://github.com/AsamK/signal-cli) for Signal integration
- Implements [Model Context Protocol (MCP)](https://modelcontextprotocol.io) for extensibility
