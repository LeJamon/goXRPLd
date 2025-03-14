package main

import (
	"fmt"
	"github.com/LeJamon/goXRPLd/internal/server/api/jsonrpc"
	"net/http"
)

func main() {
	handler := jsonrpc.NewXRPLHandler()
	server := jsonrpc.NewServer(handler)

	http.Handle("/", server)
	fmt.Println("Starting server on :8080")
	http.ListenAndServe(":8080", nil)
}
