package gateway

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

const (
	missingAuthMessageString        = `{"code":400,"message":"missing auth message"}`
	unauthorizedMessageString       = `{"code":401,"message":"unauthorized"}`
	badSubscribeMessageString       = `{"code":400,"message":"bad subscribe message"}`
	helloStrangerMessageString      = `{"code":200,"message":"hello stranger"}`
	helloMemberMessageFormat        = `{"code":200,"message":"hello %d"}`
	subscribeSuccessMessageFormat   = `{"code":200,"message":"subscribe %s success"}`
	subscribeForbiddenMessageFormat = `{"code":403,"message":"subscribe %s forbidden"}`
)

const (
	websocketURLPath = "/"
	pushURLPath      = "/push"

	anonymousMemberID = -1
)

var (
	privateApps = []string{"im"}
)

func isPrivateApp(app string) bool {
	for _, pa := range privateApps {
		if app == pa {
			return true
		}
	}
	return false
}

func isValidMemberID(memberID int) bool {
	if memberID > 0 {
		return true
	}
	return false
}

func missingAuthMessage() string {
	return wrapGatewayResponseMessage(missingAuthMessageString)
}

func unauthorizedMessage() string {
	return wrapGatewayResponseMessage(unauthorizedMessageString)
}

func badSubscribeMessage() string {
	return wrapGatewayResponseMessage(badSubscribeMessageString)
}

func helloStrangerMessage() string {
	return wrapGatewayResponseMessage(helloStrangerMessageString)
}

func helloMessageForMember(memberID int) string {
	return wrapGatewayResponseMessage(fmt.Sprintf(helloMemberMessageFormat, memberID))
}

func subscribeSuccessMessageForApp(app string) string {
	return wrapGatewayResponseMessage(fmt.Sprintf(subscribeSuccessMessageFormat, app))
}

func subscribeForbiddenMessageForApp(app string) string {
	return wrapGatewayResponseMessage(fmt.Sprintf(subscribeForbiddenMessageFormat, app))
}

func wrapGatewayResponseMessage(message string) string {
	pushMsg := new(PushMessage)
	pushMsg.App = "gateway"
	pushMsg.MemberID = -1
	pushMsg.Text = message
	m, _ := json.Marshal(pushMsg)
	return string(m)
}

// Conn websocket connection interface
type Conn interface {
	ReadMessage() (msg []byte, err error)
	WriteMessage(msg []byte) (err error)
	RemoteAddr() string
}

type wsStore interface {
	save(app string, memberID int, ws Conn) error
	delete(memberID int, ws Conn)
	publicWSClientsForApp(app string) []Conn
	privateWSClientsForMember(memberID int) []Conn
	apps() []string
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
	App      string `json:"app"`
	MemberID int    `json:"member_id"`
	Text     string `json:"text"`
}

// AuthServer client auth server interface
type AuthServer interface {
	Auth(int, string) bool
}

// Server websocket gateway server
type Server struct {
	http.Handler

	upgrader      websocket.Upgrader
	wsClientStore wsStore
	authServer    AuthServer
}

// NewGatewayServer create a new gateway server
func NewGatewayServer(store wsStore, authServer AuthServer) *Server {
	server := &Server{
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
		wsClientStore: store,
		authServer:    authServer,
	}

	router := http.NewServeMux()
	router.HandleFunc(websocketURLPath, server.websocket)
	router.HandleFunc(pushURLPath, server.push)

	server.Handler = router
	return server
}

func (g *Server) push(w http.ResponseWriter, r *http.Request) {
	pushMsg, err := g.bindPushMessage(r)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "bind push message error %v\n", err)
		return
	}

	w.WriteHeader(http.StatusAccepted)

	if isPrivateApp(pushMsg.App) {
		g.imMessage(pushMsg)
		return
	}

	g.publicMessage(pushMsg)
}

