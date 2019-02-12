# Start from a Debian image with the latest version of Go installed
# and a workspace (GOPATH) configured at /go.
FROM golang

# Copy the local package files to the container's workspace.
ADD . /go/src/github.com/wowlsh93/hyperledger-fabric-400-gossip

# Build the gossip command inside the container.
# (You may fetch or manage dependencies here,
# either manually or with a tool like "godep".)
RUN go install github.com/wowlsh93/hyperledger-fabric-400-gossip/gossip

# how to run
# docker run -p 28000:28000 --net host --name  gossip1 --rm gossip /go/bin/gossip -name 28000 -port 28000 -leader
# docker run -p 28001:28001 --net host --name  gossip2 --rm gossip /go/bin/gossip -name 28001 -port 28001 -bootstrap 28000
