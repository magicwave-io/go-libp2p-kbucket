package kbucket

import (
	"math/rand"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/test"

	pstore "github.com/libp2p/go-libp2p-peerstore"
	"github.com/stretchr/testify/require"
)

var NoOpThreshold = float64(100 * time.Hour)

func TestPrint(t *testing.T) {
	t.Parallel()
	local := test.RandPeerIDFatal(t)
	m := pstore.NewMetrics()
	rt, err := NewRoutingTable(1, ConvertPeerID(local), time.Hour, m, NoOpThreshold)
	require.NoError(t, err)
	rt.Print()
}

// Test basic features of the bucket struct
func TestBucket(t *testing.T) {
	t.Parallel()
	testTime1 := time.Now()

	b := newBucket()

	peers := make([]peer.ID, 100)
	for i := 0; i < 100; i++ {
		peers[i] = test.RandPeerIDFatal(t)
		b.pushFront(&peerInfo{peers[i], testTime1, ConvertPeerID(peers[i])})
	}

	local := test.RandPeerIDFatal(t)
	localID := ConvertPeerID(local)

	infos := b.peers()
	require.Len(t, infos, 100)

	i := rand.Intn(len(peers))
	p := b.getPeer(peers[i])
	require.NotNil(t, p)
	require.Equal(t, peers[i], p.Id)
	require.Equal(t, ConvertPeerID(peers[i]), p.dhtId)
	require.EqualValues(t, testTime1, p.lastSuccessfulOutboundQuery)

	// mark as missing
	t2 := time.Now().Add(1 * time.Hour)
	p.lastSuccessfulOutboundQuery = t2
	p = b.getPeer(peers[i])
	require.NotNil(t, p)
	require.EqualValues(t, t2, p.lastSuccessfulOutboundQuery)

	spl := b.split(0, ConvertPeerID(local))
	llist := b.list
	for e := llist.Front(); e != nil; e = e.Next() {
		p := ConvertPeerID(e.Value.(*peerInfo).Id)
		cpl := CommonPrefixLen(p, localID)
		if cpl > 0 {
			t.Fatalf("split failed. found id with cpl > 0 in 0 bucket")
		}
	}

	rlist := spl.list
	for e := rlist.Front(); e != nil; e = e.Next() {
		p := ConvertPeerID(e.Value.(*peerInfo).Id)
		cpl := CommonPrefixLen(p, localID)
		if cpl == 0 {
			t.Fatalf("split failed. found id with cpl == 0 in non 0 bucket")
		}
	}
}

func TestRemovePeer(t *testing.T) {
	t.Parallel()
	local := test.RandPeerIDFatal(t)

	m := pstore.NewMetrics()
	rt, err := NewRoutingTable(2, ConvertPeerID(local), time.Hour, m, NoOpThreshold)
	require.NoError(t, err)

	p1, _ := rt.GenRandPeerID(0)
	p2, _ := rt.GenRandPeerID(0)
	b, err := rt.TryAddPeer(p1, true)
	require.True(t, b)
	require.NoError(t, err)
	b, err = rt.TryAddPeer(p2, true)
	require.True(t, b)
	require.NoError(t, err)

	// ensure p1 & p2 are in the RT
	require.Len(t, rt.ListPeers(), 2)
	require.Contains(t, rt.ListPeers(), p1)
	require.Contains(t, rt.ListPeers(), p2)

	// remove a peer and ensure it's not in the RT
	require.NotEmpty(t, rt.Find(p1))
	rt.RemovePeer(p1)
	require.Empty(t, rt.Find(p1))
	require.NotEmpty(t, rt.Find(p2))
}

func TestTableCallbacks(t *testing.T) {
	t.Parallel()

	local := test.RandPeerIDFatal(t)
	m := pstore.NewMetrics()
	rt, err := NewRoutingTable(10, ConvertPeerID(local), time.Hour, m, NoOpThreshold)
	require.NoError(t, err)

	peers := make([]peer.ID, 100)
	for i := 0; i < 100; i++ {
		peers[i] = test.RandPeerIDFatal(t)
	}

	pset := make(map[peer.ID]struct{})
	rt.PeerAdded = func(p peer.ID) {
		pset[p] = struct{}{}
	}
	rt.PeerRemoved = func(p peer.ID) {
		delete(pset, p)
	}

	rt.TryAddPeer(peers[0], true)
	if _, ok := pset[peers[0]]; !ok {
		t.Fatal("should have this peer")
	}

	rt.RemovePeer(peers[0])
	if _, ok := pset[peers[0]]; ok {
		t.Fatal("should not have this peer")
	}

	for _, p := range peers {
		rt.TryAddPeer(p, true)
	}

	out := rt.ListPeers()
	for _, outp := range out {
		if _, ok := pset[outp]; !ok {
			t.Fatal("should have peer in the peerset")
		}
		delete(pset, outp)
	}

	if len(pset) > 0 {
		t.Fatal("have peers in peerset that were not in the table", len(pset))
	}
}