func (g *Server) bindPushMessage(r *http.Request) (*PushMessage, error) {
	var pushMsg PushMessage
	postData, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(postData, &pushMsg); err != nil {
		return nil, err
	}
	return &pushMsg, nil
}

func (g *Server) publicMessage(pushMsg *PushMessage) {
	msg, _ := json.Marshal(pushMsg)
	conns := g.wsClientStore.publicWSClientsForApp(pushMsg.App)
	for _, conn := range conns {
		conn.WriteMessage(msg)
	}
}

func (g *Server) imMessage(pushMsg *PushMessage) {
	msg, _ := json.Marshal(pushMsg)
	conns := g.wsClientStore.privateWSClientsForMember(pushMsg.MemberID)
	for _, conn := range conns {
		conn.WriteMessage(msg)
	}
}

func (g *Server) websocket(w http.ResponseWriter, r *http.Request) {
	ws, err := g.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer ws.Close()

	authMsg, err := g.getAuthMessage(ws)
	if err != nil {
		ws.WriteMessage(websocket.TextMessage, []byte(missingAuthMessage()))
		return
	}

	memberID := g.authMember(ws, authMsg)
	g.waitForSubscribe(ws, memberID)
}

func (g *Server) getAuthMessage(ws *websocket.Conn) (authMsg AuthMessage, err error) {
	msg, err := g.readMessageWithTimeout(ws, time.Second*10)
	err = json.Unmarshal(msg, &authMsg)
	if err != nil {
		authMsg = AuthMessage{}
	}
	if authMsg.MemberID == 0 {
		err = fmt.Errorf("member id is 0")
	}
	return
}

func (g *Server) readMessageWithTimeout(ws *websocket.Conn, timeout time.Duration) ([]byte, error) {
	ws.SetReadDeadline(time.Now().Add(timeout))
	_, msg, err := ws.ReadMessage()
	return msg, err
}

func (g *Server) authMember(ws *websocket.Conn, auth AuthMessage) (memberID int) {
	if !isValidMemberID(auth.MemberID) {
		ws.WriteMessage(websocket.TextMessage, []byte(helloStrangerMessage()))
		return anonymousMemberID
	}

	if !g.authServer.Auth(auth.MemberID, auth.Token) {
		ws.WriteMessage(websocket.TextMessage, []byte(unauthorizedMessage()))
		return anonymousMemberID
	}

	ws.WriteMessage(websocket.TextMessage, []byte(helloMessageForMember(auth.MemberID)))
	return auth.MemberID
}

func (g *Server) clearWSReadDeadline(ws *websocket.Conn) {
	ws.SetReadDeadline(time.Time{})
}

func (g *Server) waitForSubscribe(ws *websocket.Conn, memberID int) {
	for {
		g.clearWSReadDeadline(ws)
		_, msg, err := ws.ReadMessage()
		if err != nil {
			g.wsClientStore.delete(memberID, newWSConn(ws))
			return
		}

		var sub SubscribeMessage
		if err := json.Unmarshal(msg, &sub); err != nil {
			ws.WriteMessage(websocket.TextMessage, []byte(badSubscribeMessage()))
			continue
		}

		if !isValidMemberID(memberID) && isPrivateApp(sub.App) {
			ws.WriteMessage(websocket.TextMessage, []byte(subscribeForbiddenMessageForApp(sub.App)))
			continue
		}

		ws.WriteMessage(websocket.TextMessage, []byte(subscribeSuccessMessageForApp(sub.App)))
		g.wsClientStore.save(sub.App, memberID, newWSConn(ws))
	}
}

type wsCount struct {
	name  string
	count int
}

type statStore interface {
	appsWSClientCount() []wsCount
}

// StatServer store gateway stat
type StatServer struct {
	store statStore
}

// NewStatServer create a new statServer
func NewStatServer(store statStore) *StatServer {
	return &StatServer{
		store: store,
	}
}

func (s *StatServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	for _, wc := range s.store.appsWSClientCount() {
		fmt.Fprintf(w, "%s %d\n", wc.name, wc.count)
	}
}
