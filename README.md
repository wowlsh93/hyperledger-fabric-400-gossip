# hyperledger-fabric-400-gossip
hyperledger fabric block propagation in less than 700 lines of Go

# tutorial
https://hamait.tistory.com/1012

# run 
leader peer : go run peer.go -name 28000 -port 28000 -leader

non-leader peer : go run peer.go -name 28001 -port 28001 -bootstrap 28000

# docker run 
docker run -p 28000:28000 --net host --name  gossip1 --rm gossip /go/bin/gossip -name 28000 -port 28000 -leader

docker run -p 28001:28001 --net host --name  gossip2 --rm gossip /go/bin/gossip -name 28001 -port 28001 -bootstrap 28000
