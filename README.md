# hyperledger-fabric-400-gossip
hyperledger fabric block propagation in less than 700 lines of Go

# tutorial
CODE: https://hamait.tistory.com/1012

DOCKER : https://hamait.tistory.com/1019

# run 
leader peer : go run peer.go -name 127.0.0.1:28000 -ip 127.0.0.1 -port 28000 -leader

non-leader peer : go run peer.go -name 127.0.0.1:28001 -ip 127.0.0.1 -port 28001 -bootstrap 127.0.0.1:28000

# run by docker on host network
docker run -p 28000:28000 --net host --name  gossip1 --rm gossip /go/bin/gossip -name  127.0.0.1:28000 -ip  127.0.0.1 -port 28000 -leader

docker run -p 28001:28001 --net host --name  gossip2 --rm gossip /go/bin/gossip -name  127.0.0.1:28001 -ip  127.0.0.1 -port 28001  -bootstrap   127.0.0.1:28000

docker run -p 28001:28002 --net host --name  gossip3 --rm gossip /go/bin/gossip -name  127.0.0.1:28002 -ip  127.0.0.1 -port 28002  -bootstrap   127.0.0.1:28000



# run by docker on bridge network
docker run -it --network=gossip-network --name gossip1  gossip /go/bin/gossip -name  gossip1:28000 -ip gossip1 -port 28000 -leader

docker run -it --network=gossip-network --name gossip2  gossip /go/bin/gossip -name  gossip2:28001 -ip gossip2 -port 28001  -bootstrap  gossip1:28000

docker run -it --network=gossip-network --name gossip3  gossip /go/bin/gossip -name  gossip2:28002 -ip gossip3 -port 28002  -bootstrap  gossip1:28000

# run by docker compose
docker-compose -p gossipnetwork up -d