// Right now, this just makes sure that it doesnt hang or crash
func TestTryAddPeerLoad(t *testing.T) {
	t.Parallel()

	local := test.RandPeerIDFatal(t)
	m := pstore.NewMetrics()
	rt, err := NewRoutingTable(10, ConvertPeerID(local), time.Hour, m, NoOpThreshold)
	require.NoError(t, err)

	peers := make([]peer.ID, 100)
	for i := 0; i < 100; i++ {
		peers[i] = test.RandPeerIDFatal(t)
	}

	for i := 0; i < 10000; i++ {
		rt.TryAddPeer(peers[rand.Intn(len(peers))], true)
	}

	for i := 0; i < 100; i++ {
		id := ConvertPeerID(test.RandPeerIDFatal(t))
		ret := rt.NearestPeers(id, 5)
		if len(ret) == 0 {
			t.Fatal("Failed to find node near ID.")
		}
	}
}

func TestTableFind(t *testing.T) {
	t.Parallel()

	local := test.RandPeerIDFatal(t)
	m := pstore.NewMetrics()
	rt, err := NewRoutingTable(10, ConvertPeerID(local), time.Hour, m, NoOpThreshold)
	require.NoError(t, err)

	peers := make([]peer.ID, 100)
	for i := 0; i < 5; i++ {
		peers[i] = test.RandPeerIDFatal(t)
		rt.TryAddPeer(peers[i], true)
	}

	t.Logf("Searching for peer: '%s'", peers[2])
	found := rt.NearestPeer(ConvertPeerID(peers[2]))
	if !(found == peers[2]) {
		t.Fatalf("Failed to lookup known node...")
	}
}

func TestUpdateLastSuccessfulOutboundQuery(t *testing.T) {
	local := test.RandPeerIDFatal(t)
	m := pstore.NewMetrics()
	rt, err := NewRoutingTable(10, ConvertPeerID(local), time.Hour, m, NoOpThreshold)
	require.NoError(t, err)

	p := test.RandPeerIDFatal(t)
	b, err := rt.TryAddPeer(p, true)
	require.True(t, b)
	require.NoError(t, err)

	// increment and assert
	t2 := time.Now().Add(1 * time.Hour)
	rt.UpdateLastSuccessfulOutboundQuery(p, t2)
	rt.tabLock.Lock()
	pi := rt.buckets[0].getPeer(p)
	require.NotNil(t, pi)
	require.EqualValues(t, t2, pi.lastSuccessfulOutboundQuery)
	rt.tabLock.Unlock()
}

