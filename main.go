package main

import (
	"fmt"
	"github.com/hashicorp/raft"
	"github.com/hashicorp/raft-boltdb"
	"io"
	"log"
	"net"
	"os"
	"time"
)

type fsm struct {
	ID string
}

func (s *fsm) Apply(l *raft.Log) interface{} {
	log.Printf("fsm.Apply called with: %v", l)
	return nil
}

func (s *fsm) Snapshot() (raft.FSMSnapshot, error) {
	log.Println("fsm.Snapshot called")
	return &fsmSnapshot{}, nil
}

func (s *fsm) Restore(old io.ReadCloser) error {
	log.Println("fs.Restore called")
	return nil
}

type fsmSnapshot struct {
	ID string
}

func (s *fsmSnapshot) Persist(sink raft.SnapshotSink) error {
	log.Println("fsmSnapshot.Persist called")
	return nil
}

func (s *fsmSnapshot) Release() {
	log.Println("fsmSnapsnot.Release called")
}

func main() {
	conf := raft.DefaultConfig()
	conf.ShutdownOnRemove = true
	conf.EnableSingleNode = true
	conf.LogOutput = os.Stdout

	port := os.Args[1]

	// Create the backend raft store for logs and stable storage
	store, err := raftboltdb.NewBoltStore("./data/raft.db")
	if err != nil {
		log.Fatal(err)
	}

	// Wrap the store in a LogCache to improve performance
	cacheStore, err := raft.NewLogCache(512, store)
	if err != nil {
		store.Close()
		log.Fatal(err)
	}

	// Create the snapshot store
	snapshots, err := raft.NewFileSnapshotStore("./snapshots", 2, os.Stdout)
	if err != nil {
		store.Close()
		log.Fatal(err)
	}
	// Create a transport layer
	advertiseAddr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("localhost:%s", port))
	if err != nil {
		log.Fatal(err)
	}
	bindAddr := fmt.Sprintf("0.0.0.0:%s", port)
	trans, err := raft.NewTCPTransport(bindAddr, advertiseAddr, 1, 5*time.Second, os.Stdout)
	if err != nil {
		store.Close()
		log.Fatal(err)
	}
	// Setup the peer store
	raftPeers := raft.NewJSONPeers("./peers", trans)

	// Ensure local host is always included
	peers, err := raftPeers.Peers()
	if err != nil {
		store.Close()
		trans.Close()
		log.Fatal(err)
	}
	if !raft.PeerContained(peers, trans.LocalAddr()) {
		raftPeers.SetPeers(raft.AddUniquePeer(peers, trans.LocalAddr()))
	}

	r, err := raft.NewRaft(conf, &fsm{}, cacheStore, store, snapshots, raftPeers, trans)
	if err != nil {
		store.Close()
		trans.Close()
		log.Fatal(err)
	}
	monitorLeadership(r)
}

// monitorLeadership is used to monitor if we acquire or lose our role
// as the leader in the Raft cluster. There is some work the leader is
// expected to do, so we must react to changes
func monitorLeadership(r *raft.Raft) {
	leaderCh := r.LeaderCh()
	var stopCh chan struct{}
	for {
		select {
		case isLeader := <-leaderCh:
			if isLeader {
				stopCh = make(chan struct{})
				go leaderLoop(stopCh)
				log.Printf("cluster leadership acquired")
			} else if stopCh != nil {
				close(stopCh)
				stopCh = nil
				log.Printf("cluster leadership lost")
			}
		}
	}
}

// leaderLoop runs as long as we are the leader to run various
// maintence activities
func leaderLoop(stopCh chan struct{}) {
	log.Println("running leaderLoop")
	for {
		select {
		case <-stopCh:
			return
		}
	}
}
