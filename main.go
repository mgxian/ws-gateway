package main

import (
	"flag"
	"log"
	"net/http"
	_ "net/http/pprof"

	"github.com/mgxian/ws-gateway/gateway"
)

func main() {
	debugEnabled := flag.Bool("debug", false, "pprof debug mode")

	flag.Parse()

	if *debugEnabled {
		go func() {
			log.Println(http.ListenAndServe(":6060", nil))
		}()
	}

	authServer := &gateway.FakeAuthServer{}
	store := gateway.NewInMemeryWSClientStore()
	server := gateway.NewGatewayServer(store, authServer)
	statServer := gateway.NewStatServer(store)

	go http.ListenAndServe("127.0.0.1:6000", statServer)

	if err := http.ListenAndServe(":5000", server); err != nil {
		log.Fatalf("could not listen on port 5000 %v", err)
	}
}
