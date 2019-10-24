package gateway

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