# Makefile for generating Go code from proto files
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

PROTO_DIR := org/xrpl/rpc/v1
OUT_DIR := ../v1

.PHONY: all clean generate

all: generate

generate:
	@mkdir -p $(OUT_DIR)
	protoc \
		--proto_path=. \
		--go_out=$(OUT_DIR) \
		--go_opt=paths=source_relative \
		--go-grpc_out=$(OUT_DIR) \
		--go-grpc_opt=paths=source_relative \
		$(PROTO_DIR)/ledger.proto \
		$(PROTO_DIR)/get_ledger.proto \
		$(PROTO_DIR)/get_ledger_entry.proto \
		$(PROTO_DIR)/get_ledger_data.proto \
		$(PROTO_DIR)/get_ledger_diff.proto \
		$(PROTO_DIR)/xrp_ledger.proto

clean:
	rm -rf $(OUT_DIR)/*.pb.go
