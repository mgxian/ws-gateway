package gateway

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gorilla/websocket"
)

type AuthMessage struct {
	MemberID int    `json:"member_id"`
	Token    string `json:"token"`
}

type PushMessage struct {
	MemberID int    `json:"member_id"`
	Text     string `json:"text"`
}

type AuthServer interface {
	Auth(int, string) bool
}

type gatewayServer struct {
	upgrader      websocket.Upgrader
	wsClientStore wsClientStore
	authServer    AuthServer
}

type wsClientStore interface {
	Save(int, *websocket.Conn) error
}

func NewGatewayServer(store wsClientStore, authServer AuthServer) *gatewayServer {
	return &gatewayServer{
		upgrader:      websocket.Upgrader{},
		wsClientStore: store,
		authServer:    authServer,
	}
}

func (g *gatewayServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/push" {
		g.websocket(w, r)
	}

	w.WriteHeader(http.StatusAccepted)
}

func (g *gatewayServer) websocket(w http.ResponseWriter, r *http.Request) {
	ws, err := g.upgrader.Upgrade(w, r, nil)
	defer ws.Close()
	if err != nil {
		return
	}

	var auth AuthMessage
	_, msg, err := ws.ReadMessage()
	if err := json.Unmarshal(msg, &auth); err != nil {
		return
	}

	g.wsClientStore.Save(auth.MemberID, ws)

	if auth.MemberID <= 0 {
		ws.WriteMessage(websocket.TextMessage, []byte(`{code:200,message:"hello stranger"}`))
		return
	}

	if !g.authServer.Auth(auth.MemberID, auth.Token) {
		ws.WriteMessage(websocket.TextMessage, []byte(`{code:401,message:"unauthorized"}`))
		return
	}

	ws.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf(`{code:200,message:"hello %d"}`, auth.MemberID)))
}
