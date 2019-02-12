# hyperledger-fabric-400-gossip
hyperledger fabric block propagation in less than 700 lines of Go

# tutorial
https://hamait.tistory.com/1012

# run 
leader peer : go run peer.go -name 127.0.0.1:28000 -ip 127.0.0.1 -port 28000 -leader

non-leader peer : go run peer.go -name 127.0.0.1:28001 -ip 127.0.0.1 -port 28001 -bootstrap 127.0.0.1:28000
