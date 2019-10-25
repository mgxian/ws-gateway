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

	if err := http.ListenAndServe(":5000", server); err != nil {
		log.Fatalf("could not listen on port 5000 %v", err)
	}
}
