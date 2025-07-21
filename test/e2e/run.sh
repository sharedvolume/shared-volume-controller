#!/bin/bash

# SharedVolume Controller E2E Test Runner
# This script runs end-to-end tests for the SharedVolume controller

set -e

# Configuration
PARAMETERS_FILE="parameters.txt"
MANIFESTS_DIR="manifests"
GENERATED_DIR="generated"

# Function to substitute placeholders with actual values
substitute_placeholders() {
    local input_file="$1"
    local output_file="$2"
    
    if [[ ! -f "$PARAMETERS_FILE" ]]; then
        echo "Error: Parameters file $PARAMETERS_FILE not found"
        exit 1
    fi
    
    # Start with input file content
    cp "$input_file" "$output_file"
    
    # Read parameters and substitute
    while IFS='=' read -r key value; do
        # Skip comments and empty lines
        [[ "$key" =~ ^#.*$ ]] && continue
        [[ -z "$key" ]] && continue
        
        # Remove any trailing whitespace/comments from value
        value=$(echo "$value" | sed 's/#.*$//' | sed 's/[[:space:]]*$//')
        
        # Substitute placeholder with actual value using # as delimiter to avoid issues with URLs
        sed -i '' "s#{{$key}}#$value#g" "$output_file"
    done < "$PARAMETERS_FILE"
}

# Function to generate real files from templates (internal implementation)
generate_real_files_internal() {
    echo "Generating real YAML files from templates..."
    
    # Create generated directory
    mkdir -p "$GENERATED_DIR"
    
    # Copy directory structure
    find "$MANIFESTS_DIR" -type d | while read -r dir; do
        relative_dir=${dir#$MANIFESTS_DIR/}
        if [[ "$relative_dir" != "$MANIFESTS_DIR" ]]; then
            mkdir -p "$GENERATED_DIR/$relative_dir"
        fi
    done
    
    # Process all YAML files
    local count=0
    find "$MANIFESTS_DIR" -name "*.yaml" | while read -r file; do
        relative_file=${file#$MANIFESTS_DIR/}
        output_file="$GENERATED_DIR/$relative_file"
        
        echo "Processing: $file -> $output_file"
        substitute_placeholders "$file" "$output_file"
        count=$((count + 1))
    done
    
    echo "✅ Generated files created in $GENERATED_DIR/"
    echo "📁 Total files generated: $(find "$GENERATED_DIR" -name "*.yaml" | wc -l | tr -d ' ')"
}

# Function to generate real files from templates
generate_real_files() {
    generate_real_files_internal
}

# Function to clean generated files
cleanup_generated() {
    echo "Cleaning up generated files..."
    if [[ -d "$GENERATED_DIR" ]]; then
        rm -rf "$GENERATED_DIR"
        echo "✅ Cleaned up $GENERATED_DIR/"
    else
        echo "ℹ️  No generated directory to clean"
    fi
}

# Function to run end-to-end tests
run_e2e_tests() {
    echo "Running SharedVolume Controller E2E Tests..."
    
    # Check if kubectl is available
    if ! command -v kubectl &> /dev/null; then
        echo "❌ kubectl is not installed or not in PATH"
        exit 1
    fi
    
    # Check if we have generated files
    if [[ ! -d "$GENERATED_DIR" ]]; then
        echo "📁 No generated files found. Generating them first..."
        generate_real_files
    fi
    
    echo "🚀 Applying test manifests..."
    
    # Apply namespace manifests first
    if [[ -d "$GENERATED_DIR/ns" ]]; then
        echo "Creating test namespaces..."
        kubectl apply -f "$GENERATED_DIR/ns/"
    fi
    
    # Apply CSV manifests
    if [[ -d "$GENERATED_DIR/csv" ]]; then
        echo "Applying ClusterSharedVolume manifests..."
        kubectl apply -f "$GENERATED_DIR/csv/"
    fi
    
    # Apply SV manifests
    if [[ -d "$GENERATED_DIR/sv" ]]; then
        echo "Applying SharedVolume manifests..."
        kubectl apply -f "$GENERATED_DIR/sv/"
    fi
    
    # Apply pod manifests
    if [[ -d "$GENERATED_DIR/pod" ]]; then
        echo "Applying Pod test manifests..."
        kubectl apply -f "$GENERATED_DIR/pod/"
    fi
    
    echo "✅ E2E test manifests applied successfully"
}

# Main function
main() {
    case "${1:-}" in
        --generate)
            generate_real_files
            ;;
        --clean)
            cleanup_generated
            ;;
        --clean-generated)
            cleanup_generated
            ;;
        --run)
            run_e2e_tests
            ;;
        --test)
            run_e2e_tests
            ;;
        *)
            echo "SharedVolume Controller E2E Test Runner"
            echo "Usage: $0 [OPTION]"
            echo ""
            echo "Options:"
            echo "  --generate       Generate real YAML files from templates"
            echo "  --clean          Clean up generated files"
            echo "  --clean-generated Clean up generated files (same as --clean)"
            echo "  --run            Run E2E tests (generates files if needed)"
            echo "  --test           Same as --run"
            echo ""
            echo "Examples:"
            echo "  $0 --generate    # Create real files from templates"
            echo "  $0 --run         # Run complete E2E test suite"
            echo "  $0 --clean       # Remove generated files"
            ;;
    esac
}

# Check if script is being sourced or executed
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    main "$@"
fi