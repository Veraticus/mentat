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

## Signal Integration

### Overview

Mentat uses its own Signal phone number rather than linking to your personal account. This design provides clean separation, multi-user support, and a professional identity for your AI assistant.

### Registration Flow

The Signal integration supports a complete registration flow for new phone numbers:

1. **Phone Number Setup**: Mentat guides you through obtaining a dedicated phone number (Google Voice recommended)
2. **Signal Registration**: Automated registration with captcha and SMS verification support
3. **Device Management**: Full control over linked devices via CLI commands

### Device Management

Mentat includes comprehensive device management capabilities:

```bash
# List all linked devices
./mentat devices list

# Remove a linked device (by ID)
./mentat devices remove 2

# Link a new device (generates QR code)
./mentat devices link "Mom's iPad"
```

**Device Listing Output:**
```
ID  NAME           PRIMARY  CREATED     LAST SEEN
--  ----           -------  -------     ---------
1   Primary Phone  Yes      2024-01-01  2024-01-02 15:04
2   Desktop                 2024-01-01  2024-01-02 10:00
3   Mom's iPad              2024-01-02  2024-01-02 14:30
```

### Setup Process for New Users

1. **Obtain a Phone Number**
   - Google Voice (recommended): Free US number with SMS support
   - Twilio: Programmable numbers with global availability
   - Other VOIP providers that support SMS

2. **Initial Registration**
   ```bash
   # The setup wizard guides you through:
   - Phone number entry
   - Captcha verification (if required)
   - SMS code verification
   - Bidirectional message testing
   ```

3. **Link Your Devices**
   ```bash
   # Generate a linking URI/QR code
   ./mentat devices link "Your Device Name"
   
   # This displays a QR code to scan in Signal app
   # The device appears in your device list once linked
   ```

### Architecture Details

The Signal integration consists of several key components:

- **Transport Layer**: JSON-RPC communication with signal-cli daemon
- **Manager**: Orchestrates registration, health checks, and subprocess lifecycle
- **Device Manager**: Handles device operations (list, remove, link)
- **Messenger**: Sends messages and typing indicators
- **Process Manager**: Manages the signal-cli daemon process

### Registration States

The registration flow uses a state machine with these states:
- `Unregistered`: No registration attempted
- `RegistrationStarted`: Request sent to Signal
- `CaptchaRequired`: Human verification needed
- `AwaitingSMSCode`: Waiting for SMS verification
- `Registered`: Successfully registered

### Technical Implementation

**Key Files:**
- `internal/signal/manager.go`: Main Signal manager orchestration
- `internal/signal/devices.go`: Device management implementation  
- `internal/signal/registration.go`: Registration state machine
- `internal/setup/registration.go`: Registration coordinator
- `cmd/mentat/devices.go`: CLI interface for device management

**Security Considerations:**
- Phone numbers are stored in `/etc/signal-bot/phone-number` (production) or environment variables
- Signal data stored in `/var/lib/mentat/signal/data/{phone-number}/`
- All Signal communication happens via local Unix socket
- No Signal credentials are stored - uses signal-cli's secure storage

### Troubleshooting

**Registration Issues:**
- Ensure signal-cli is installed and in PATH
- Check that the phone number can receive SMS
- Some VOIP numbers may be rejected by Signal - try a different provider
- Captcha may be required for new numbers - follow the provided URL

**Device Management:**
- Cannot remove the primary device (your phone)
- Linked devices appear after scanning the complete QR code
- Device linking has a 2-minute timeout - scan promptly

For detailed architecture information, see [ARCHITECTURE.md](ARCHITECTURE.md).

## License

This project is licensed under the MIT License - see the LICENSE file for details.

## Acknowledgments

- Built on [Claude Code](https://claude.ai/code) by Anthropic
- Uses [signal-cli](https://github.com/AsamK/signal-cli) for Signal integration
- Implements [Model Context Protocol (MCP)](https://modelcontextprotocol.io) for extensibility
