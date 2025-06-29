# Phase Transition Workflow

This document explains the automated workflow for transitioning between implementation phases in the Mentat project.

## The Problem

When working through multi-phase projects, you need to:
1. Generate comprehensive prompts for each phase
2. Clear the context to avoid confusion
3. Start fresh with focused instructions
4. Maintain consistent quality standards

## Solutions Created

### 1. Quick Slash Command: `/project:n`

The simplest solution for fast transitions:

```bash
/project:n phase 2
/project:n "Add Claude Integration"
```

This command:
- Generates a comprehensive prompt for the specified phase
- Saves it to `prompts/next-phase.md`
- Shows the exact command to launch a fresh session

**Location**: `.claude/commands/n.md`

### 2. Full Workflow Command: `/project:next`

More sophisticated phase transition:

```bash
/project:next phase 2
/project:next intelligence layer
```

Features:
- Identifies phase by number or name
- Generates phase-specific prompt with all requirements
- Creates a timestamped prompt file
- Provides multiple launch options

**Location**: `.claude/commands/next.md`

### 3. Python Automation Script

The most powerful option for complex workflows:

```bash
python scripts/next-phase.py --phase 2
python scripts/next-phase.py --name "Queue & Persistence"
python scripts/next-phase.py --current  # Auto-detect next phase
```

Features:
- Parses TODO.md to understand all phases
- Generates comprehensive prompts with architecture references
- Creates executable launch scripts
- Tracks phase progress
- Provides interactive launch options

**Location**: `scripts/next-phase.py`

## How It Works

### 1. Claude Code Sessions

Claude Code supports named sessions that can be started fresh:

```bash
# Start new session with prompt
claude --session "mentat-phase-2" -p "$(cat prompt.md)"

# Resume later
claude --continue "mentat-phase-2"
```

### 2. Headless Mode

For automation or when you want a completely fresh start:

```bash
# Non-interactive mode
claude -p "$(cat prompt.md)" --output-format stream-json
```

### 3. Manual Process

Always available as a fallback:
1. Run `/clear` to reset context
2. Copy the generated prompt
3. Paste to start fresh

## Generated Prompt Structure

All generated prompts include:

1. **Phase identification** with ultrathink requirement
2. **Context documents** to study (ARCHITECTURE.md sections)
3. **Critical requirements**:
   - Research → Plan → Implement workflow
   - Zero linter warnings
   - Complete implementations only
   - No TODOs or placeholders
4. **Implementation tasks** from TODO.md
5. **Architecture conformance** requirements
6. **Success criteria** specific to the phase
7. **Confirmation statement** for completion

## Best Practices

1. **Use `/project:n` for quick transitions** during active development
2. **Use the Python script** for formal phase starts with full documentation
3. **Always start phases fresh** - don't carry context from previous phases
4. **Save generated prompts** in `prompts/` for reference
5. **Use named sessions** to allow resuming if needed

## Example Workflow

```bash
# Complete Phase 1
make test && make lint

# Transition to Phase 2
/project:n phase 2

# Claude responds with:
# ✅ Phase 2: Add Claude Integration ready!
# 
# To continue with fresh context:
# claude -p "$(cat prompts/next-phase.md)"

# Exit and run the command
/exit
claude -p "$(cat prompts/next-phase.md)"

# Now working in fresh context on Phase 2
```

## Technical Details

### Slash Commands

Claude Code automatically recognizes markdown files in:
- `.claude/commands/` - Project-specific commands
- `~/.claude/commands/` - Personal commands

Commands support:
- `$ARGUMENTS` placeholder for dynamic input
- Tool access (Read, Write, Bash)
- Structured output generation

### Session Management

Claude Code sessions:
- Are isolated conversation contexts
- Can be named for easy reference
- Support continuation/resumption
- Don't share context between sessions

This workflow leverages these features to ensure each phase starts with optimal context and focus.