package gateway

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gorilla/websocket"
)

// Conn connection interface
type Conn interface {
	ReadMessage() (msg []byte, err error)
	WriteMessage(msg []byte) (err error)
	RemoteAddr() string
}

type wsConn struct {
	conn *websocket.Conn
}

func newWSConn(conn *websocket.Conn) *wsConn {
	return &wsConn{
		conn: conn,
	}
}

func (ws *wsConn) ReadMessage() ([]byte, error) {
	_, msg, err := ws.conn.ReadMessage()
	return msg, err
}

func (ws *wsConn) WriteMessage(msg []byte) error {
	return ws.conn.WriteMessage(websocket.TextMessage, msg)
}

func (ws *wsConn) RemoteAddr() string {
	return ws.conn.RemoteAddr().String()
}

// AuthMessage client auth message
type AuthMessage struct {
	MemberID int    `json:"member_id"`
	Token    string `json:"token"`
}

// SubscribeMessage client subscribe data message
type SubscribeMessage struct {
	App string `json:"app"`
}

// PushMessage push request message
type PushMessage struct {
	APP      string `json:"app"`
	MemberID int    `json:"member_id"`
	Text     string `json:"text"`
}

// AuthServer client auth server interface
type AuthServer interface {
	Auth(int, string) bool
}

// Server websocket gateway server
type Server struct {
	upgrader      websocket.Upgrader
	wsClientStore wsClientStore
	authServer    AuthServer
}

type wsClientStore interface {
	save(app string, memberID int, ws Conn) error
	publicWSClientsForApp(app string) []Conn
	privateWSClientsForMember(memberID int) []Conn
}

// NewGatewayServer create a new gateway server
func NewGatewayServer(store wsClientStore, authServer AuthServer) *Server {
	return &Server{
		upgrader:      websocket.Upgrader{},
		wsClientStore: store,
		authServer:    authServer,
	}
}

func (g *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/push" {
		g.websocket(w, r)
	}

	var pushMsg PushMessage
	if err := json.NewDecoder(r.Body).Decode(&pushMsg); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusAccepted)

	if pushMsg.APP == "im" {
		conns := g.wsClientStore.privateWSClientsForMember(pushMsg.MemberID)
		for _, conn := range conns {
			conn.WriteMessage([]byte(pushMsg.Text))
		}
		return
	}

	conns := g.wsClientStore.publicWSClientsForApp(pushMsg.APP)
	for _, conn := range conns {
		conn.WriteMessage([]byte(pushMsg.Text))
	}
}

func (g *Server) websocket(w http.ResponseWriter, r *http.Request) {
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

func (g *Server) verifyConnection(ws *websocket.Conn) int {
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

func (g *Server) waitForSubscribe(ws *websocket.Conn, memberID int) {
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
		g.wsClientStore.save(sub.App, memberID, newWSConn(ws))
	}
}
