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

type remoteAddr = string

// MemberWSClients store websocket connections of member
type MemberWSClients struct {
	memberID int
	wsConns  map[remoteAddr]Conn
}

// NewMemberWSClients create a new MemberWSClients
func NewMemberWSClients(memberID int) *MemberWSClients {
	return &MemberWSClients{
		memberID: memberID,
		wsConns:  make(map[remoteAddr]Conn),
	}
}

// Save store websocket connection of member
func (m *MemberWSClients) save(ws Conn) error {
	addr := ws.RemoteAddr()
	m.wsConns[addr] = ws
	return nil
}

// WSClients return websocket connections of member
func (m *MemberWSClients) wsClients() []Conn {
	result := make([]Conn, 0)
	for _, v := range m.wsConns {
		result = append(result, v)
	}
	return result
}

// APPWSClients store websocket connections of app
type APPWSClients struct {
	name          string
	memberClients map[int]*MemberWSClients
}

// NewAPPWSClients create a new APPWSClients
func NewAPPWSClients(name string) *APPWSClients {
	return &APPWSClients{
		name:          name,
		memberClients: make(map[int]*MemberWSClients),
	}
}

// Save store websocket connection of app
func (app *APPWSClients) save(memberID int, ws Conn) error {
	mwsc, ok := app.memberClients[memberID]
	if !ok {
		mwsc = NewMemberWSClients(memberID)
		app.memberClients[memberID] = mwsc
	}
	return mwsc.save(ws)
}

// WSClientsForMember returns websocket connections of member
func (app *APPWSClients) wsClientsForMember(memberID int) []Conn {
	memberClient, ok := app.memberClients[memberID]
	if !ok {
		return nil
	}
	return memberClient.wsClients()
}

// WSClientStore store websocket connection
type WSClientStore struct {
	appClients map[string]*APPWSClients
}

// NewWSClientStore create a new WSClientStore
func NewWSClientStore() *WSClientStore {
	store := &WSClientStore{
		appClients: make(map[string]*APPWSClients),
	}
	return store
}

// Save store websocket connection
func (wcs *WSClientStore) save(app string, memberID int, ws Conn) error {
	if app != "im" {
		memberID = 0
	}

	if app == "im" && memberID <= 0 {
		return nil
	}

	appWSClient, ok := wcs.appClients[app]
	if !ok {
		appWSClient = NewAPPWSClients(app)
		wcs.appClients[app] = appWSClient
	}
	return appWSClient.save(memberID, ws)
}

// PublicWSClientsForApp return public websocket connections for app
func (wcs *WSClientStore) publicWSClientsForApp(app string) []Conn {
	appClient, ok := wcs.appClients[app]
	if !ok {
		return nil
	}
	return appClient.wsClientsForMember(0)
}

// PrivateWSClientsForMember return private websocket connections for member
func (wcs *WSClientStore) privateWSClientsForMember(memberID int) []Conn {
	app := "im"
	appClient, ok := wcs.appClients[app]
	if !ok {
		return nil
	}
	return appClient.wsClientsForMember(memberID)
}
