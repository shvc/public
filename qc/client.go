package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	mrand "math/rand"
	"net"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/libp2p/go-reuseport"
	"github.com/lucas-clemente/quic-go"
	"github.com/lucas-clemente/quic-go/http3"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type data struct {
	ID     string `json:"id,omitempty"`
	Local  string `json:"local,omitempty"`
	Remote string `json:"remote,omitempty"`
	Public string `json:"public,omitempty"`
	Peer   string `json:"peer,omitempty"`
	Msg    string `json:"msg,omitempty"`
	Op     string `json:"op,omitempty"`
}

func (f *data) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	if f.ID != "" {
		enc.AddString("id", f.ID)
	}
	if f.Local != "" {
		enc.AddString("local", f.Local)
	}
	if f.Public != "" {
		enc.AddString("public", f.Public)
	}
	if f.Peer != "" {
		enc.AddString("peer", f.Peer)
	}
	if f.Msg != "" {
		enc.AddString("msg", f.Msg)
	}
	if f.Op != "" {
		enc.AddString("op", f.Op)
	}
	return nil
}

type QuicClient struct {
	secure             bool
	debug              bool
	nat                bool
	punched            atomic.Bool
	gotPeer            atomic.Bool
	port               int
	dialTimeout        uint
	pingServerInterval uint32
	pingPeerInterval   uint32
	clientID           string
	keyLogFile         string
	serverAddress1     string
	serverAddress2     string
	networkType        string
	roundTripper       *http3.RoundTripper
}

func (u *QuicClient) connPrepare() (net.PacketConn, error) {
	if !u.nat {
		return net.ListenUDP(u.networkType, &net.UDPAddr{IP: net.IPv4zero, Port: u.port})
	}
	serverAddr1, err := net.ResolveUDPAddr(u.networkType, u.serverAddress1)
	if err != nil {
		return nil, fmt.Errorf("resolve addr %s err: %w", u.serverAddress1, err)
	}

	serverAddr2, err := net.ResolveUDPAddr(u.networkType, u.serverAddress2)
	if err != nil {
		return nil, fmt.Errorf("resolve addr %s err: %w", u.serverAddress2, err)
	}

	conn, err := reuseport.ListenPacket(u.networkType, fmt.Sprintf(":%v", u.port))
	if err != nil {
		return nil, fmt.Errorf("listen addr %s err: %w", fmt.Sprintf(":%v", u.port), err)
	}

	u.clientID = fmt.Sprintf("%s:%v", u.clientID, u.port)

	logger.Info("udp client start",
		zap.String("id", u.clientID),
		zap.String("laddr", conn.LocalAddr().String()),
		zap.String("server1", u.serverAddress1),
		zap.String("server2", u.serverAddress2),
		zap.Uint("dial-timeout", u.dialTimeout),
	)

	reqData := &data{
		ID: u.clientID,
	}
	buf := make([]byte, 2048)

	reqData.Op = "ping1"
	reqBuf, _ := json.Marshal(reqData)
	_, err = conn.WriteTo(reqBuf, serverAddr1)
	if err != nil {
		return nil, fmt.Errorf("ping1 WriteTo server %s err: %w", serverAddr1.String(), err)

	}
	n, sraddr1, err := conn.ReadFrom(buf)
	if err != nil {
		return nil, fmt.Errorf("ping1 ReadFrom server %s err: %w", serverAddr1.String(), err)
	}

	rcvData1 := &data{}
	if err := json.Unmarshal(buf[:n], rcvData1); err != nil {
		return nil, fmt.Errorf("ping1 %s Unmarshal %s err: %w", sraddr1.String(), buf[:n], err)
	}
	if rcvData1.Op != "pong1" {
		return nil, fmt.Errorf("ping1 %s got invalid response %s", sraddr1.String(), rcvData1.Op)
	}

	logger.Info("ping1 success",
		zap.String("raddr", sraddr1.String()),
		zap.String("public", rcvData1.Public),
		zap.Object("response", rcvData1),
	)

	reqData.Op = "ping2"
	reqBuf2, _ := json.Marshal(reqData)
	_, err = conn.WriteTo(reqBuf2, serverAddr2)
	if err != nil {
		return nil, fmt.Errorf("WriteTo server %s err: %w", serverAddr2.String(), err)
	}

	n, sraddr2, err := conn.ReadFrom(buf)
	if err != nil {
		return nil, fmt.Errorf("ping2 ReadFrom server %s err: %w", serverAddr1.String(), err)
	}

	rcvData2 := &data{}
	if err := json.Unmarshal(buf[:n], rcvData2); err != nil {
		return nil, fmt.Errorf("ping2 %s Unmarshal %s err: %w", sraddr2.String(), buf[:n], err)
	}
	if rcvData2.Op != "pong2" {
		return nil, fmt.Errorf("ping2 %s got invalid response %s", sraddr2.String(), rcvData2.Op)
	}

	if rcvData1.Public != rcvData2.Public {
		logger.Warn("not a cone nat",
			zap.String("public addr1", rcvData1.Public),
			zap.String("public addr2", rcvData2.Public),
			zap.Object("data", rcvData2),
		)
		return nil, fmt.Errorf("not a cone nat: %s != %s", rcvData1.Public, rcvData2.Public)
	}

	logger.Info("ping2 success",
		zap.String("raddr", sraddr2.String()),
		zap.String("public", rcvData2.Public),
		zap.Object("response", rcvData2),
	)

	recvPeerMessage := make(chan string)
	punchedMessage := make(chan bool)
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			n, raddr, err := conn.ReadFrom(buf)
			if err != nil {
				logger.Warn("ReadFrom error",
					zap.String("raddr", raddr.String()),
					zap.Error(err),
				)
				break
			}
			rcvData := &data{}
			if err := json.Unmarshal(buf[:n], rcvData); err != nil {
				logger.Warn("Unmarshal response error",
					zap.String("raddr", raddr.String()),
					zap.Error(err),
				)
				continue
			}

			switch rcvData.Op {
			case "pong3": // response of ping3 from server
				if rcvData.Peer != "" && !u.gotPeer.Load() {
					u.gotPeer.Store(true)
					recvPeerMessage <- rcvData.Peer
				}
			case "pping": // peer ping from peer
				if !u.punched.Load() {
					u.punched.Store(true)
					punchedMessage <- true
				}
			default:
				logger.Warn("recv unknown msg",
					zap.String("raddr", raddr.String()),
					zap.Object("data", rcvData),
				)
				continue
			}

			logger.Info("recv msg",
				zap.String("raddr", raddr.String()),
				zap.Object("data", rcvData),
			)
		}

	}()

	ticker := time.NewTicker(time.Duration(u.pingServerInterval) * time.Second)
	peerAddress := ""
