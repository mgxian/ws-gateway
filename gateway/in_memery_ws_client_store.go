package gateway

import (
	"sync"
)

const (
	publicAppMemberID = 0
)

type remoteAddr = string

// memberWSClients store websocket connections of member
type memberWSClients struct {
	memberID int
	wsConns  sync.Map
}

// newMemberWSClients create a new MemberWSClients
func newMemberWSClients(memberID int) *memberWSClients {
	return &memberWSClients{
		memberID: memberID,
	}
}

// save store websocket connection of member
func (m *memberWSClients) save(ws Conn) error {
	addr := ws.RemoteAddr()
	m.wsConns.Store(addr, ws)
	return nil
}

func (m *memberWSClients) delete(ws Conn) {
	m.wsConns.Delete(ws.RemoteAddr())
}

// wsClients return websocket connections of member
func (m *memberWSClients) wsClients() []Conn {
	result := make([]Conn, 0)
	m.wsConns.Range(func(k, v interface{}) bool {
		result = append(result, v.(Conn))
		return true
	})
	return result
}

// appWSClients store websocket connections of app
type appWSClients struct {
	name          string
	memberClients sync.Map
}

// newAPPWSClients create a new APPWSClients
func newAPPWSClients(name string) *appWSClients {
	return &appWSClients{
		name: name,
	}
}

// save store websocket connection of app
func (app *appWSClients) save(memberID int, ws Conn) error {
	v, ok := app.memberClients.Load(memberID)
	if !ok {
		mcs := newMemberWSClients(memberID)
		app.memberClients.Store(memberID, mcs)
		return mcs.save(ws)
	}
	return v.(*memberWSClients).save(ws)
}

func (app *appWSClients) delete(memberID int, ws Conn) {
	if v, ok := app.memberClients.Load(memberID); ok {
		mcs := v.(*memberWSClients)
		mcs.delete(ws)
		if len(mcs.wsClients()) == 0 {
			app.memberClients.Delete(memberID)
		}
	}
}

// wsClientsForMember returns websocket connections of member
func (app *appWSClients) wsClientsForMember(memberID int) []Conn {
	v, ok := app.memberClients.Load(memberID)
	if !ok {
		return nil
	}
	mcs := v.(*memberWSClients)
	return mcs.wsClients()
}

// InMemeryWSClientStore store websocket connection
type InMemeryWSClientStore struct {
	appClients sync.Map
}

// NewInMemeryWSClientStore create a new WSClientStore
func NewInMemeryWSClientStore() *InMemeryWSClientStore {
	store := &InMemeryWSClientStore{}
	return store
}

// Save store websocket connection
func (wcs *InMemeryWSClientStore) save(app string, memberID int, ws Conn) error {
	if !isPrivateApp(app) {
		memberID = publicAppMemberID
	}

	if isPrivateApp(app) && !isValidMemberID(memberID) {
		return nil
	}

	v, ok := wcs.appClients.Load(app)
	if !ok {
		appWSClient := newAPPWSClients(app)
		wcs.appClients.Store(app, appWSClient)
		return appWSClient.save(memberID, ws)
	}

	appWSClient := v.(*appWSClients)
	return appWSClient.save(memberID, ws)
}

func (wcs *InMemeryWSClientStore) delete(memberID int, ws Conn) {
	wcs.appClients.Range(func(k, v interface{}) bool {
		mid := memberID
		if !isPrivateApp(k.(string)) {
			mid = publicAppMemberID
		}
		appWSClient, ok := v.(*appWSClients)
		if ok {
			appWSClient.delete(mid, ws)
		}
		return true
	})
}

// publicWSClientsForApp return public websocket connections for app
func (wcs *InMemeryWSClientStore) publicWSClientsForApp(app string) []Conn {
	v, ok := wcs.appClients.Load(app)
	if !ok {
		return nil
	}

	appClient, ok := v.(*appWSClients)
	if !ok {
		return nil
	}

	return appClient.wsClientsForMember(0)
}

// privateWSClientsForMember return private websocket connections for member
func (wcs *InMemeryWSClientStore) privateWSClientsForMember(memberID int) []Conn {
	app := imApp
	v, ok := wcs.appClients.Load(app)
	if !ok {
		return nil
	}

	appClient, ok := v.(*appWSClients)
	if !ok {
		return nil
	}

	return appClient.wsClientsForMember(memberID)
}

func (wcs *InMemeryWSClientStore) appsWSClientCount() []wsCount {
	var result []wsCount
	wcs.appClients.Range(func(k, v interface{}) bool {
		count := 0
		app := k.(string)
		if isPrivateApp(app) {
			v, _ := wcs.appClients.Load(app)
			appClients := v.(*appWSClients)
			appClients.memberClients.Range(func(k, v interface{}) bool {
				count++
				return true
			})
		} else {
			count = len(wcs.publicWSClientsForApp(app))
		}
		result = append(result, wsCount{app, count})
		return true
	})
	return result
}

func (wcs *InMemeryWSClientStore) apps() []string {
	var result []string
	wcs.appClients.Range(func(k, v interface{}) bool {
		result = append(result, k.(string))
		return true
	})
	return result
}
