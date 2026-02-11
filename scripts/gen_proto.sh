#!/bin/bash
# Strix Distributed Game Server Framework - Protobuf Generator
# This script generates Go code for all .proto files with flexible directory mapping
# Supports different output directories for different proto files based on naming conventions

set -euo pipefail

# Color output for better visibility
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
PROTO_DIR="./proto"
PROTOC_OPTS=(
    --go_opt=paths=source_relative
    --go-grpc_opt=paths=source_relative
    --go-grpc_opt=require_unimplemented_servers=false
)

# Directory mapping rules based on proto file names
declare -A OUTPUT_DIRS=(
    ["strix_net.proto"]="./network/pb"          # Network-related protos to network/pb
    ["strix_mesh.proto"]="./network/pb"         # Mesh-related protos to network/pb
)

# Default directory for unmatched proto files
DEFAULT_OUTPUT_DIR="./proto"

# Function to log with timestamp
log() {
    echo -e "${GREEN}[$(date +'%Y-%m-%d %H:%M:%S')]${NC} $1"
}

error() {
    echo -e "${RED}[ERROR]${NC} $1" >&2
}

warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

# Check if protoc is available
check_dependencies() {
    if ! command -v protoc &> /dev/null; then
        error "protoc is not installed. Please install protobuf compiler."
        exit 1
    fi
    
    if ! command -v protoc-gen-go &> /dev/null; then
        error "protoc-gen-go is not installed. Run: go install google.golang.org/protobuf/cmd/protoc-gen-go@latest"
        exit 1
    fi
    
    if ! command -v protoc-gen-go-grpc &> /dev/null; then
        error "protoc-gen-go-grpc is not installed. Run: go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest"
        exit 1
    fi
}

# Get output directory for a proto file
get_output_dir() {
    local proto_file=$1
    local filename=$(basename "$proto_file")
    
    # Check if we have a specific mapping for this file
    if [[ -n "${OUTPUT_DIRS[$filename]:-}" ]]; then
        echo "${OUTPUT_DIRS[$filename]}"
        return 0
    fi
    
    # Check for pattern-based mappings
    case "$filename" in
        *net*.proto)
            echo "./net"
            ;;
        *)
            echo "$DEFAULT_OUTPUT_DIR"
            ;;
    esac
}

# Find all proto files
find_proto_files() {
    find "$PROTO_DIR" -name "*.proto" -type f | sort
}

# Check if proto file needs generation
needs_generation() {
    local proto_file=$1
    local output_dir=$2
    local base_name=$(basename "$proto_file" .proto)
    local pb_file="$output_dir/${base_name}.pb.go"
    local grpc_file="$output_dir/${base_name}_grpc.pb.go"
    
    # If output directory doesn't exist, create it
    mkdir -p "$output_dir"
    
    # If any generated file doesn't exist, regenerate
    if [[ ! -f "$pb_file" || ! -f "$grpc_file" ]]; then
        return 0
    fi
    
    # Check if proto file is newer than generated files
    if [[ "$proto_file" -nt "$pb_file" || "$proto_file" -nt "$grpc_file" ]]; then
        return 0
    fi
    
    return 1
}

# Generate protobuf for a single file
generate_proto() {
    local proto_file=$1
    local output_dir=$2
    local relative_path="${proto_file#$PROTO_DIR/}"
    
    log "Generating $(basename "$proto_file") → $output_dir/"
    
    protoc \
        "${PROTOC_OPTS[@]}" \
        --go_out="$output_dir" \
        --go-grpc_out="$output_dir" \
        --proto_path="$PROTO_DIR" \
        "$relative_path"
    
    if [[ $? -eq 0 ]]; then
        log "  ✓ Success: $(basename "$proto_file") → $output_dir/"
    else
        error "  ✗ Failed: $(basename "$proto_file")"
        return 1
    fi
}

# Clean generated files
clean_generated() {
    log "Cleaning generated files..."
    
    # Clean all possible output directories
    local all_dirs=($(printf "%s\n" "${OUTPUT_DIRS[@]}" | sort -u))
    all_dirs+=("$DEFAULT_OUTPUT_DIR")
    
    for dir in "${all_dirs[@]}"; do
        if [[ -d "$dir" ]]; then
            find "$dir" -name "*.pb.go" -o -name "*_grpc.pb.go" | xargs rm -f
            info "  Cleaned $dir/"
        fi
    done
}

# Validate generated code
validate_code() {
    log "Validating generated code..."
    
    # Check all directories that have generated files
    local dirs_to_check=()
    
    for dir in $(printf "%s\n" "${OUTPUT_DIRS[@]}" | sort -u); do
        if [[ -d "$dir" ]] && [[ -n "$(find "$dir" -name "*.pb.go" -print -quit 2>/dev/null)" ]]; then
            dirs_to_check+=("./$dir")
        fi
    done
    
    if [[ -d "$DEFAULT_OUTPUT_DIR" ]] && [[ -n "$(find "$DEFAULT_OUTPUT_DIR" -name "*.pb.go" -print -quit 2>/dev/null)" ]]; then
        dirs_to_check+=("./$DEFAULT_OUTPUT_DIR")
    fi
    
    if [[ ${#dirs_to_check[@]} -eq 0 ]]; then
        warn "No generated files found to validate"
        return 0
    fi
    
    # Build all directories with generated files
    for dir in "${dirs_to_check[@]}"; do
        info "  Validating $dir..."
        if go build -v "$dir/..."; then
            log "    ✓ $dir builds successfully"
        else
            error "    ✗ $dir has compilation errors"
            return 1
        fi
        
        # Format code in this directory
        go fmt "$dir/..."
        
        # Run go vet
        if go vet "$dir/..."; then
            log "    ✓ $dir passes go vet"
        else
            warn "    ⚠ $dir has warnings from go vet"
        fi
    done
}

# Show current mapping configuration
show_mapping() {
    log "Current directory mapping configuration:"
    for proto in "${!OUTPUT_DIRS[@]}"; do
        info "  $proto → ${OUTPUT_DIRS[$proto]}"
    done
    info "  * → $DEFAULT_OUTPUT_DIR (default)"
}

# Main generation logic
generate() {
    log "Starting protobuf generation with flexible directory mapping..."
    
    check_dependencies
    
    local proto_files=($(find_proto_files))
    
    if [[ ${#proto_files[@]} -eq 0 ]]; then
        warn "No .proto files found in $PROTO_DIR"
        exit 0
    fi
    
    log "Found ${#proto_files[@]} proto file(s)"
    
    local files_to_generate=()
    local -A dir_mapping
    
    # Build mapping of files to their output directories
    for proto_file in "${proto_files[@]}"; do
        local output_dir=$(get_output_dir "$proto_file")
        dir_mapping["$proto_file"]="$output_dir"
        
        if needs_generation "$proto_file" "$output_dir"; then
            files_to_generate+=("$proto_file")
        fi
    done
    
    if [[ ${#files_to_generate[@]} -eq 0 ]]; then
        log "All proto files are up to date"
        exit 0
    fi
    
    log "Generating ${#files_to_generate[@]} file(s)..."
    
    # Show current mapping
    show_mapping
    
    for proto_file in "${files_to_generate[@]}"; do
        generate_proto "$proto_file" "${dir_mapping[$proto_file]}"
    done
    
    validate_code
    
    log "Protobuf generation completed successfully!"
}

# Handle command line arguments
case "${1:-}" in
    --clean)
        clean_generated
        ;;
    --force)
        log "Force regenerating all proto files..."
        clean_generated
        generate
        ;;
    --mapping)
        show_mapping
        ;;
    *)
        generate
        ;;
esac
