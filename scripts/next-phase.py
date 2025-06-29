#!/usr/bin/env python3
"""
Next Phase Workflow Automation for Mentat Project

This script automates the transition between implementation phases by:
1. Generating comprehensive implementation prompts
2. Creating launch scripts for fresh Claude Code sessions
3. Managing phase progression tracking

Usage:
    python scripts/next-phase.py --phase 2
    python scripts/next-phase.py --name "intelligence layer"
    python scripts/next-phase.py --current  # Auto-detect next phase
"""

import argparse
import json
import os
import re
import subprocess
import sys
from datetime import datetime
from pathlib import Path
from typing import Dict, List, Optional, Tuple


class PhaseManager:
    def __init__(self, project_root: Path):
        self.project_root = project_root
        self.todo_path = project_root / "TODO.md"
        self.architecture_path = project_root / "ARCHITECTURE.md"
        self.claude_md_path = Path.home() / ".claude" / "CLAUDE.md"
        self.prompts_dir = project_root / "prompts"
        self.prompts_dir.mkdir(exist_ok=True)
        
    def parse_phases(self) -> Dict[int, Dict[str, any]]:
        """Parse TODO.md to extract all phases."""
        phases = {}
        current_phase = None
        
        with open(self.todo_path, 'r') as f:
            content = f.read()
            
        # Extract phases using regex
        phase_pattern = r'## Phase (\d+): (.+?) \(Target: (.+?)\)'
        phase_matches = re.finditer(phase_pattern, content)
        
        for match in phase_matches:
            phase_num = int(match.group(1))
            phase_name = match.group(2)
            target_time = match.group(3)
            
            # Find the content for this phase
            start_pos = match.end()
            next_phase_match = re.search(r'\n## Phase \d+:', content[start_pos:])
            end_pos = start_pos + next_phase_match.start() if next_phase_match else len(content)
            
            phase_content = content[start_pos:end_pos].strip()
            
            phases[phase_num] = {
                'name': phase_name,
                'target': target_time,
                'content': phase_content,
                'tasks': self._extract_tasks(phase_content)
            }
            
        return phases
    
    def _extract_tasks(self, content: str) -> List[str]:
        """Extract task items from phase content."""
        tasks = []
        for line in content.split('\n'):
            if line.strip().startswith('- [ ]'):
                tasks.append(line.strip())
        return tasks
    
    def generate_prompt(self, phase_num: int, phase_info: Dict[str, any]) -> str:
        """Generate a comprehensive implementation prompt for a phase."""
        
        # Read reference documents
        architecture_sections = self._get_relevant_architecture_sections(phase_info['name'])
        
        prompt = f"""# Phase {phase_num}: {phase_info['name']} Implementation

You are tasked with implementing Phase {phase_num} ({phase_info['name']}) of the Mentat personal assistant bot. This phase has a target completion time of {phase_info['target']}. You must **ultrathink** deeply about every design decision, considering long-term maintenance, testability, and performance implications.

## Context Documents

You MUST study these documents thoroughly before implementing:
1. **ARCHITECTURE.md** - Sections: {', '.join(architecture_sections)}
2. **TODO.md** - Phase {phase_num} section for detailed tasks and acceptance criteria
3. **docs/signal-rpc-integration.md** - For Signal integration details
4. **~/.claude/CLAUDE.md** - Critical development standards

## Critical Requirements

### Research → Plan → Implement
**NEVER JUMP STRAIGHT TO CODING!** Follow this sequence:
1. **Research**: Study the architecture sections relevant to this phase. Understand how components integrate.
2. **Plan**: Create a detailed implementation plan. List files to create/modify in order.
3. **Implement**: Execute the plan with validation checkpoints after each component.

### Completion Standards
The task is NOT complete until:
- ALL linters pass with zero warnings (run `make lint` with all 31 linters enabled)
- ALL tests pass with >80% coverage of business logic
- The phase objectives are fully achieved and tested end-to-end
- Integration with previous phases is verified
- No placeholder comments, TODOs, or "good enough" compromises remain

### Code Evolution Rules
- This is a feature branch - implement the NEW solution directly
- DELETE any placeholder or old code when replacing
- NO migration functions, compatibility layers, or versioned names
- When implementing an interface, implement it COMPLETELY per ARCHITECTURE.md
- If the architecture specifies a pattern, follow it EXACTLY

### Go Code Quality Requirements
- **NO interface{{}} or any{{}}** - The architecture defines all types concretely
- Simple, focused interfaces following Interface Segregation Principle
- Error handling: Use `fmt.Errorf("context: %w", err)` pattern - NO custom error struct hierarchies
- Follow the project structure from ARCHITECTURE.md exactly
- Make zero values useful

### Phase-Specific Implementation Tasks

{phase_info['content']}

### Architecture Conformance

Key patterns to implement from ARCHITECTURE.md:
- **Dependency Injection**: All components use constructor injection
- **Option Pattern**: Use functional options for configuration
- **Interface Design**: Small, focused interfaces over large ones
- **State Management**: Explicit state machines where specified
- **Persistence**: SQLite integration as designed in Section 8

### Testing Requirements

This phase requires:
- Unit tests for all new components (>80% coverage)
- Integration tests connecting to previous phases
- Table-driven tests for complex logic
- Mock implementations for all new interfaces
- Concurrent access tests where applicable

### Validation Checkpoints

After implementing each major component:
1. Run `make lint` - fix ALL warnings immediately
2. Run `make test` - ensure coverage meets standards
3. Run integration tests with previous phases
4. Manually test the functionality end-to-end
5. Verify no regression in existing features

### Success Criteria

Phase {phase_num} is complete when:
{self._generate_success_criteria(phase_info)}

### Antipatterns to Avoid

- Do NOT skip the research phase - understanding is crucial
- Do NOT implement partial interfaces - complete or don't start
- Do NOT use reflection unless absolutely necessary
- Do NOT create "helper" packages outside the architecture
- Do NOT leave any linter warnings as "acceptable"
- Do NOT stop at "mostly working" - production quality only

## Remember

- **Ultrathink** before implementing - consider maintenance and evolution
- Reference specific sections of ARCHITECTURE.md in comments
- When in doubt, the architecture document is the source of truth
- This is production code - no shortcuts or compromises
- Each phase builds on the previous - ensure solid integration

Confirm when ALL linters pass, ALL meaningful tests pass, the feature is completely implemented and working, and all old code has been removed."""
        
        return prompt
    
    def _get_relevant_architecture_sections(self, phase_name: str) -> List[str]:
        """Determine which architecture sections are relevant for a phase."""
        # This is a mapping based on phase names
        mappings = {
            "Minimal Signal Echo Bot": ["Core Interfaces (1-2)", "Signal-CLI JSON-RPC Integration"],
            "Add Claude Integration": ["LLM Interface (1)", "Claude Configuration"],
            "Add Queue & Persistence": ["Queue Integration (7)", "Persistence Layer (8)", "Storage Implementation"],
            "Add MCP Integration": ["MCP Server Integration (6)", "Claude Configuration"],
            "Multi-Agent Validation": ["Validation Strategy Interface (4)", "Multi-Agent Validation Process"],
            "Production Hardening": ["All sections", "Deployment", "Security Considerations"]
        }
        
        return mappings.get(phase_name, ["Review all relevant sections"])
    
    def _generate_success_criteria(self, phase_info: Dict[str, any]) -> str:
        """Generate phase-specific success criteria."""
        criteria = []
        
        # Count completed tasks
        total_tasks = len(phase_info['tasks'])
        criteria.append(f"- All {total_tasks} tasks marked as completed in TODO.md")
        
        # Add phase-specific criteria based on name
        if "Signal" in phase_info['name']:
            criteria.append("- Can send and receive Signal messages reliably")
            criteria.append("- Reconnection logic handles network interruptions")
        
        if "Claude" in phase_info['name']:
            criteria.append("- Claude integration works with proper error handling")
            criteria.append("- Response times are under 10 seconds")
        
        if "Queue" in phase_info['name']:
            criteria.append("- Messages persist across restarts")
            criteria.append("- Queue handles concurrent conversations correctly")
            criteria.append("- State transitions are properly enforced")
        
        if "MCP" in phase_info['name']:
            criteria.append("- All MCP servers are accessible")
            criteria.append("- Tool usage is logged and trackable")
        
        criteria.append("- ALL linters pass with zero warnings")
        criteria.append("- ALL tests pass with required coverage")
        criteria.append("- Manual testing confirms expected behavior")
        
        return '\n'.join(criteria)
    
    def create_launch_script(self, phase_num: int, phase_name: str, prompt_file: Path) -> Path:
        """Create a bash script to launch the phase with a fresh Claude session."""
        
        script_name = f"launch-phase-{phase_num}.sh"
        script_path = self.project_root / script_name
        
        # Convert phase name to kebab case
        kebab_name = re.sub(r'[^\w\s-]', '', phase_name.lower())
        kebab_name = re.sub(r'[-\s]+', '-', kebab_name)
        
        script_content = f"""#!/bin/bash
#
# Launch Script for Phase {phase_num}: {phase_name}
# Generated: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}
#
set -euo pipefail

# Configuration
PHASE_NUM={phase_num}
PHASE_NAME="{phase_name}"
PROMPT_FILE="{prompt_file.relative_to(self.project_root)}"
SESSION_NAME="mentat-phase-{phase_num}-$(date +%Y%m%d-%H%M%S)"

# Colors for output
GREEN='\\033[0;32m'
BLUE='\\033[0;34m'
YELLOW='\\033[1;33m'
NC='\\033[0m' # No Color

# Header
echo -e "${{GREEN}}🚀 Mentat Implementation - Phase ${{PHASE_NUM}}${{NC}}"
echo -e "${{BLUE}}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${{NC}}"
echo -e "Phase: ${{YELLOW}}${{PHASE_NAME}}${{NC}}"
echo -e "Prompt: ${{PROMPT_FILE}}"
echo -e "Session: ${{SESSION_NAME}}"
echo ""

# Check if prompt file exists
if [ ! -f "${{PROMPT_FILE}}" ]; then
    echo -e "${{YELLOW}}⚠️  Error: Prompt file not found!${{NC}}"
    echo "Expected at: ${{PROMPT_FILE}}"
    exit 1
fi

# Check if claude command exists
if ! command -v claude &> /dev/null; then
    echo -e "${{YELLOW}}⚠️  Error: claude command not found!${{NC}}"
    echo "Please install Claude Code first."
    exit 1
fi

# Display options
echo "Select launch mode:"
echo "1) Interactive mode (recommended for development)"
echo "2) Headless mode (for automation/CI)"
echo "3) View prompt only"
echo ""
read -p "Enter choice [1-3]: " choice

case $choice in
    1)
        echo -e "\\n${{GREEN}}Starting interactive Claude Code session...${{NC}}"
        echo "Tip: This session can be resumed later with:"
        echo "  claude --continue \\"${{SESSION_NAME}}\\""
        echo ""
        sleep 2
        exec claude --session "${{SESSION_NAME}}" -p "$(cat "${{PROMPT_FILE}}")"
        ;;
    2)
        echo -e "\\n${{GREEN}}Starting headless Claude Code session...${{NC}}"
        echo "Output will be streamed as JSON..."
        echo ""
        sleep 2
        exec claude -p "$(cat "${{PROMPT_FILE}}")" --output-format stream-json
        ;;
    3)
        echo -e "\\n${{GREEN}}Prompt content:${{NC}}"
        echo -e "${{BLUE}}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${{NC}}"
        cat "${{PROMPT_FILE}}"
        echo -e "${{BLUE}}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${{NC}}"
        echo ""
        echo "To use this prompt:"
        echo "1. Run: /clear"
        echo "2. Copy and paste the prompt above"
        echo "3. Or run: ./$(basename "$0") and select option 1 or 2"
        ;;
    *)
        echo "Invalid choice. Exiting."
        exit 1
        ;;
esac
"""
        
        with open(script_path, 'w') as f:
            f.write(script_content)
            
        # Make executable
        os.chmod(script_path, 0o755)
        
        return script_path
    
    def save_progress(self, phase_num: int) -> None:
        """Save current progress to track which phases are completed."""
        progress_file = self.project_root / ".phase-progress.json"
        
        progress = {}
        if progress_file.exists():
            with open(progress_file, 'r') as f:
                progress = json.load(f)
        
        progress[str(phase_num)] = {
            'started': datetime.now().isoformat(),
            'status': 'in_progress'
        }
        
        with open(progress_file, 'w') as f:
            json.dump(progress, f, indent=2)


