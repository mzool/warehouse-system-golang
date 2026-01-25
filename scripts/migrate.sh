#!/bin/bash

################################################################################
# Database Migration Script
# 
# Description:
#   Applies SQL migrations from a directory to PostgreSQL database.
#   Tracks applied migrations in a 'migrations' table.
#   Supports rollback, dry-run, and forced re-application.
#
# Usage:
#   ./migrate.sh [OPTIONS]
#
# Options:
#   -d, --dir <path>       Migration directory (default: internal/database/migrations)
#   -e, --env <file>       .env file path (default: .env in project root)
#   -r, --rollback [n]     Rollback last n migrations (default: 1)
#   -f, --force            Force re-apply migrations (dangerous!)
#   --dry-run              Show what would be executed without applying
#   --status               Show migration status
#   -h, --help             Show this help message
#
# Examples:
#   ./migrate.sh                                    # Apply pending migrations
#   ./migrate.sh -d ./my-migrations                 # Use custom directory
#   ./migrate.sh --status                           # Show migration status
#   ./migrate.sh --dry-run                          # Preview migrations
#   ./migrate.sh --rollback 2                       # Rollback last 2 migrations
#
# Environment Variables (from .env):
#   DB_URL                 PostgreSQL connection URL (required)
#
################################################################################

set -euo pipefail  # Exit on error, undefined vars, pipe failures

# Color output
readonly RED='\033[0;31m'
readonly GREEN='\033[0;32m'
readonly YELLOW='\033[1;33m'
readonly BLUE='\033[0;34m'
readonly NC='\033[0m' # No Color

# Default configuration
MIGRATION_DIR="internal/database/migrations"
ENV_FILE=".env"
DRY_RUN=false
SHOW_STATUS=false
ROLLBACK_COUNT=0
FORCE_APPLY=false

################################################################################
# Helper Functions
################################################################################

log_info() {
    echo -e "${BLUE}[INFO]${NC} $*"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $*"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $*"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $*" >&2
}

show_help() {
    sed -n '/^# Description:/,/^################################################################################$/p' "$0" | 
        sed 's/^# //; s/^#//'
    exit 0
}

################################################################################
# Parse Command Line Arguments
################################################################################

parse_args() {
    while [[ $# -gt 0 ]]; do
        case $1 in
            -d|--dir)
                MIGRATION_DIR="$2"
                shift 2
                ;;
            -e|--env)
                ENV_FILE="$2"
                shift 2
                ;;
            -r|--rollback)
                ROLLBACK_COUNT="${2:-1}"
                shift 2
                ;;
            -f|--force)
                FORCE_APPLY=true
                shift
                ;;
            --dry-run)
                DRY_RUN=true
                shift
                ;;
            --status)
                SHOW_STATUS=true
                shift
                ;;
            -h|--help)
                show_help
                ;;
            *)
                log_error "Unknown option: $1"
                echo "Use --help for usage information"
                exit 1
                ;;
        esac
    done
}

################################################################################
# Environment and Database Configuration
################################################################################

load_env() {
    if [[ ! -f "$ENV_FILE" ]]; then
        log_error ".env file not found at: $ENV_FILE"
        log_info "Please create .env file or specify path with --env option"
        exit 1
    fi

    log_info "Loading environment from: $ENV_FILE"
    
    # Load DB_URL from .env file
    if ! DB_URL=$(grep "^DB_URL=" "$ENV_FILE" | cut -d '=' -f2- | tr -d '"' | tr -d "'"); then
        log_error "Failed to read .env file"
        exit 1
    fi

    if [[ -z "$DB_URL" ]]; then
        log_error "DB_URL not found in $ENV_FILE"
        log_info "Please add DB_URL to your .env file"
        log_info "Example: DB_URL=postgres://user:password@localhost:5432/dbname"
        exit 1
    fi

    log_success "Database URL loaded"
}

check_psql() {
    if ! command -v psql &> /dev/null; then
        log_error "psql command not found"
        log_info "Please install PostgreSQL client tools"
        log_info "  Ubuntu/Debian: sudo apt-get install postgresql-client"
        log_info "  macOS: brew install postgresql"
        log_info "  Fedora/RHEL: sudo dnf install postgresql"
        exit 1
    fi
}

