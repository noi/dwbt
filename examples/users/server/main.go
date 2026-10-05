// Command server serves the mock users API for the example workflows.
package main

import (
	"flag"
	"log"
	"net"
	"net/http"
	"os"

	"github.com/noi/dwbt/examples/users/mockapi"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8080", "address to listen on")
	socket := flag.String("unix", "", "path of a Unix domain socket to listen on instead of -addr")
	flag.Parse()
	if *socket == "" {
		log.Printf("mock users API listening on http://%s", *addr)
		log.Fatal(http.ListenAndServe(*addr, mockapi.New()))
	}
	os.Remove(*socket)
	ln, err := net.Listen("unix", *socket)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("mock users API listening on unix:%s", *socket)
	log.Fatal(http.Serve(ln, mockapi.New()))
}
