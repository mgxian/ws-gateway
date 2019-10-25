package gateway

import "testing"

func TestWSClientStore(t *testing.T) {
	imApp := "im"
	chatApp := "chat"
	matchApp := "match"
	imMemberID := 123456
	notConnectedImMemberID := 123
	store := NewWSClientStore()
	ws1 := newStubWSConn("1")
	ws2 := newStubWSConn("2")
	store.save(matchApp, -1, ws1)
	store.save(matchApp, -1, ws2)
	store.save(imApp, imMemberID, ws2)

	assertWSclientCount(t, len(store.publicWSClientsForApp(chatApp)), 0)
	assertWSclientCount(t, len(store.publicWSClientsForApp(matchApp)), 2)

	assertWSclientCount(t, len(store.privateWSClientsForMember(notConnectedImMemberID)), 0)
	assertWSclientCount(t, len(store.privateWSClientsForMember(imMemberID)), 1)
}
