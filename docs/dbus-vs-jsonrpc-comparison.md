# DBus vs JSON-RPC for Signal-CLI Integration

## Executive Summary

For the Mentat project, **JSON-RPC over Unix socket is the recommended approach**. It offers better portability, simpler implementation, and aligns perfectly with our Go-based architecture while providing all the functionality we need.

## Detailed Comparison

### JSON-RPC Advantages

#### 1. **Simplicity and Portability**
- Works on any platform that supports Unix sockets or TCP
- No external dependencies beyond standard networking
- Easier to containerize (no DBus daemon required)
- Works seamlessly in minimal environments

#### 2. **Better Go Ecosystem Support**
- Standard `net` package handles Unix sockets natively
- JSON marshaling/unmarshaling is built into Go
- No need for DBus-specific Go libraries
- Cleaner, more idiomatic Go code

#### 3. **Flexible Connection Options**
```bash
# Unix socket (recommended for local communication)
signal-cli daemon --socket /var/run/signal-cli/socket

# TCP socket (useful for development/testing)
signal-cli daemon --tcp 127.0.0.1:7583

# HTTP endpoint (adds REST capabilities)
signal-cli daemon --http 127.0.0.1:8080
```

#### 4. **Easier Testing and Mocking**
- JSON-RPC is just JSON over a transport
- Easy to create mock servers for testing
- Can capture and replay messages
- Simpler to debug (just JSON text)

#### 5. **Production Deployment Benefits**
- Lower resource overhead (no DBus daemon)
- Better security isolation options
- Easier to monitor and log
- Compatible with standard Unix tools

### DBus Advantages

#### 1. **Native Linux Desktop Integration**
- Integrates with systemd user sessions
- Can use system/session bus infrastructure
- Better for desktop applications
- Native support for signals/events

#### 2. **Type Safety**
- DBus has a type system
- Method signatures are defined
- Some compile-time checking possible

#### 3. **Established Ecosystem**
- Many Linux tools use DBus
- Good for system-wide integration
- Standardized interfaces

### JSON-RPC Disadvantages

#### 1. **Manual Connection Management**
- Need to handle reconnection logic
- No built-in service discovery
- Must parse notifications vs responses

#### 2. **Text-Based Protocol**
- Slightly higher bandwidth usage
- Parsing overhead (minimal in practice)

### DBus Disadvantages

#### 1. **Platform Limitations**
- Linux-only in practice
- Requires DBus daemon running
- Complex in containers/minimal systems
- Not available on macOS/Windows

#### 2. **Complexity**
- Steeper learning curve
- More boilerplate code
- Complex type system
- Harder to debug

#### 3. **Go Support Issues**
- DBus Go libraries are less mature
- More complex code patterns
- Harder to test
- Less idiomatic Go

#### 4. **Deployment Challenges**
- Requires DBus session/system setup
- Permission configuration needed
- Harder to isolate
- More moving parts

## Feature Comparison

| Feature | JSON-RPC | DBus |
|---------|----------|------|
| Send messages | ✅ | ✅ |
| Receive messages | ✅ | ✅ |
| Attachments | ✅ | ✅ |
| Group messages | ✅ | ✅ |
| Typing indicators | ✅ | ✅ |
| Read receipts | ✅ | ✅ |
| Multi-account | ✅ | ✅ |
| Register/Link | ❌ | ❌ |
| Platform support | All | Linux |
| Container-friendly | ✅ | ⚠️ |
| Go implementation | Easy | Complex |

## Performance Considerations

### JSON-RPC
- Minimal overhead for local Unix socket
- JSON parsing is fast in Go
- No intermediary daemon
- Direct communication

### DBus
- Additional hop through DBus daemon
- More complex message routing
- Higher memory usage
- Better for high-frequency events

## Security Considerations

### JSON-RPC
- Unix socket permissions control access
- Can use filesystem ACLs
- No system-wide exposure
- Easy to isolate

### DBus
- DBus policy configuration required
- System vs session bus considerations
- More attack surface
- Harder to isolate

## Code Complexity Example

### JSON-RPC (Simple and Clear)
```go
// Connect
conn, err := net.Dial("unix", "/var/run/signal-cli/socket")

// Send
request := map[string]interface{}{
    "jsonrpc": "2.0",
    "method": "send",
    "params": map[string]interface{}{
        "recipient": []string{"+1234567890"},
        "message": "Hello",
    },
    "id": "1",
}
json.NewEncoder(conn).Encode(request)

// Receive
var response map[string]interface{}
json.NewDecoder(conn).Decode(&response)
```

### DBus (More Complex)
```go
// Requires dbus library
conn, err := dbus.SessionBus()

// Complex object path handling
obj := conn.Object("org.asamk.Signal", 
    dbus.ObjectPath("/org/asamk/Signal"))

// Method call with type marshaling
call := obj.Call("org.asamk.Signal.sendMessage", 0,
    "Hello", []string{}, []string{"+1234567890"})
```

## Mentat-Specific Considerations

### Why JSON-RPC is Better for Mentat:

1. **Deployment Simplicity**
   - Single binary + signal-cli
   - No DBus configuration
   - Works in containers
   - Easier NixOS packaging

2. **Go Implementation**
   - Cleaner interfaces
   - Better testability
   - More maintainable
   - Follows Go idioms

3. **Operational Benefits**
   - Easier to debug
   - Simple logging
   - Clear error messages
   - Standard monitoring

4. **Future Flexibility**
   - Could add HTTP transport
   - Easy to add encryption
   - Simple to extend
   - Protocol stability

## Recommendation

**Use JSON-RPC over Unix socket** for Mentat because:

1. ✅ Simpler implementation and maintenance
2. ✅ Better Go ecosystem fit
3. ✅ Easier testing and mocking
4. ✅ More portable and flexible
5. ✅ Lower operational complexity
6. ✅ All required features are supported

The only reason to choose DBus would be if Mentat needed deep integration with a Linux desktop environment, which is not the case for a server-side Signal bot.

## Implementation Decision

Based on this analysis, the Mentat project will use:
- **Protocol**: JSON-RPC 2.0
- **Transport**: Unix domain socket
- **Path**: `/var/run/signal-cli/socket`
- **Mode**: Daemon mode with automatic message reception

This provides the best balance of simplicity, reliability, and functionality for our use case.