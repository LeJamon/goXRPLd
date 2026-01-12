#!/bin/bash
# Script to generate Go code from proto files
#
# Prerequisites:
#   1. Install protoc (Protocol Buffers compiler):
#      - macOS: brew install protobuf
#      - Linux: apt-get install protobuf-compiler
#      - Or download from: https://github.com/protocolbuffers/protobuf/releases
#
#   2. Install Go plugins for protoc:
#      go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
#      go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
#
#   3. Ensure $GOPATH/bin is in your PATH:
#      export PATH="$PATH:$(go env GOPATH)/bin"

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROTO_DIR="${SCRIPT_DIR}/org/xrpl/rpc/v1"
OUT_DIR="${SCRIPT_DIR}/../v1"

# Create output directory
mkdir -p "${OUT_DIR}"

# Generate Go code
protoc \
    --proto_path="${SCRIPT_DIR}" \
    --go_out="${OUT_DIR}" \
    --go_opt=paths=source_relative \
    --go-grpc_out="${OUT_DIR}" \
    --go-grpc_opt=paths=source_relative \
    "${PROTO_DIR}/ledger.proto" \
    "${PROTO_DIR}/get_ledger.proto" \
    "${PROTO_DIR}/get_ledger_entry.proto" \
    "${PROTO_DIR}/get_ledger_data.proto" \
    "${PROTO_DIR}/get_ledger_diff.proto" \
    "${PROTO_DIR}/xrp_ledger.proto"

echo "Go code generated successfully in ${OUT_DIR}"
