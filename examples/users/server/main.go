// Command server serves the mock users API for the example workflows.
package main

import (
	"flag"
	"log"
	"net/http"

	"github.com/noi/dwbt/examples/users/mockapi"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8080", "address to listen on")
	flag.Parse()
	log.Printf("mock users API listening on http://%s", *addr)
	log.Fatal(http.ListenAndServe(*addr, mockapi.New()))
}
