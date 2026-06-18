// Command forksync runs the embedded HTTP server for headless / programmatic
// access. The primary desktop UI is built with Wails (see root main.go). This
// binary exposes all engine capabilities over REST (+ WebSocket for streaming
// agent output) on 127.0.0.1:<port>.
//
// The server prints a FORKSYNC_HTTP_ADDR=127.0.0.1:<port> line to stdout once
// it is listening, so callers can discover the bound port.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/loongxjin/forksync/engine/core/app"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:0", "listen address (use :0 for a random port)")
	flag.Parse()

	if err := app.Run(*addr); err != nil {
		fmt.Fprintf(os.Stderr, "forksync: %v\n", err)
		os.Exit(1)
	}
}
