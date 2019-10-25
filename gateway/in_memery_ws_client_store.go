package gateway

type remoteAddr = string

// memberWSClients store websocket connections of member
type memberWSClients struct {
	memberID int
	wsConns  map[remoteAddr]Conn
}

// newMemberWSClients create a new MemberWSClients
func newMemberWSClients(memberID int) *memberWSClients {
	return &memberWSClients{
		memberID: memberID,
		wsConns:  make(map[remoteAddr]Conn),
	}
}

// save store websocket connection of member
func (m *memberWSClients) save(ws Conn) error {
	addr := ws.RemoteAddr()
	m.wsConns[addr] = ws
	return nil
}

// wsClients return websocket connections of member
func (m *memberWSClients) wsClients() []Conn {
	result := make([]Conn, 0)
	for _, v := range m.wsConns {
		result = append(result, v)
	}
	return result
}

// appWSClients store websocket connections of app
type appWSClients struct {
	name          string
	memberClients map[int]*memberWSClients
}

// newAPPWSClients create a new APPWSClients
func newAPPWSClients(name string) *appWSClients {
	return &appWSClients{
		name:          name,
		memberClients: make(map[int]*memberWSClients),
	}
}

// save store websocket connection of app
func (app *appWSClients) save(memberID int, ws Conn) error {
	mwsc, ok := app.memberClients[memberID]
	if !ok {
		mwsc = newMemberWSClients(memberID)
		app.memberClients[memberID] = mwsc
	}
	return mwsc.save(ws)
}

// wsClientsForMember returns websocket connections of member
func (app *appWSClients) wsClientsForMember(memberID int) []Conn {
	memberClient, ok := app.memberClients[memberID]
	if !ok {
		return nil
	}
	return memberClient.wsClients()
}

// inMemeryWSClientStore store websocket connection
type inMemeryWSClientStore struct {
	appClients map[string]*appWSClients
}

// newInMemeryWSClientStore create a new WSClientStore
func newInMemeryWSClientStore() *inMemeryWSClientStore {
	store := &inMemeryWSClientStore{
		appClients: make(map[string]*appWSClients),
	}
	return store
}

// Save store websocket connection
func (wcs *inMemeryWSClientStore) save(app string, memberID int, ws Conn) error {
	if app != "im" {
		memberID = 0
	}

	if app == "im" && memberID <= 0 {
		return nil
	}

	appWSClient, ok := wcs.appClients[app]
	if !ok {
		appWSClient = newAPPWSClients(app)
		wcs.appClients[app] = appWSClient
	}
	return appWSClient.save(memberID, ws)
}

// PublicWSClientsForApp return public websocket connections for app
func (wcs *inMemeryWSClientStore) publicWSClientsForApp(app string) []Conn {
	appClient, ok := wcs.appClients[app]
	if !ok {
		return nil
	}
	return appClient.wsClientsForMember(0)
}

// PrivateWSClientsForMember return private websocket connections for member
func (wcs *inMemeryWSClientStore) privateWSClientsForMember(memberID int) []Conn {
	app := "im"
	appClient, ok := wcs.appClients[app]
	if !ok {
		return nil
	}
	return appClient.wsClientsForMember(memberID)
}