test_db_connection() {
    log_info "Testing database connection..."
    
    if ! psql "$DB_URL" -c "SELECT 1;" &> /dev/null; then
        log_error "Failed to connect to database"
        log_info "Please check your DB_URL in $ENV_FILE"
        exit 1
    fi
    
    log_success "Database connection successful"
}

################################################################################
# Migration Table Management
################################################################################

create_migrations_table() {
    log_info "Ensuring migrations table exists..."
    
    psql "$DB_URL" -q <<-EOSQL 2>/dev/null
		CREATE TABLE IF NOT EXISTS migrations (
		    id SERIAL PRIMARY KEY,
		    version VARCHAR(255) NOT NULL UNIQUE,
		    filename VARCHAR(255) NOT NULL,
		    applied_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		    checksum VARCHAR(64) NOT NULL,
		    execution_time_ms INTEGER
		);
		
		CREATE INDEX IF NOT EXISTS idx_migrations_version 
		    ON migrations(version);
		CREATE INDEX IF NOT EXISTS idx_migrations_applied_at 
		    ON migrations(applied_at DESC);
	EOSQL
    
    log_success "Migration tracking table ready"
}

get_applied_migrations() {
    psql "$DB_URL" -t -c "SELECT version FROM migrations ORDER BY version;" | 
        sed 's/^[[:space:]]*//;s/[[:space:]]*$//' |
        grep -v '^$'
}

get_last_applied_migration() {
    psql "$DB_URL" -t -c "SELECT version FROM migrations ORDER BY applied_at DESC LIMIT 1;" |
        sed 's/^[[:space:]]*//;s/[[:space:]]*$//'
}

################################################################################
# Migration File Operations
################################################################################

validate_migration_dir() {
    if [[ ! -d "$MIGRATION_DIR" ]]; then
        log_error "Migration directory not found: $MIGRATION_DIR"
        exit 1
    fi

    local file_count=$(find "$MIGRATION_DIR" -maxdepth 1 -name "*.sql" | wc -l)
    if [[ $file_count -eq 0 ]]; then
        log_warning "No SQL migration files found in: $MIGRATION_DIR"
        exit 0
    fi

    log_info "Found $file_count migration file(s) in: $MIGRATION_DIR"
}

get_migration_files() {
    find "$MIGRATION_DIR" -maxdepth 1 -name "*.sql" -type f | sort
}

get_file_checksum() {
    local file="$1"
    sha256sum "$file" | awk '{print $1}'
}

extract_version_from_filename() {
    local filename="$1"
    basename "$filename" .sql
}

################################################################################
# Migration Status
################################################################################

show_migration_status() {
    log_info "Migration Status"
    echo "==============================================="
    
    local applied_migrations=$(get_applied_migrations)
    
    while IFS= read -r file; do
        local version=$(extract_version_from_filename "$file")
        local status
        
        if echo "$applied_migrations" | grep -q "^${version}$"; then
            status="${GREEN}✓ Applied${NC}"
        else
            status="${YELLOW}○ Pending${NC}"
        fi
        
        echo -e "  $status  $version"
    done < <(get_migration_files)
    
    echo "==============================================="
    
    local total=$(get_migration_files | wc -l)
    local applied=0
    if [[ -n "$applied_migrations" ]]; then
        applied=$(echo "$applied_migrations" | grep -c . || echo 0)
    fi
    local pending=$((total - applied))
    
    echo -e "Total: $total | Applied: ${GREEN}$applied${NC} | Pending: ${YELLOW}$pending${NC}"
}

################################################################################
# Migration Application
################################################################################

