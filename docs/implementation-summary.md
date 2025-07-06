# Mentat Implementation Summary

## Overview

This document summarizes the key implementation decisions made during our design session for Mentat's MCP integration and component management architecture.

## Major Architectural Decisions

### 1. Self-Contained Platform Approach

We evolved Mentat from a "thin orchestration layer" to a comprehensive personal assistant platform that manages all its dependencies:

- **Signal CLI**: Embedded and managed as a subprocess
- **MCP Servers**: Deployed as Docker containers with lifecycle management
- **Claude CLI**: Auto-downloaded and version managed

**Rationale**: This provides a superior user experience - just run `mentat setup` and have a fully working assistant.

### 2. Docker-Based MCP Integration

After evaluating multiple approaches (NPM process management, external services, NixOS containers), we chose Docker-based MCP deployment:

**Advantages**:
- Single dependency (Docker/Podman) instead of Node.js ecosystem
- Better isolation and resource control
- Built-in health checks and restart policies
- Unified logging and monitoring
- Clean credential mounting via volumes

**Implementation**:
- `internal/mcp/docker_server.go`: Container lifecycle management
- `internal/mcp/manager.go`: Parallel startup and health monitoring
- Dynamic MCP config generation for Claude CLI

### 3. Own Phone Number Architecture

Mentat uses its own Signal phone number rather than linking to a user's existing account:

**Benefits**:
- Clean separation - doesn't clutter personal Signal
- Multi-user support - family members can all text the same Mentat
- Professional identity - "Text your assistant at +1-XXX-MENTAT"
- Simplified permissions - Mentat owns the primary device

**Setup Flow**:
1. Guide user to obtain phone number (Google Voice recommended)
2. Handle Signal registration with SMS verification
3. Provide device management capabilities
4. Test bidirectional messaging

### 4. Comprehensive Setup Experience

Interactive setup wizard (`mentat setup`) that handles:
- Signal phone number registration
- SMS verification code entry
- MCP service credential configuration
- Health verification of all components
- Test message exchange

Future enhancement: Web-based setup UI with QR codes and OAuth flows.

## Implementation Phases

### Phase 1: MCP Infrastructure (3-4 days)
- Docker-based MCP server implementation
- Health monitoring and restart logic
- Credential management from filesystem
- Dynamic config generation

### Phase 2: Component Management (3-4 days)
- Signal CLI embedding and lifecycle
- Claude CLI auto-download
- Unified startup/shutdown sequences
- Component health monitoring

### Phase 3: Setup Experience (2-3 days)
- Interactive CLI setup wizard
- Signal registration flow
- Credential validation
- End-to-end testing

### Phase 4: Production Hardening (2-3 days)
- Error recovery mechanisms
- Comprehensive logging
- Operational documentation
- Integration tests

## Key Design Documents

1. **[ARCHITECTURE.md](../ARCHITECTURE.md)**: Updated with component management architecture
2. **[docs/mcp-integration-design.md](mcp-integration-design.md)**: Detailed MCP implementation
3. **[docs/signal-setup-design.md](signal-setup-design.md)**: Signal integration and setup flow

## Configuration Structure

```yaml
# Mentat manages these components internally
mentat:
  manage_components: true
  
  signal:
    phone_number: "+1-555-MENTAT"
    data_path: "/var/lib/mentat/signal"
    embedded: true
    
  claude:
    manage_installation: true
    version: "latest"
    
  mcp:
    runtime: "docker"  # or "podman"
    config_path: "/var/lib/mentat/mcp-config.json"
    servers:
      google-calendar:
        enabled: true
        port: 3000
      gmail:
        enabled: true
        port: 3002
      # ... other servers
```

## Credential Management

```
~/.config/mentat/
├── google-credentials.json    # Google OAuth
├── todoist-api-key           # Plain text API key
└── expensify-credentials.json # Expensify API
```

Credentials are mounted read-only into containers, never passed as environment variables.

## Operational Commands

```bash
# Setup
mentat setup                    # Interactive setup wizard

# Device Management
mentat devices list            # List Signal linked devices
mentat devices remove <id>     # Remove a device
mentat devices link            # Link new device (QR code)

# MCP Management
mentat mcp status              # Check all MCP servers
mentat mcp restart <server>    # Restart specific server
mentat mcp logs <server>       # View server logs

# Main Operation
mentat                         # Start the assistant
```

## Success Metrics

- **Setup Time**: < 10 minutes from download to working assistant
- **Component Reliability**: Automatic recovery from failures
- **User Experience**: No manual component management required
- **Multi-User Support**: Family members can share one Mentat

## Future Enhancements

1. **Web Setup UI**: Beautiful browser-based setup experience
2. **Automated Credential Flows**: OAuth for Google services
3. **Multi-Instance Support**: Run multiple Mentats with different numbers
4. **Custom MCP Servers**: Allow users to add their own MCP tools

## Conclusion

The shift to a self-contained platform approach dramatically improves the user experience while maintaining the architectural benefits of the original design. By managing all dependencies internally, Mentat becomes a true "personal assistant appliance" that just works.