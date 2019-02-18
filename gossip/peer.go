package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	"log"
	"math/rand"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	pb "github.com/wowlsh93/hyperledger-fabric-400-gossip/gossip/ledgerAPI"
)

const (
	maxActiveDialTasks = 10
)

type DiscReason uint

const (
	DiscNetworkError DiscReason = iota
)

var (
	zero16           = make([]byte, 16)
	errServerStopped = errors.New("server stopped")
)

func contains(s []int, e int) bool {
	for _, a := range s {
		if a == e {
			return true
		}
	}
	return false
}

func selectRandom3(peers map[string]*Peer) map[string]*Peer {
	if len(peers) <= 3 {
		return peers
	}
	selectedPeers := make(map[string]*Peer)

	unique := make([]int, 0, 3)

	for {
		ranNum := rand.Intn(len(peers)-1) + 1
		if !contains(unique, ranNum) {
			unique = append(unique, ranNum)
			if len(unique) == 3 {
				break
			}
		}
	}

	var i = 1
	for _, peer := range peers {
		if contains(unique, i) {
			fmt.Printf("[PeerManager] to [%s] \n", peer.name)
			selectedPeers[peer.name] = peer

		}
		i++
	}
	return selectedPeers
}

// ================================= Msg ================================//
type Msg struct {
	Code    string
	Payload string
}

// ============================== blockchain ================================//


var block_mutex = &sync.RWMutex{}


type blockchain struct {
	blocks map[string]string
}

func (b *blockchain) contains(id string) bool {
	block_mutex.RLock()
	defer block_mutex.RUnlock()
	if _, ok := b.blocks[id]; ok {
		return true
	}

	return false
}

func (b *blockchain) getBlockInfo(id string) string {
	block_mutex.RLock()
	defer block_mutex.RUnlock()
	if value, ok := b.blocks[id]; ok {
		return value
	}
	return "there is no block"
}

func (b *blockchain) add(id string, body string) {
	block_mutex.Lock()
	defer block_mutex.Unlock()
	b.blocks[id] = body
	}

// ================================= conn ================================//

type conn struct {
	name string
	fd   net.Conn
	cont chan error
}

func (c *conn) close(err error) {
	c.fd.Close()
}

// ================================= Peer ================================//

type Peer struct {
	name     string
	c        *conn
	node     *Node
	bc       *blockchain
	isLeader bool

	in       chan Msg // receives read messages
	protoErr chan error
	closed   chan struct{}

	wg sync.WaitGroup
}

func (p *Peer) run() {

	var (
		readErr = make(chan error, 1)
		reason  DiscReason
	)
	p.wg.Add(1)
	go p.readLoop(readErr)

loop:
	for {
		select {
		case err := <-readErr:
			if err != nil {
				reason = DiscNetworkError
				fmt.Printf("readErr %s. \n", reason)
				break loop
			}
		}
	}

	close(p.closed)
	fmt.Println(reason)
	p.wg.Wait()
}

//여기서는 간단하게 \n 구분자로 패킷을 정의했지만, 보다 큰 패킷의 경우에는 사이즈를 헤더에 포함시키는 방법등 다른 방식으로 한다.
func (p *Peer) writeMsg(msg Msg) error {
	p.c.fd.SetWriteDeadline(time.Now().Add(1 * time.Second))
	packet := msg.Code + "|" + msg.Payload + "\n"
	_, err := p.c.fd.Write([]byte(packet))

	if err != nil {
		//fmt.Printf("write err %s. \n", err)
		return err
	}

	//fmt.Printf("[Peer] writeMsg : %s [From]%s [To]%s \n", packet, p.node.name ,p.name)
	return nil
}

func (p *Peer) ReadMsg() (Msg, error) {
	msg := Msg{}
	// read the header
	reader := bufio.NewReader(p.c.fd)
	packet, err := reader.ReadString('\n')

	if err != nil {
		//fmt.Printf("[Peer] ReadMsg err : [%s] \n", err)
		return msg, err
	}
	packet = strings.TrimSuffix(packet, "\n")
	splited_packet := strings.Split(packet, "|")
	//fmt.Printf("[Peer] ReadMsg : [%s] \n", packet)

	msg.Code = splited_packet[0]
	msg.Payload = splited_packet[1]

	return msg, nil
}