apply_migration() {
    local file="$1"
    local version=$(extract_version_from_filename "$file")
    local filename=$(basename "$file")
    local checksum=$(get_file_checksum "$file")
    
    log_info "Applying migration: $version"
    
    if [[ "$DRY_RUN" == true ]]; then
        log_info "[DRY RUN] Would execute: $filename"
        echo "---"
        head -n 10 "$file"
        echo "..."
        return 0
    fi
    
    # Start timing
    local start_time=$(date +%s%3N)
    
    # Execute migration in transaction
    if ! psql "$DB_URL" <<-EOSQL
		BEGIN;
		
		-- Execute the migration
		\i $file
		
		-- Record in migrations table
		INSERT INTO migrations (version, filename, checksum, execution_time_ms)
		VALUES ('$version', '$filename', '$checksum', 0);
		
		COMMIT;
	EOSQL
    then
        log_error "Migration failed: $version"
        log_error "Transaction rolled back automatically"
        return 1
    fi
    
    # Calculate execution time
    local end_time=$(date +%s%3N)
    local execution_time=$((end_time - start_time))
    
    # Update execution time
    psql "$DB_URL" -c "UPDATE migrations SET execution_time_ms = $execution_time WHERE version = '$version';" > /dev/null
    
    log_success "Applied migration: $version (${execution_time}ms)"
}

apply_pending_migrations() {
    log_info "Checking for pending migrations..."
    
    local applied_migrations=$(get_applied_migrations)
    local pending_count=0
    
    while IFS= read -r file; do
        local version=$(extract_version_from_filename "$file")
        
        # Check if already applied
        if echo "$applied_migrations" | grep -q "^${version}$"; then
            if [[ "$FORCE_APPLY" == true ]]; then
                log_warning "Force re-applying migration: $version"
                # Delete record first
                psql "$DB_URL" -c "DELETE FROM migrations WHERE version = '$version';" > /dev/null
            else
                log_info "Skipping already applied: $version"
                continue
            fi
        fi
        
        pending_count=$((pending_count + 1))
        
        if ! apply_migration "$file"; then
            log_error "Migration failed, stopping"
            exit 1
        fi
    done < <(get_migration_files)
    
    if [[ $pending_count -eq 0 ]]; then
        log_success "No pending migrations. Database is up to date!"
    else
        log_success "Applied $pending_count migration(s) successfully!"
    fi
}

################################################################################
# Rollback Operations
################################################################################

rollback_migrations() {
    local count="$1"
    
    log_warning "Rolling back last $count migration(s)..."
    log_warning "This will drop tables, functions, and data!"
    
    if [[ "$DRY_RUN" == false ]]; then
        read -p "Are you sure? (yes/no): " -r
        if [[ ! $REPLY =~ ^[Yy][Ee][Ss]$ ]]; then
            log_info "Rollback cancelled"
            exit 0
        fi
    fi
    
    local migrations=$(psql "$DB_URL" -t -c "SELECT version FROM migrations ORDER BY applied_at DESC LIMIT $count;")
    
    while IFS= read -r version; do
        version=$(echo "$version" | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')
        [[ -z "$version" ]] && continue
        
        if [[ "$DRY_RUN" == true ]]; then
            log_info "[DRY RUN] Would rollback: $version"
        else
            log_warning "Rolling back: $version"
            psql "$DB_URL" -c "DELETE FROM migrations WHERE version = '$version';" > /dev/null
            log_success "Rolled back: $version"
            log_warning "Note: Data changes are NOT reverted automatically"
        fi
    done <<< "$migrations"
}

################################################################################
# Main Execution
################################################################################

main() {
    parse_args "$@"
    
    log_info "Database Migration Tool"
    log_info "======================="
    
    # Load configuration
    load_env
    check_psql
    test_db_connection
    
    # Ensure migrations table exists
    if [[ "$DRY_RUN" == false ]]; then
        create_migrations_table
    fi
    
    # Validate migration directory
    validate_migration_dir
    
    # Execute requested operation
    if [[ "$SHOW_STATUS" == true ]]; then
        show_migration_status
    elif [[ $ROLLBACK_COUNT -gt 0 ]]; then
        rollback_migrations "$ROLLBACK_COUNT"
    else
        apply_pending_migrations
    fi
    
    log_success "Migration process completed"
}

# Run main function
main "$@"
