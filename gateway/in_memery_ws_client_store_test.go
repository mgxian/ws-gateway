package gateway

import "testing"

func TestWSClientStore(t *testing.T) {
	chatApp := "chat"
	matchApp := "match"
	imMemberID := 123456
	notConnectedImMemberID := 123
	store := NewInMemeryWSClientStore()
	ws1 := newStubWSConn("1")
	ws2 := newStubWSConn("2")
	store.save(matchApp, -1, ws1)
	store.save(matchApp, -1, ws2)
	store.save(imApp, imMemberID, ws2)

	assertWSClientCount(t, len(store.publicWSClientsForApp(chatApp)), 0)
	assertWSClientCount(t, len(store.publicWSClientsForApp(matchApp)), 2)

	assertWSClientCount(t, len(store.privateWSClientsForMember(notConnectedImMemberID)), 0)
	assertWSClientCount(t, len(store.privateWSClientsForMember(imMemberID)), 1)

	store.delete(-1, ws1)
	assertWSClientCount(t, len(store.publicWSClientsForApp(matchApp)), 1)
	assertWSClientCount(t, len(store.privateWSClientsForMember(imMemberID)), 1)

	store.delete(imMemberID, ws2)
	assertWSClientCount(t, len(store.publicWSClientsForApp(matchApp)), 0)
	assertWSClientCount(t, len(store.privateWSClientsForMember(imMemberID)), 0)
}