func (p *Peer) readLoop(errc chan<- error) {
	defer p.wg.Done()
	for {
		msg, err := p.ReadMsg()
		if err != nil {
			errc <- err
			return
		}
		if err = p.handle(msg); err != nil {
			errc <- err
			return
		}
	}
}

func (p *Peer) handle(msg Msg) error {
	switch {
	case msg.Code == "alive":
		p.node.aliveSig <- msg
	case msg.Code == "block":
		p.recvBlock(msg)
	case msg.Code == "collect":
		p.collect()
	case msg.Code == "collected":
		p.collected(msg.Payload)
	}
	return nil
}

// 리모트 peer로 부터 내 피어 정보가 요청됨.
func (p *Peer) collect() {

	fmt.Printf("[Peer] collect \n")
	cs := make(chan string)
	p.node.pm.collectSig <- cs
	collected_peers := <-cs

	msg := Msg{Code: "collected", Payload: collected_peers}
	p.writeMsg(msg)
}

// 리모트 peer의 피어 정보를 받음
func (p *Peer) collected(collected string) {
	fmt.Printf("[Peer] collected [%s] \n", collected)
	p.node.pm.collectedSig <- collected
}

//블록 받으면 이웃노드들중 랜덤 3개에 전파.
func (p *Peer) propagate(msg Msg) {
	fmt.Printf("[Peer] propagate [%s] \n", msg.Payload)
	p.node.pm.propagationBlockSig <- msg
}

//받은 블록를 기록해두고, 다시 전파한다. 만약 똑같은 것을 받으면 패스 (같은 블 처리를 방지)
func (p *Peer) recvBlock(msg Msg) {
	fmt.Printf("[Peer] recvBlock %s\n", msg.Payload)
	splitedBlock := strings.Split(msg.Payload, "-")
	id := splitedBlock[1]
	if p.bc.contains(id) == false {
		fmt.Println(msg.Payload)
		p.bc.add(splitedBlock[1], splitedBlock[0])
		p.propagate(msg)

	}
}

func newPeer(n *Node, c *conn) *Peer {
	fmt.Printf("[Peer] new peer is agreed [%s] \n", c.name)
	p := &Peer{
		node:     n,
		name:     c.name,
		c:        c,
		in:       make(chan Msg), // receives read messages
		protoErr: make(chan error),
		closed:   make(chan struct{}),
		bc:       n.blockchain,
	}
	return p
}

// ================================= discovery & peer manager ================================//
type PeerManager struct {
	node  *Node
	peers map[string]*Peer
	dial  *Dialer

	tick_cllect *time.Ticker

	aliveSig            chan Msg
	propagationBlockSig chan Msg
	collectedSig        chan string
	collectSig          chan chan string
	addPeer             chan *Peer

	stop chan struct{}
}

func (pm *PeerManager) Start() {

	pm.aliveSig = make(chan Msg)
	pm.propagationBlockSig = make(chan Msg)
	pm.collectedSig = make(chan string)
	pm.collectSig = make(chan chan string)
	pm.addPeer = make(chan *Peer)
	pm.stop = make(chan struct{})

	if !pm.node.isLeader {
		pm.boostrap(pm.node.bootstrap)
	}
	go pm.run()
	go pm.collect()
}

