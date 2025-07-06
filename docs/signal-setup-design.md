# Signal Setup Design

## Overview

This document describes the design for Mentat's Signal integration, focusing on the setup experience and device management capabilities.

## Key Design Decision: Own Phone Number

Mentat uses its own phone number rather than linking to a user's existing Signal account. This provides:

1. **Clean separation**: Mentat doesn't clutter personal Signal
2. **Multi-user support**: Family members can all text the same Mentat
3. **Professional identity**: "Text your assistant at +1-XXX-MENTAT"
4. **Simplified permissions**: Mentat owns the primary device

## Setup Flow Design

### Interactive Setup Wizard

The setup process guides users through obtaining and registering a phone number:

```
┌─────────────────────────────────────────┐
│          Mentat Setup Wizard            │
│                                         │
│  1. 📱 Phone number guidance            │
│     - Recommend Google Voice            │
│     - Show alternatives (Twilio, etc)   │
│                                         │
│  2. 🔐 Signal registration              │
│     - Attempt registration              │
│     - Handle captcha if required        │
│     - Guide through SMS verification    │
│                                         │
│  3. 🧪 Bidirectional testing            │
│     - Send test message FROM Mentat     │
│     - Receive test message TO Mentat    │
│                                         │
│  4. 🚀 MCP service configuration        │
│     - Google OAuth flow                 │
│     - API key collection                │
│     - Health verification               │
│                                         │
│  5. ✅ Save config & create service     │
└─────────────────────────────────────────┘
```

### Phone Number Providers

Recommended options presented during setup:

| Provider | Cost | Pros | Cons |
|----------|------|------|------|
| Google Voice | Free | Easy setup, reliable | No API |
| Twilio | $1/month | Full API, webhooks | Requires payment |
| TextNow | Free | No cost | Less reliable |
| Dedicated SIM | Varies | Most reliable | Hardware required |

### Registration Flow

```go
// Pseudo-code for registration process
func setupSignal() error {
    // 1. Get phone number from user
    phoneNumber := promptForPhoneNumber()
    
    // 2. Attempt registration
    if err := registerWithSignal(phoneNumber); err != nil {
        if isCaptchaError(err) {
            // 3. Handle captcha
            token := promptForCaptchaToken()
            err = registerWithCaptcha(phoneNumber, token)
        }
    }
    
    // 4. SMS verification
    code := promptForSMSCode()
    return verifyWithCode(phoneNumber, code)
}
```

## Data Storage

Signal data is stored in a configurable location:

```
/var/lib/mentat/signal/data/+1XXXXXXXXXX/
├── account.db          # Account database
├── signal.db           # Messages and contacts
├── avatars/            # Contact avatars
├── attachments/        # Downloaded attachments
└── stickers/           # Sticker packs
```

## Device Management

Since Mentat owns the primary device, it provides comprehensive device management:

### CLI Commands

```bash
# List all linked devices
mentat devices list

# Remove a linked device
mentat devices remove <device-id>

# Link a new device (shows QR code)
mentat devices link
```

### Device Naming

When linking devices, Mentat prompts for meaningful names:
- "Mom's iPad" instead of "Signal Desktop"
- "Work Laptop" instead of "Chrome"
- Names are stored and displayed in device lists

### Implementation

```go
type DeviceManager interface {
    ListDevices() ([]Device, error)
    RemoveDevice(deviceID int) error
    GenerateLinkingURI(deviceName string) (string, error)
}

type Device struct {
    ID       int
    Name     string
    Created  time.Time
    LastSeen time.Time
    Primary  bool
}
```

## Embedded Signal Management

Mentat embeds and manages signal-cli as a subprocess:

```go
type EmbeddedSignalManager struct {
    phoneNumber string
    configPath  string
    process     *os.Process
    rpcClient   JSONRPCClient
}

// Lifecycle methods
Start(ctx context.Context) error
Stop(ctx context.Context) error
HealthCheck(ctx context.Context) error
```

### Health Monitoring

- Check signal-cli process status every 30 seconds
- Verify RPC endpoint responds
- Restart on failure with exponential backoff
- Alert on repeated failures

## Configuration

### User Configuration

```yaml
signal:
  phone_number: "+1-555-MENTAT"
  data_path: "/var/lib/mentat/signal"
  
  # Device management permissions
  device_admins:
    - "+1234567890"  # Only these numbers can manage devices
  
  # Access control
  allowed_users:
    - "+1234567890"  # Owner
    - "+1234567891"  # Spouse
    - "+1234567892"  # Kid
```

### Setup State

Setup progress is tracked to allow resumption:

```yaml
setup_state:
  completed_steps:
    - phone_number_selected
    - signal_registered
    - signal_verified
  current_step: mcp_configuration
  phone_number: "+1-555-MENTAT"
```

## Security Considerations

1. **Signal data encryption**: All Signal data remains encrypted at rest
2. **Device authorization**: Only admin numbers can manage devices
3. **Access control**: Whitelist of allowed users
4. **Audit logging**: Track all device management operations

## Future Enhancements

### Web-Based Setup

Future version will provide web UI for setup:
- QR code display for registration
- Real-time progress updates
- OAuth flows for Google services
- Beautiful, guided experience

### Automated SMS Handling

Potential integrations for automatic SMS code retrieval:
- Twilio webhook integration
- Android companion app
- Email forwarding (Google Voice)

### Multi-Instance Support

Future architecture for multiple Mentat instances:
- Shared MCP servers
- Separate Signal numbers per instance
- Centralized management UI

## Testing

### Setup Testing

1. Test with fresh phone number
2. Test with already-registered number
3. Test captcha flow
4. Test SMS timeout and retry
5. Test invalid phone formats

### Device Management Testing

1. Link multiple devices
2. Remove devices
3. Handle primary device edge cases
4. Test permission enforcement
5. Verify state consistency