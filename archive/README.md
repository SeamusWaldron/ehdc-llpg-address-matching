# Archive Directory

This directory contains historical files that were moved from the project root to keep the main directory clean and organized.

## Directory Structure

### `/tests/`
Test files and validation scripts:
- `test_*.sql` - SQL test queries and validations
- `test_*.go` - Go test programs  
- `check_*.sql` - Data integrity checks
- `validate_*.sql` - Validation queries
- `generate_*.sql` - SQL generation scripts

### `/old-binaries/`
Legacy compiled executables:
- `address-matcher*` - Previous matcher implementations
- `component-matcher*` - Component-based matching tools
- `gopostal-*` - Address parsing utilities
- `matcher-v2*` - Previous versions of main matcher
- `loader` - Legacy data loading utility

### `/old-scripts/`
Previous deployment and utility scripts:
- `deploy_*.sh` - Old deployment scripts
- `execute_*.sh` - Migration execution scripts
- `run_*.sh` - Previous run scripts
- `*.log` - Historical log files
- `*_stats.txt` - Performance statistics from previous runs

### `/old-docs/`
Historical documentation:
- Previous version documentation
- Implementation plans that have been completed
- Deployment guides for older architectures
- Technical specifications that have been superseded

## Current Active Files

The main project directory now contains only:
- **Core Documentation**: README.md, CLAUDE.md, PROJECT_SPECIFICATION.md
- **Quick Start**: QUICKSTART.md, QUICKSTART_NEW_MACHINE.md
- **Configuration**: docker-compose.yml, setup scripts
- **Active Directories**: cmd/, internal/, migrations/, source_docs/

## Note

These archived files are preserved for reference but are not part of the active development workflow. The current system uses:
- `cmd/matcher-v2/main.go` - Active matcher implementation
- `start_complete_system.sh` - Current startup script
- `setup_new_machine.sh` - New machine setup
- Documentation in the main directory - Current specifications