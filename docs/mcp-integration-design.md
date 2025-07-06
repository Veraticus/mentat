# MCP Integration Design

## Overview

This document describes the design and implementation of MCP (Model Context Protocol) server integration in Mentat. MCP servers provide Claude with access to external tools like calendar, email, and memory storage.

## Architecture Decision

After evaluating multiple approaches, we chose **Docker-based process management** for MCP servers:

### Why Docker over NPM
1. **Single dependency**: Just Docker/Podman instead of Node.js ecosystem
2. **Version isolation**: Each container has its own runtime
3. **Resource control**: Built-in CPU/memory limits
4. **Better operations**: Unified logging, health checks, restart policies
5. **Security**: Credential isolation via volume mounts

### Why Not Separate Services
1. **MCP servers exist only for Mentat**: No independent value
2. **Atomic deployment**: Either everything works or nothing does
3. **Simplified operations**: One thing to monitor and debug
4. **Resource efficiency**: Lightweight Node.js containers

## Implementation Design

### Component Architecture

```
┌─────────────────────────────────────────┐
│              Mentat Process             │
│                                         │
│  ┌─────────────────────────────────┐   │
│  │       MCP Manager                │   │
│  │                                  │   │
│  │  - Start/stop containers         │   │
│  │  - Health monitoring             │   │
│  │  - Config generation             │   │
│  │  - Credential mounting           │   │
│  └─────────────────────────────────┘   │
└─────────────────────────────────────────┘
                    │
                    ├── Docker/Podman Runtime
                    │
                    ├── mcp-google-calendar (container)
                    ├── mcp-gmail (container)
                    ├── mcp-memory (container)
                    ├── mcp-todoist (container)
                    └── mcp-expensify (container)
```

### MCP Servers

| Server | Port | Purpose | Credentials |
|--------|------|---------|-------------|
| google-calendar | 3000 | Calendar management | google-credentials.json |
| google-contacts | 3001 | Contact lookup | google-credentials.json |
| gmail | 3002 | Email access | google-credentials.json |
| todoist | 3003 | Task management | todoist-api-key |
| memory | 3004 | Conversation memory | None |
| expensify | 3005 | Expense tracking | expensify-credentials.json |

### Credential Management

Credentials are stored in `~/.config/mentat/` and mounted read-only:

```
~/.config/mentat/
├── google-credentials.json    # OAuth for Google services
├── todoist-api-key            # Plain text API key
└── expensify-credentials.json # Expensify API credentials
```

### Health Monitoring

Each MCP server exposes `/health` endpoint:
- Check every 30 seconds
- Restart after 3 consecutive failures
- Exponential backoff on restart attempts

## Implementation Phases

### Phase 1: Docker Infrastructure (Days 1-3)
- Create `internal/mcp/interfaces.go`
- Implement `DockerMCPServer` with container lifecycle
- Add health check monitoring
- Test with single MCP server

### Phase 2: Manager Implementation (Days 4-6)
- Implement `MCPManager` for parallel startup
- Add config generation and persistence
- Create credential validation
- Wire into main application

### Phase 3: Production Hardening (Days 7-9)
- Add restart logic and recovery
- Implement metrics collection
- Create operational documentation
- Test failure scenarios

## Configuration

### Runtime Configuration
```yaml
mcp:
  runtime: "docker"  # or "podman"
  config_path: "/var/lib/mentat/mcp-config.json"
  health_check_interval: "30s"
  startup_timeout: "60s"
  
  servers:
    google-calendar:
      enabled: true
      image: "ghcr.io/mentat/mcp-google-calendar:latest"
      port: 3000
      
    gmail:
      enabled: true  
      image: "ghcr.io/mentat/mcp-gmail:latest"
      port: 3002
    
    # ... other servers
```

### Generated MCP Config (for Claude)
```json
{
  "mcpServers": {
    "google-calendar": {
      "transport": {
        "type": "http",
        "url": "http://localhost:3000"
      }
    },
    "gmail": {
      "transport": {
        "type": "http",
        "url": "http://localhost:3002"
      }
    }
  }
}
```

## Security Considerations

1. **Credential Protection**: Read-only mounts, never in environment
2. **Network Isolation**: Containers only expose localhost ports
3. **Resource Limits**: Prevent runaway containers
4. **Audit Logging**: Track all MCP operations

## Monitoring and Operations

### Key Metrics
- Container health status
- API response times
- Error rates per MCP server
- Resource usage (CPU, memory)

### Operational Commands
```bash
# Check MCP server status
mentat mcp status

# Restart specific server
mentat mcp restart gmail

# View logs
mentat mcp logs google-calendar

# Test health endpoints
mentat mcp health
```

## Future Enhancements

1. **Dynamic MCP Discovery**: Auto-detect available MCP servers
2. **Custom MCP Servers**: Allow users to add their own
3. **Redundancy**: Multiple instances for critical servers
4. **Caching Layer**: Reduce API calls to external services