ping3Loop:
	for {
		reqData.Op = "ping3"
		reqBuf, _ := json.Marshal(reqData)
		_, err = conn.WriteTo(reqBuf, serverAddr1)
		if err != nil {
			return nil, fmt.Errorf("ping3 WriteTo server %s err: %w", serverAddr1.String(), err)

		}
		logger.Info("ping3 success",
			zap.String("raddr", serverAddr1.String()),
			zap.Object("data", reqData),
		)
		select {
		case peerAddress = <-recvPeerMessage:
			logger.Info("got peer address",
				zap.String("raddr", serverAddr1.String()),
				zap.String("peer", peerAddress),
			)
			ticker.Stop()
			break ping3Loop
		case <-ticker.C:
			continue
		}

	}

	peerAddr, err := net.ResolveUDPAddr(u.networkType, peerAddress)
	if err != nil {
		return nil, fmt.Errorf("resolve peer %s err: %w", peerAddress, err)
	}
	pingPeerCount := 0
	ticker = time.NewTicker(time.Duration(u.pingPeerInterval+mrand.Uint32()%u.pingPeerInterval) * time.Millisecond)
pingPeerLoop:
	for {
		reqData.Op = "pping"
		reqData.Msg = "ping peer"
		reqData.Peer = peerAddr.String()
		reqBuf3, _ := json.Marshal(reqData)
		_, err := conn.WriteTo(reqBuf3, peerAddr)
		if err != nil {
			logger.Warn("ping peer error",
				zap.String("paddr", peerAddr.String()),
				zap.Error(err),
			)
		} else {
			pingPeerCount++
			logger.Debug("ping peer success",
				zap.String("paddr", peerAddr.String()),
			)
		}
		if pingPeerCount > 10 {
			return nil, fmt.Errorf("nat traversal failed")
		}
		select {
		case <-punchedMessage:
			logger.Info("traversal success",
				zap.String("raddr", serverAddr1.String()),
				zap.String("peer", peerAddress),
			)
			ticker.Stop()
			break pingPeerLoop
		case <-ticker.C:
			continue
		}

	}

	return conn, nil
}

func (q *QuicClient) get(ctx context.Context, raddr string, dialTimeout uint, filename string) error {
	q.roundTripper.Dial = func(ctx context.Context, addr string, tlsConf *tls.Config, config *quic.Config) (quic.EarlyConnection, error) {
		udpAddr, err := net.ResolveUDPAddr("udp", addr)
		if err != nil {
			return nil, err
		}

		udpConn, err := q.connPrepare()
		if err != nil {
			return nil, err
		}

		qc, err := quic.DialContext(ctx, udpConn, udpAddr, addr, tlsConf, config)
		if err != nil {
			return nil, err
		}

		ec, ok := qc.(quic.EarlyConnection)
		if !ok {
			return nil, fmt.Errorf("dail error")
		}

		logger.Info("conn ready",
			zap.String("remote", addr),
			zap.String("local", ec.LocalAddr().String()),
		)

		return ec, nil
	}

	defer q.roundTripper.Close()

	if q.debug {
		log.Printf("GET %s", raddr)

	}

	logger.Debug("get",
		zap.String("addr", raddr),
	)

	if len(q.keyLogFile) > 0 {
		f, err := os.Create(q.keyLogFile)
		if err == nil {
			defer f.Close()
			q.roundTripper.TLSClientConfig.KeyLogWriter = f
		} else {
			log.Println("key log error ", err)
		}

	}

	hclient := http.Client{
		Transport: q.roundTripper,
	}
	rsp, err := hclient.Get(raddr)
	if err != nil {
		return fmt.Errorf("get error %w", err)
	}

	if q.debug {
		log.Printf("%s response: %#v", raddr, rsp)
	}

	logger.Debug("get response",
		zap.String("addr", raddr),
		zap.String("status", rsp.Status),
		zap.Int64("content-length", rsp.ContentLength),
	)

	w := os.Stdout
	if filename != "" {
		f, err := os.Create(filename)
		if err != nil {
			return err
		}
		defer f.Close()
		w = f
	}

	io.Copy(w, rsp.Body)

	return nil
}

func (q *QuicClient) put(ctx context.Context, addr string, dialTimeout uint) error {

	return nil
}

func (q *QuicClient) post(ctx context.Context, addr string, dialTimeout uint) error {

	return nil
}

func (q *QuicClient) delete(ctx context.Context, addr string, dialTimeout uint) error {

	return nil
}