func TestTryAddPeer(t *testing.T) {
	minThreshold := float64(24 * 1 * time.Hour)
	t.Parallel()

	local := test.RandPeerIDFatal(t)
	m := pstore.NewMetrics()
	rt, err := NewRoutingTable(2, ConvertPeerID(local), time.Hour, m, minThreshold)
	require.NoError(t, err)

	// generate 2 peers to saturate the first bucket for cpl=0
	p1, _ := rt.GenRandPeerID(0)
	b, err := rt.TryAddPeer(p1, true)
	require.NoError(t, err)
	require.True(t, b)
	p2, _ := rt.GenRandPeerID(0)
	b, err = rt.TryAddPeer(p2, true)
	require.NoError(t, err)
	require.True(t, b)
	require.Equal(t, p1, rt.Find(p1))
	require.Equal(t, p2, rt.Find(p2))

	// trying to add a peer with cpl=0 fails
	p3, _ := rt.GenRandPeerID(0)
	b, err = rt.TryAddPeer(p3, true)
	require.Equal(t, ErrPeerRejectedNoCapacity, err)
	require.False(t, b)
	require.Empty(t, rt.Find(p3))

	// however, trying to add peer with cpl=1 works
	p4, _ := rt.GenRandPeerID(1)
	b, err = rt.TryAddPeer(p4, true)
	require.NoError(t, err)
	require.True(t, b)
	require.Equal(t, p4, rt.Find(p4))

	// adding a peer with cpl 0 works if an existing peer has lastSuccessfulOutboundQuery above the max threshold
	// because that existing peer will get replaced
	require.True(t, rt.UpdateLastSuccessfulOutboundQuery(p2, time.Now().AddDate(0, 0, -1)))
	b, err = rt.TryAddPeer(p3, true)
	require.NoError(t, err)
	require.True(t, b)
	require.Equal(t, p3, rt.Find(p3))
	require.Equal(t, p1, rt.Find(p1))
	// p2 has been removed
	require.Empty(t, rt.Find(p2))

	// however adding peer fails if below threshold
	p5, err := rt.GenRandPeerID(0)
	require.NoError(t, err)
	require.True(t, rt.UpdateLastSuccessfulOutboundQuery(p1, time.Now()))
	b, err = rt.TryAddPeer(p5, true)
	require.Error(t, err)
	require.False(t, b)

	// adding non query peer
	p6, err := rt.GenRandPeerID(3)
	require.NoError(t, err)
	b, err = rt.TryAddPeer(p6, false)
	require.NoError(t, err)
	require.True(t, b)
	rt.tabLock.Lock()
	pi := rt.buckets[rt.bucketIdForPeer(p6)].getPeer(p6)
	require.NotNil(t, p6)
	require.True(t, pi.lastSuccessfulOutboundQuery.IsZero())
	rt.tabLock.Unlock()

}

func TestTableFindMultiple(t *testing.T) {
	t.Parallel()

	local := test.RandPeerIDFatal(t)
	m := pstore.NewMetrics()
	rt, err := NewRoutingTable(20, ConvertPeerID(local), time.Hour, m, NoOpThreshold)
	require.NoError(t, err)

	peers := make([]peer.ID, 100)
	for i := 0; i < 18; i++ {
		peers[i] = test.RandPeerIDFatal(t)
		rt.TryAddPeer(peers[i], true)
	}

	t.Logf("Searching for peer: '%s'", peers[2])
	found := rt.NearestPeers(ConvertPeerID(peers[2]), 15)
	if len(found) != 15 {
		t.Fatalf("Got back different number of peers than we expected.")
	}
}

func assertSortedPeerIdsEqual(t *testing.T, a, b []peer.ID) {
	t.Helper()
	if len(a) != len(b) {
		t.Fatal("slices aren't the same length")
	}
	for i, p := range a {
		if p != b[i] {
			t.Fatalf("unexpected peer %d", i)
		}
	}
}

