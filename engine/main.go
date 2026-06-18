// Command forksync runs the embedded HTTP server that the Electron main
// process spawns. It replaces the old Cobra CLI: there are no subcommands —
// every engine capability is exposed over REST (+ one WebSocket for streaming
// agent resolve output) on 127.0.0.1:<port>.
//
// The server prints a single FORKSYNC_HTTP_ADDR=127.0.0.1:<port> line to
// stdout once it is listening, so the Electron parent can discover the port.
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
