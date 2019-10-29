package gateway

import (
	"sync"
)

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

func (m *memberWSClients) delete(ws Conn) {
	delete(m.wsConns, ws.RemoteAddr())
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
	mcs, ok := app.memberClients[memberID]
	if !ok {
		mcs = newMemberWSClients(memberID)
		app.memberClients[memberID] = mcs
	}
	return mcs.save(ws)
}

func (app *appWSClients) delete(memberID int, ws Conn) {
	if mcs, ok := app.memberClients[memberID]; ok {
		mcs.delete(ws)
	}
}

// wsClientsForMember returns websocket connections of member
func (app *appWSClients) wsClientsForMember(memberID int) []Conn {
	mcs, ok := app.memberClients[memberID]
	if !ok {
		return nil
	}
	return mcs.wsClients()
}

// InMemeryWSClientStore store websocket connection
type InMemeryWSClientStore struct {
	appClients map[string]*appWSClients
	sync.RWMutex
}

// NewInMemeryWSClientStore create a new WSClientStore
func NewInMemeryWSClientStore() *InMemeryWSClientStore {
	store := &InMemeryWSClientStore{
		appClients: make(map[string]*appWSClients),
	}
	return store
}

// Save store websocket connection
func (wcs *InMemeryWSClientStore) save(app string, memberID int, ws Conn) error {
	wcs.Lock()
	defer wcs.Unlock()

	if app != privateApp {
		memberID = 0
	}

	if app == privateApp && memberID <= 0 {
		return nil
	}

	appWSClient, ok := wcs.appClients[app]
	if !ok {
		appWSClient = newAPPWSClients(app)
		wcs.appClients[app] = appWSClient
	}
	return appWSClient.save(memberID, ws)
}

func (wcs *InMemeryWSClientStore) delete(memberID int, ws Conn) {
	wcs.Lock()
	defer wcs.Unlock()

	for app, ac := range wcs.appClients {
		mid := memberID
		if app != privateApp {
			mid = 0
		}
		ac.delete(mid, ws)
	}
}

// publicWSClientsForApp return public websocket connections for app
func (wcs *InMemeryWSClientStore) publicWSClientsForApp(app string) []Conn {
	wcs.RLock()
	defer wcs.RUnlock()

	appClient, ok := wcs.appClients[app]
	if !ok {
		return nil
	}
	return appClient.wsClientsForMember(0)
}

// privateWSClientsForMember return private websocket connections for member
func (wcs *InMemeryWSClientStore) privateWSClientsForMember(memberID int) []Conn {
	wcs.RLock()
	defer wcs.RUnlock()

	app := privateApp
	appClient, ok := wcs.appClients[app]
	if !ok {
		return nil
	}
	return appClient.wsClientsForMember(memberID)
}
