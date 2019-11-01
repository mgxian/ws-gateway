package main

import (
	"log"
	"net/http"

	"github.com/mgxian/ws-gateway/gateway"
)

func main() {
	authServer := &gateway.FakeAuthServer{}
	store := gateway.NewInMemeryWSClientStore()
	server := gateway.NewGatewayServer(store, authServer)
	statServer := gateway.NewStatServer(store)

	go http.ListenAndServe("127.0.0.1:6000", statServer)

	if err := http.ListenAndServe(":5000", server); err != nil {
		log.Fatalf("could not listen on port 5000 %v", err)
	}
}