func (pm *PeerManager) Stop() {
	pm.tick_cllect.Stop()
	pm.stop <- struct{}{}
}
func (pm *PeerManager) run() {
	for {
		select {
		case aliveMsg := <-pm.aliveSig:
			fmt.Println(aliveMsg)
			break
		case peer := <-pm.addPeer:
			pm.peers[peer.name] = peer
		case msg := <-pm.propagationBlockSig:
			pm.propagationBlock(msg)
		case collchan := <-pm.collectSig:
			collchan <- pm.getCollect()
		case peersInfo := <-pm.collectedSig:
			pm.collected(peersInfo)
		case <-pm.stop:
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (pm *PeerManager) propagationBlock(msg Msg) {
	fmt.Println("[PeerManager] propagationBlock ===================================")
	selectedPeer := selectRandom3(pm.peers)
	for _, peer := range selectedPeer {
		peer.writeMsg(msg)
	}

	fmt.Println("==================================================================")
}

func (pm *PeerManager) boostrap(address string) {
	fmt.Println("[PeerManager] boostrap")
	pm.dial.addDial(address)
}

// 30초에 한번씩 리모트 peer들 피어 정보를 요청
func (pm *PeerManager) collect() {
	fmt.Println("[PeerManager] collect start ******************************************")
	pm.tick_cllect = time.NewTicker(time.Second * 30)
	go func() {
		for t := range pm.tick_cllect.C {
			fmt.Printf("Collect Tick [%s]***********************************\n", t.String())

			if len(pm.peers) <= 3 {
				for _, peer := range pm.peers {
					msg := Msg{Code: "collect"}
					peer.writeMsg(msg)
				}
			} else {
				selectedPeer := selectRandom3(pm.peers)
				msg := Msg{Code: "collect"}
				for _, peer := range selectedPeer {
					peer.writeMsg(msg)
				}

			}
			fmt.Println("*********************************************************")

		}
	}()
}

// 리모트 peer의 피어 정보를 받아서 처리
func (pm *PeerManager) collected(peersInfo string) {
	fmt.Println("[PeerManager] collected")
	peersAddress := strings.Split(peersInfo, ",")
	for _, address := range peersAddress {
		fmt.Printf("[PeerManager] collected address [%s] \n", address)
		//새로운 Peer 정보가 자기 자신이 아니고, 이미 연결된 피어가 아니라면 새 연결
		_, ok := pm.peers[address]
		if pm.node.name != address && !ok {
			pm.dial.addDial(address)
		}
	}
}

// 리모트 peer로 부터 내 피어 정보가 요청된 것을 처리
func (pm *PeerManager) getCollect() string {
	fmt.Println("[PeerManager] getCollect")
	peersAddress := []string{}
	for _, peer := range pm.peers {
		peersAddress = append(peersAddress, peer.name)
	}

	return strings.Join(peersAddress, ",")
}

// ================================= Dialer & Task ================================//

var dialer_mutex = &sync.Mutex{}

type Dialer struct {
	needDial []string // ip/port
}

func (dialer *Dialer) addDial(address string) {
	fmt.Printf("addDial to [%s] \n", address)
	dialer_mutex.Lock()
	defer dialer_mutex.Unlock()
	dialer.needDial = append(dialer.needDial, address)
}

func (dialer *Dialer) newTask() []Task {
	dialer_mutex.Lock()
	defer dialer_mutex.Unlock()

	if len(dialer.needDial) > 0 {
		alltasks := make([]Task, 0, len(dialer.needDial))
		for _, port := range dialer.needDial {
			alltasks = append(alltasks, Task{address: port})
		}

		dialer.needDial = dialer.needDial[:0]
		return alltasks
	}

	return nil
}

type Task struct {
	address string
}

func (t *Task) Do(n *Node) {
	fmt.Printf("[Task] Connecting to %s...\n", t.address)
	new_conn, err := net.Dial("tcp", t.address)
	if err != nil {
		if _, t := err.(*net.OpError); t {
			fmt.Println("Some problem connecting.")
		} else {
			fmt.Println("Unknown error: " + err.Error())
		}
		os.Exit(1)
	}

	n.SetupConn(new_conn, false)

}



// ================================= grpc Service ================================//

type rpcService struct{
	n * Node
}

func (s *rpcService) GetBlock(ctx context.Context, in *pb.BlockRequest) (*pb.BlockReply, error) {
	fmt.Printf("RPC Received: %s \n", in.BlockId)
	info := s.n.blockchain.getBlockInfo(in.BlockId)
	return &pb.BlockReply{BlockBody: "Block is : " + info}, nil
}

// ================================= Node ================================//
type Node struct {
	name       string
	pm         *PeerManager
	blockchain *blockchain

	ListenAddr string // ":28000"
	listener   net.Listener
	bootstrap  string
	running    bool
	isLeader   bool

	ListenRPCAddr   string // ":29000"

	addConn  chan *conn
	delpeer  chan string
	aliveSig chan Msg
	quit     chan struct{}
	stop     chan struct{}

	lock sync.Mutex // protects running

	tick_getblock *time.Ticker
}

func (n *Node) Start() {
	fmt.Printf("Node-%s Start \n", n.name)
	n.addConn = make(chan *conn)
	n.delpeer = make(chan string)
	n.aliveSig = make(chan Msg)
	n.quit = make(chan struct{})
	n.stop = make(chan struct{})

	n.blockchain = &blockchain{blocks: make(map[string]string, 100)}

	// listening
	n.startListening()

	// dialer
	dialer := &Dialer{needDial: []string{}}
	go n.run(dialer)
	n.running = true

	// protocolManager
	n.pm = &PeerManager{node: n, dial: dialer, peers: make(map[string]*Peer, 100) }
	n.pm.Start()

	// if this peer is leader, get block from orderer
	if n.isLeader {
		go n.getBlock()

		// grpc for exposing block information
		go func(){

			lis, err := net.Listen("tcp", n.ListenRPCAddr)
			if err != nil {
				log.Fatalf("failed to listen: %v", err)
			}
			s := grpc.NewServer()
			pb.RegisterBlockServer(s, &rpcService{n: n})
			reflection.Register(s)
			if err := s.Serve(lis); err != nil {
				log.Fatalf("failed to serve: %v", err)
			}

		}()
	}


	<-n.stop
}

//구체적 종료 방식 구현 안함. Ctrl+C 같은 시그널에 의해서 호출될 수 있음.
func (n *Node) Stop() {
	n.pm.Stop()
	n.stop <- struct{}{}
}

// 1. Dialing을 한다.
// 2. Dialing 이나 Accept를 통해 연결된 리모트에 대한 새로운 peer개체를 만든다.
func (n *Node) run(dialer *Dialer) {
	var (
		runningTasks []Task
		queuedTasks  []Task
	)

	startTasks := func(ts []Task) (rest []Task) {
		i := 0
		for ; len(runningTasks) < maxActiveDialTasks && i < len(ts); i++ {
			t := ts[i]
			go func() { t.Do(n) }()
			runningTasks = append(runningTasks, t)
		}
		return ts[i:]
	}

	scheduleTasks := func() {
		queuedTasks = append(queuedTasks[:0], startTasks(queuedTasks)...)
		if len(runningTasks) < maxActiveDialTasks {
			nt := dialer.newTask()
			if nt == nil {
				return
			}
			queuedTasks = append(queuedTasks, startTasks(nt)...)
		}
	}

running:
	for {
		scheduleTasks()

		select {
		case <-n.quit:
			break running
		case c := <-n.addConn:
			p := newPeer(n, c)
			go n.runPeer(p)
			n.pm.addPeer <- p
		case name := <-n.delpeer:
			delete(n.pm.peers, name)
		case alive_msg := <-n.aliveSig:
			n.pm.aliveSig <- alive_msg
		default:
			time.Sleep(10 * time.Millisecond)
		}

	}
}

func (n *Node) runPeer(p *Peer) {
	p.run()
	n.delpeer <- p.name
}

func (n *Node) startListening() error {
	listener, err := net.Listen("tcp", n.ListenAddr)
	if err != nil {
		return err
	}
	laddr := listener.Addr().(*net.TCPAddr)
	n.ListenAddr = laddr.String()
	n.listener = listener
	go n.listenLoop()

	return nil
}

func (n *Node) listenLoop() {
	//tokens := defaultMaxPendingPeers
	//slots := make(chan struct{}, tokens)
	for {
		//<-slots
		var (
			fd  net.Conn
			err error
		)
		for {
			fd, err = n.listener.Accept()
			if err != nil {
				fmt.Printf("listenLoop-Read error %s \n", err)
				return
			}
			break
		}
		fmt.Printf("Accepted connection %s \n", fd.RemoteAddr())

		go func() {
			n.SetupConn(fd, true)
			//slots <- struct{}{}
		}()
	}
}

// inbound/outbound 커넥션 초기화 처리
// 프로토콜 및 암호에 대한 정보 교환 등 다양한 핸드쉐이킹을 할 수 있지만 현 코드에서는 이름만 교환.
func (n *Node) SetupConn(fd net.Conn, inbound bool) error {

	var yourName string

	if inbound {
		reader := bufio.NewReader(fd)
		yourName, _ = reader.ReadString('\n')
		fmt.Printf("#############################   [HandShake]  your name %s ################### \n", yourName)

		fd.SetWriteDeadline(time.Now().Add(1 * time.Second))
		fd.Write([]byte(n.name + "\n"))
		fmt.Printf("#############################    [HandShake] my name %s  ####################3\n", n.name)

	} else { // outbound
		fd.SetWriteDeadline(time.Now().Add(1 * time.Second))
		fd.Write([]byte(n.name + "\n"))
		fmt.Printf("##############################     [HandShake] my name %s #################### \n", n.name)

		reader := bufio.NewReader(fd)
		yourName, _ = reader.ReadString('\n')
		fmt.Printf("##############################     [HandShake] your name %s ################### \n", yourName)
	}

	yourName = strings.TrimSuffix(yourName, "\n")
	c := &conn{name: yourName, fd: fd, cont: make(chan error)}
	n.addConn <- c
	return nil
}

//리더피어는 5초에 한번씩 오더러에서 블록를 가져와서 주변에 전파한다.
func (n *Node) getBlock() {
	n.tick_getblock = time.NewTicker(time.Second * 5)
	go func() {
		var i = 0
		for _ = range n.tick_getblock.C {
			// 오더러에서 가져왔다고 가정하자.
			payload := fmt.Sprintf("Block-%d", i)
			fmt.Printf("Block-%d From Orderer ======================================== \n", i)

			n.blockchain.add(strconv.Itoa(i),payload)
			selectedPeer := selectRandom3(n.pm.peers)
			for _, peer := range selectedPeer {
				msg := Msg{Code: "block", Payload: payload}
				peer.writeMsg(msg)
			}
			i++
			fmt.Printf("============================================================== \n")
		}
	}()
}

func newNode(name string, ip string, port int, rpcPort int,  isLeader bool, bootstrap string) *Node {
	return &Node{name: name, ListenAddr: fmt.Sprintf("%s:%d", ip, port), ListenRPCAddr: fmt.Sprintf("%s:%d", ip, rpcPort), isLeader: isLeader, bootstrap: bootstrap}
}

// ================================= Main ================================//
// how to run:
// leader peer : go run peer.go -name 127.0.0.1:28000 -ip 127.0.0.1 -port 28000 -rpc 29000 -leader
// non-leader peer : go run peer.go -name 127.0.0.1:28001 -ip 127.0.0.1 -port 28001 -bootstrap 127.0.0.1:28000
func main() {
	NAME := flag.String("name", "127.0.0.1:28000", "peer name")
	IP := flag.String("ip", "127.0.0.1", "-ip string")
	PORT := flag.Int("port", 28000, "-port num")
	RPC_PORT := flag.Int("rpc", 29000, "-rpc num")
	BOOTSTRAP_ADDRESS := flag.String("bootstrap", "127.0.0.1:28000", "-bootstrap")
	LEADER := flag.Bool("leader", false, "lead peer")
	flag.Parse()

	node := newNode(*NAME, *IP, *PORT, *RPC_PORT, *LEADER, *BOOTSTRAP_ADDRESS)
	node.Start()
}