func TestTableFindMultipleBuckets(t *testing.T) {
	t.Parallel()

	local := test.RandPeerIDFatal(t)
	localID := ConvertPeerID(local)
	m := pstore.NewMetrics()

	rt, err := NewRoutingTable(5, localID, time.Hour, m, NoOpThreshold)
	require.NoError(t, err)

	peers := make([]peer.ID, 100)
	for i := 0; i < 100; i++ {
		peers[i] = test.RandPeerIDFatal(t)
		rt.TryAddPeer(peers[i], true)
	}

	targetID := ConvertPeerID(peers[2])

	closest := SortClosestPeers(rt.ListPeers(), targetID)
	targetCpl := CommonPrefixLen(localID, targetID)

	// split the peers into closer, same, and further from the key (than us).
	var (
		closer, same, further []peer.ID
	)
	var i int
	for i = 0; i < len(closest); i++ {
		cpl := CommonPrefixLen(ConvertPeerID(closest[i]), targetID)
		if targetCpl >= cpl {
			break
		}
	}
	closer = closest[:i]

	var j int
	for j = len(closer); j < len(closest); j++ {
		cpl := CommonPrefixLen(ConvertPeerID(closest[j]), targetID)
		if targetCpl > cpl {
			break
		}
	}
	same = closest[i:j]
	further = closest[j:]

	// should be able to find at least 30
	// ~31 (logtwo(100) * 5)
	found := rt.NearestPeers(targetID, 20)
	if len(found) != 20 {
		t.Fatalf("asked for 20 peers, got %d", len(found))
	}

	// We expect this list to include:
	// * All peers with a common prefix length > than the CPL between our key
	//   and the target (peers in the target bucket).
	// * Then a subset of the peers with the _same_ CPL (peers in buckets to the right).
	// * Finally, other peers to the left of the target bucket.

	// First, handle the peers that are _closer_ than us.
	if len(found) <= len(closer) {
		// All of our peers are "closer".
		assertSortedPeerIdsEqual(t, found, closer[:len(found)])
		return
	}
	assertSortedPeerIdsEqual(t, found[:len(closer)], closer)

	// We've worked through closer peers, let's work through peers at the
	// "same" distance.
	found = found[len(closer):]

	// Next, handle peers that are at the same distance as us.
	if len(found) <= len(same) {
		// Ok, all the remaining peers are at the same distance.
		// unfortunately, that means we may be missing peers that are
		// technically closer.

		// Make sure all remaining peers are _somewhere_ in the "closer" set.
		pset := peer.NewSet()
		for _, p := range same {
			pset.Add(p)
		}
		for _, p := range found {
			if !pset.Contains(p) {
				t.Fatalf("unexpected peer %s", p)
			}
		}
		return
	}
	assertSortedPeerIdsEqual(t, found[:len(same)], same)

	// We've worked through closer peers, let's work through the further
	// peers.
	found = found[len(same):]

	// All _further_ peers will be correctly sorted.
	assertSortedPeerIdsEqual(t, found, further[:len(found)])

	// Finally, test getting _all_ peers. These should be in the correct
	// order.

	// Ok, now let's try finding all of them.
	found = rt.NearestPeers(ConvertPeerID(peers[2]), 100)
	if len(found) != rt.Size() {
		t.Fatalf("asked for %d peers, got %d", rt.Size(), len(found))
	}

	// We should get _all_ peers in sorted order.
	for i, p := range found {
		if p != closest[i] {
			t.Fatalf("unexpected peer %d", i)
		}
	}
}

// Looks for race conditions in table operations. For a more 'certain'
// test, increase the loop counter from 1000 to a much higher number
// and set GOMAXPROCS above 1
func TestTableMultithreaded(t *testing.T) {
	t.Parallel()

	local := peer.ID("localPeer")
	m := pstore.NewMetrics()
	tab, err := NewRoutingTable(20, ConvertPeerID(local), time.Hour, m, NoOpThreshold)
	require.NoError(t, err)
	var peers []peer.ID
	for i := 0; i < 500; i++ {
		peers = append(peers, test.RandPeerIDFatal(t))
	}

	done := make(chan struct{})
	go func() {
		for i := 0; i < 1000; i++ {
			n := rand.Intn(len(peers))
			tab.TryAddPeer(peers[n], true)
		}
		done <- struct{}{}
	}()

	go func() {
		for i := 0; i < 1000; i++ {
			n := rand.Intn(len(peers))
			tab.TryAddPeer(peers[n], true)
		}
		done <- struct{}{}
	}()

	go func() {
		for i := 0; i < 1000; i++ {
			n := rand.Intn(len(peers))
			tab.Find(peers[n])
		}
		done <- struct{}{}
	}()
	<-done
	<-done
	<-done
}

func BenchmarkAddPeer(b *testing.B) {
	b.StopTimer()
	local := ConvertKey("localKey")
	m := pstore.NewMetrics()
	tab, err := NewRoutingTable(20, local, time.Hour, m, NoOpThreshold)
	require.NoError(b, err)

	var peers []peer.ID
	for i := 0; i < b.N; i++ {
		peers = append(peers, test.RandPeerIDFatal(b))
	}

	b.StartTimer()
	for i := 0; i < b.N; i++ {
		tab.TryAddPeer(peers[i], true)
	}
}

func BenchmarkFinds(b *testing.B) {
	b.StopTimer()
	local := ConvertKey("localKey")
	m := pstore.NewMetrics()
	tab, err := NewRoutingTable(20, local, time.Hour, m, NoOpThreshold)
	require.NoError(b, err)

	var peers []peer.ID
	for i := 0; i < b.N; i++ {
		peers = append(peers, test.RandPeerIDFatal(b))
		tab.TryAddPeer(peers[i], true)
	}

	b.StartTimer()
	for i := 0; i < b.N; i++ {
		tab.Find(peers[i])
	}
}
