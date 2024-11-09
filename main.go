package main

import (
	"context"
	"flag"
	"fmt"
	"google.golang.org/grpc/credentials/insecure"
	"log"
	"math/rand"
	"net"
	"os"
	"sync"
	"time"

	pb "github.com/r03smus/mutual/proto"
	"google.golang.org/grpc"
)

const (
	NODE1_ADDR = "localhost:50051"
	NODE2_ADDR = "localhost:50052"
	NODE3_ADDR = "localhost:50053"
)

type Node struct {
	pb.UnimplementedMutexServiceServer
	id         int
	nodes      map[int]pb.MutexServiceClient
	nextId     int // ID of next node in ring
	hasToken   bool
	wantsToken bool
	mutex      sync.Mutex
}

// PassToken handles incoming token from previous node in ring
func (n *Node) PassToken(ctx context.Context, req *pb.TokenMessage) (*pb.TokenReply, error) {
	n.mutex.Lock()
	defer n.mutex.Unlock()
	log.Printf("Node %d received token", n.id)
	n.hasToken = true

	if n.wantsToken {
		go n.execute()
	} else {
		//Little wait, becuase if not, unreadable :3
		time.Sleep(1 * time.Second)
		go n.passTokenToNext()
	}
	return &pb.TokenReply{Ok: true}, nil
}

// Pass token to next node in ring
func (n *Node) passTokenToNext() {
	n.mutex.Lock()
	if !n.hasToken {
		n.mutex.Unlock()
		return
	}
	n.hasToken = false
	n.mutex.Unlock()
	ctx := context.WithoutCancel(context.Background())
	log.Printf("Node %d passing token to Node %d", n.id, n.nextId)
	_, err := n.nodes[n.nextId].PassToken(ctx, &pb.TokenMessage{
		FromId: int32(n.id),
	})
	if err != nil {
		log.Printf("Failed to pass token to node %d: %v", n.nextId, err)
	}
}

// Request access to Critical Section
func (n *Node) requestAccess() {
	n.mutex.Lock()
	if n.wantsToken {
		n.mutex.Unlock()
		return
	}
	n.wantsToken = true
	n.mutex.Unlock()
	log.Printf("Node %d requesting Critical Section access", n.id)

	if n.hasToken {
		go n.execute()
	}
}

// Execute Critical Section
func (n *Node) execute() {
	log.Printf("Node %d entering Critical Section", n.id)
	// Simulate work in Critical Section
	time.Sleep(3 * time.Second)
	// Write to shared file to demonstrate CS access
	f, err := os.OpenFile("criticalTxtFile", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Printf("Error opening file: %v", err)
		return
	}
	defer f.Close()

	_, err = fmt.Fprintf(f, "Node %d accessed CS at %v\n", n.id, time.Now())
	if err != nil {
		log.Printf("Error writing to file: %v", err)
	}
	n.wantsToken = false
	log.Printf("Node %d exiting Critical Section", n.id)
	// Pass token to next node
	n.passTokenToNext()
}

func startNode(id int, hasInitialToken bool) {
	// Initialize node
	node := &Node{
		id:         id,
		nodes:      make(map[int]pb.MutexServiceClient),
		hasToken:   hasInitialToken,
		wantsToken: false,
	}

	// Set up the next node in ring
	// 1 -> 2
	// 2 -> 3
	// 3 -> 1
	if id == 1 {
		node.nextId = 2
	} else if id == 2 {
		node.nextId = 3
	} else if id == 3 {
		node.nextId = 1
	}

	// Start gRPC server
	port := 50050 + id
	lis, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", port))
	if err != nil {
		log.Fatalf("Node %d failed to listen: %v", id, err)
	}

	grpcServer := grpc.NewServer()
	pb.RegisterMutexServiceServer(grpcServer, node)

	go func() {
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("Failed to serve: %v", err)
		}
	}()

	// Wait for other nodes to start
	time.Sleep(5 * time.Second)

	addresses := map[int]string{
		1: NODE1_ADDR,
		2: NODE2_ADDR,
		3: NODE3_ADDR,
	}

	for nodeId, addr := range addresses {
		if nodeId == id {
			continue
		}
		conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			log.Printf("Node %d failed to connect to node %d: %v", id, nodeId, err)
			continue
		}

		node.nodes[nodeId] = pb.NewMutexServiceClient(conn)
	}

	log.Printf("Node %d started. Next node in ring: %d", node.id, node.nextId)

	// If this node has initial token, start passing it
	if hasInitialToken {
		time.Sleep(3 * time.Second)
		go node.passTokenToNext()
	}
	//Keep requesting access to Critical file
	for {
		time.Sleep(time.Duration(2*rand.Int31n(4)) * time.Second)
		node.requestAccess()
	}
}

func main() {
	nodeID := flag.Int("id", 1, "Node ID (1, 2, or 3)")
	flag.Parse()

	if *nodeID < 1 || *nodeID > 3 {
		log.Fatalf("Invalid node ID. Must be 1, 2, or 3")
	}

	// Node 1 starts with the token
	hasInitialToken := *nodeID == 1

	startNode(*nodeID, hasInitialToken)

	select {} //This line has been taking from somewhere else because:
	//select {} is an empty select statement with no cases, which blocks forever since there are no channels to receive from or send to. This creates a program that:
	//
	//Never terminates
	//Doesn't consume CPU (it's blocked, not spinning)
	//Can still handle signals and goroutines
	//
	//It's commonly used as an idiom in long-running services or daemons to prevent the main function from exiting while background goroutines continue working.
}
