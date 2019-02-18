package main

import (
	"context"
	"log"
	"strconv"
	"time"

	pb "github.com/wowlsh93/hyperledger-fabric-400-gossip/gossip/ledgerAPI"
	"google.golang.org/grpc"
)

const (
	address     = "localhost:29000"
)

func main() {
	conn, err := grpc.Dial(address, grpc.WithInsecure())
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}
	defer conn.Close()
	c := pb.NewBlockClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()


	r, err := c.GetBlock(ctx, &pb.BlockRequest{BlockId: strconv.Itoa(6)})
	if err != nil {
		log.Fatalf("could not greet: %v", err)
	}
	log.Printf("Greeting: %s", r.BlockBody)
}
