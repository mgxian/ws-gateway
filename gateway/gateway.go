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

type remoteAddr = string

// MemberWSClients store websocket connections of member
type MemberWSClients struct {
	memberID  int
	wsClients map[remoteAddr]*websocket.Conn
}

// NewMemberWSClients create a new MemberWSClients
func NewMemberWSClients(memberID int) *MemberWSClients {
	return &MemberWSClients{
		memberID:  memberID,
		wsClients: make(map[remoteAddr]*websocket.Conn),
	}
}

// Save store websocket connection of member
func (m *MemberWSClients) Save(ws *websocket.Conn) error {
	addr := ws.RemoteAddr().String()
	fmt.Println(addr)
	m.wsClients[addr] = ws
	return nil
}

func (m *MemberWSClients) WSClients() []*websocket.Conn {
	result := make([]*websocket.Conn, 0)
	for _, v := range m.wsClients {
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
func (app *APPWSClients) Save(memberID int, ws *websocket.Conn) error {
	mwsc, ok := app.memberClients[memberID]
	if !ok {
		mwsc = NewMemberWSClients(memberID)
		app.memberClients[memberID] = mwsc
	}
	return mwsc.Save(ws)
}

func (app *APPWSClients) WSClientsForMember(memberID int) []*websocket.Conn {
	memberClient, ok := app.memberClients[memberID]
	if !ok {
		return nil
	}
	return memberClient.WSClients()
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
func (wcs *WSClientStore) Save(app string, memberID int, ws *websocket.Conn) error {
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
	return appWSClient.Save(memberID, ws)
}

// PublicWSClientsForApp return public websocket connections for app
func (wcs *WSClientStore) PublicWSClientsForApp(app string) []*websocket.Conn {
	appClient, ok := wcs.appClients[app]
	if !ok {
		return nil
	}
	return appClient.WSClientsForMember(0)
}

// PrivateWSClientsForMember return private websocket connections for member
func (wcs *WSClientStore) PrivateWSClientsForMember(memberID int) []*websocket.Conn {
	app := "im"
	appClient, ok := wcs.appClients[app]
	if !ok {
		return nil
	}
	return appClient.WSClientsForMember(memberID)
}
