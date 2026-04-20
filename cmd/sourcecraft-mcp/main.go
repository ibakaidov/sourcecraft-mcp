package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/aacidov/sourcecraft-mcp/internal/mcpserver"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	ctx := context.Background()

	if len(os.Args) > 1 && os.Args[1] == "http" {
		if err := runHTTP(ctx, os.Args[2:]); err != nil {
			log.Fatal(err)
		}
		return
	}

	server, err := mcpserver.NewFromEnv(".")
	if err != nil {
		log.Fatal(err)
	}
	if err := server.Run(ctx, &mcp.StdioTransport{}); err != nil {
		log.Fatal(err)
	}
}

func runHTTP(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("http", flag.ContinueOnError)
	listen := fs.String("listen", "127.0.0.1:8080", "listen address")
	path := fs.String("path", "/mcp", "HTTP path")
	if err := fs.Parse(args); err != nil {
		return err
	}

	server, err := mcpserver.NewFromEnv(".")
	if err != nil {
		return err
	}

	handler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		return server
	}, &mcp.StreamableHTTPOptions{})

	mux := http.NewServeMux()
	mux.Handle(*path, handler)

	httpServer := &http.Server{
		Addr:    *listen,
		Handler: mux,
	}

	fmt.Fprintf(os.Stderr, "sourcecraft-mcp listening on http://%s%s\n", *listen, *path)
	return httpServer.ListenAndServe()
}
