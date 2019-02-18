# Start from a Debian image with the latest version of Go installed
# and a workspace (GOPATH) configured at /go.
FROM golang

# Copy the local package files to the container's workspace.
ADD . /go/src/github.com/wowlsh93/hyperledger-fabric-400-gossip

# Build the gossip command inside the container.
# (You may fetch or manage dependencies here,
# either manually or with a tool like "godep".)
RUN go install github.com/wowlsh93/hyperledger-fabric-400-gossip/gossip

# run by host network
# docker run -p 28000:28000 --net host --name  gossip1 --rm gossip /go/bin/gossip -name  127.0.0.1:28000 -ip  127.0.0.1 -port 28000 -leader
# docker run -p 28001:28001 --net host --name  gossip2 --rm gossip /go/bin/gossip -name  127.0.0.1:28001 -ip  127.0.0.1 -port 28001  -bootstrap   127.0.0.1:28000
# docker run -p 28001:28002 --net host --name  gossip3 --rm gossip /go/bin/gossip -name  127.0.0.1:28002 -ip  127.0.0.1 -port 28002  -bootstrap   127.0.0.1:28000


# run by bridge network
# docker run -it --network=gossip-network --name gossip1  gossip /go/bin/gossip -name  gossip1:28000 -ip gossip1 -port 28000 -leader
# docker run -it --network=gossip-network --name gossip2  gossip /go/bin/gossip -name  gossip2:28001 -ip gossip2 -port 28001  -bootstrap  gossip1:28000
# docker run -it --network=gossip-network --name gossip3  gossip /go/bin/gossip -name  gossip2:28002 -ip gossip3 -port 28002  -bootstrap  gossip1:28000