def main():
    parser = argparse.ArgumentParser(description='Mentat Phase Transition Automation')
    group = parser.add_mutually_exclusive_group(required=True)
    group.add_argument('--phase', type=int, help='Phase number (1-6)')
    group.add_argument('--name', type=str, help='Phase name or partial match')
    group.add_argument('--current', action='store_true', help='Auto-detect next phase')
    
    args = parser.parse_args()
    
    # Find project root
    project_root = Path.cwd()
    while project_root.parent != project_root:
        if (project_root / 'TODO.md').exists():
            break
        project_root = project_root.parent
    else:
        print("Error: Could not find project root (no TODO.md found)")
        sys.exit(1)
    
    manager = PhaseManager(project_root)
    phases = manager.parse_phases()
    
    # Determine which phase to generate
    if args.phase:
        phase_num = args.phase
        if phase_num not in phases:
            print(f"Error: Phase {phase_num} not found")
            sys.exit(1)
    elif args.name:
        # Find phase by name
        phase_num = None
        for num, info in phases.items():
            if args.name.lower() in info['name'].lower():
                phase_num = num
                break
        if not phase_num:
            print(f"Error: No phase found matching '{args.name}'")
            sys.exit(1)
    else:
        # Auto-detect next phase
        # For now, just use phase 1 (would need to read progress file)
        phase_num = 1
    
    phase_info = phases[phase_num]
    
    print(f"🎯 Generating prompt for Phase {phase_num}: {phase_info['name']}")
    
    # Generate prompt
    prompt = manager.generate_prompt(phase_num, phase_info)
    
    # Save prompt
    kebab_name = re.sub(r'[^\w\s-]', '', phase_info['name'].lower())
    kebab_name = re.sub(r'[-\s]+', '-', kebab_name)
    prompt_file = manager.prompts_dir / f"phase-{phase_num}-{kebab_name}.md"
    
    with open(prompt_file, 'w') as f:
        f.write(prompt)
    
    print(f"✅ Prompt saved to: {prompt_file}")
    
    # Create launch script
    script_path = manager.create_launch_script(phase_num, phase_info['name'], prompt_file)
    print(f"✅ Launch script created: {script_path}")
    
    # Save progress
    manager.save_progress(phase_num)
    
    # Display instructions
    print("\n" + "="*50)
    print(f"✨ Phase {phase_num} Preparation Complete!")
    print("="*50)
    print(f"\n📁 Generated Files:")
    print(f"   Prompt: {prompt_file.relative_to(project_root)}")
    print(f"   Launcher: {script_path.name}")
    print(f"\n🚀 Quick Start:")
    print(f"   ./{script_path.name}")
    print(f"\n💡 Alternative Options:")
    print(f"   - Interactive: claude -p \"$(cat {prompt_file.name})\"")
    print(f"   - Headless: claude -p \"$(cat {prompt_file.name})\" --output-format stream-json")
    print(f"   - Manual: /clear then paste from {prompt_file.name}")
    print("\n" + "="*50)


if __name__ == "__main__":
    main()