package gateway

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gorilla/websocket"
)

type app = string
type memberID = int
type remoteAddr = string
type wsClient map[remoteAddr]*websocket.Conn
type memberWSClients map[memberID]wsClient

type AuthMessage struct {
	MemberID int    `json:"member_id"`
	Token    string `json:"token"`
}

type SubscribeMessage struct {
	App string `json:"app"`
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
	Save(app string, memberID int, ws *websocket.Conn) error
	PublicWSClientsForApp(app string) []*websocket.Conn
	PrivateWSClientsForMember(memberID int) []*websocket.Conn
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
	if err != nil {
		return
	}
	defer ws.Close()

	memberID := g.verifyConnection(ws)
	if memberID == -1 {
		return
	}

	g.waitForSubscribe(ws, memberID)
}

func (g *gatewayServer) verifyConnection(ws *websocket.Conn) int {
	var auth AuthMessage
	_, msg, err := ws.ReadMessage()
	if err != nil {
		return -1
	}

	if err := json.Unmarshal(msg, &auth); err != nil {
		ws.WriteMessage(websocket.TextMessage, []byte(`{code:400,message:"missing auth message"}`))
		return -1
	}

	if auth.MemberID <= 0 {
		ws.WriteMessage(websocket.TextMessage, []byte(`{code:200,message:"hello stranger"}`))
		return 0
	}

	if !g.authServer.Auth(auth.MemberID, auth.Token) {
		ws.WriteMessage(websocket.TextMessage, []byte(`{code:401,message:"unauthorized"}`))
		return 0
	}

	ws.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf(`{code:200,message:"hello %d"}`, auth.MemberID)))
	return auth.MemberID
}

func (g *gatewayServer) waitForSubscribe(ws *websocket.Conn, memberID int) {
	for {
		_, msg, err := ws.ReadMessage()
		if err != nil {
			return
		}

		var sub SubscribeMessage
		if err := json.Unmarshal(msg, &sub); err != nil {
			ws.WriteMessage(websocket.TextMessage, []byte(`{code:400,message:"bad subscribe message"}`))
			continue
		}

		subscribeSuccessMsg := fmt.Sprintf(`{code:200,message:"subscribe %s success"}`, sub.App)
		ws.WriteMessage(websocket.TextMessage, []byte(subscribeSuccessMsg))
		g.wsClientStore.Save(sub.App, memberID, ws)
	}
}
