package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	mrand "math/rand"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
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
	Public string `json:"public,omitempty"`
	Peer   string `json:"peer,omitempty"`
	Msg    string `json:"msg,omitempty"`
	Op     string `json:"op,omitempty"`
}

func (d *data) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	if d.ID != "" {
		enc.AddString("id", d.ID)
	}
	if d.Local != "" {
		enc.AddString("local", d.Local)
	}
	if d.Public != "" {
		enc.AddString("public", d.Public)
	}
	if d.Peer != "" {
		enc.AddString("peer", d.Peer)
	}
	if d.Msg != "" {
		enc.AddString("msg", d.Msg)
	}
	if d.Op != "" {
		enc.AddString("op", d.Op)
	}
	return nil
}

type QuicClient struct {
	secure             bool
	debug              bool
	nat                bool
	port               int
	dialTimeout        uint32
	pingServerInterval uint32
	pingPeerInterval   uint32
	pingPeerNum        uint32
	peerID             string
	keyLogFile         string
	remoteAddress      string
	serverAddress1     string
	serverAddress2     string
	serverAddr1        *net.UDPAddr
	serverAddr2        *net.UDPAddr
	networkType        string
	roundTripper       *http3.RoundTripper
}

func (u *QuicClient) readData(conn net.PacketConn) (dat data, raddr net.Addr, e error) {
	buf := make([]byte, 1024)
	n, raddr, err := conn.ReadFrom(buf)
	if err != nil {
		e = fmt.Errorf("read err: %w", err)
		return
	}

	if err := json.Unmarshal(buf[:n], &dat); err != nil {
		e = fmt.Errorf("unmarshal from %s err: %w", raddr.String(), err)
		return
	}

	return
}

func (u *QuicClient) writeData(conn net.PacketConn, raddr net.Addr, dat *data) error {
	reqBuf, err := json.Marshal(dat)
	if err != nil {
		return fmt.Errorf("marshal err: %w", err)
	}

	_, err = conn.WriteTo(reqBuf, raddr)
	if err != nil {
		return fmt.Errorf("write err: %w", err)
	}

	return nil
}

func (u *QuicClient) connPrepare() (net.PacketConn, error) {
	if !u.nat {
		u.remoteAddress = u.serverAddress1
		return net.ListenUDP(u.networkType, &net.UDPAddr{IP: net.IPv4zero, Port: u.port})
	}
	var err error
	u.serverAddr1, err = net.ResolveUDPAddr(u.networkType, u.serverAddress1)
	if err != nil {
		return nil, fmt.Errorf("resolve addr %s err: %w", u.serverAddress1, err)
	}

	u.serverAddr2, err = net.ResolveUDPAddr(u.networkType, u.serverAddress2)
	if err != nil {
		return nil, fmt.Errorf("resolve addr %s err: %w", u.serverAddress2, err)
	}

	conn, err := reuseport.ListenPacket(u.networkType, fmt.Sprintf(":%v", u.port))
	if err != nil {
		return nil, fmt.Errorf("listen addr %s err: %w", fmt.Sprintf(":%v", u.port), err)
	}

	if len(u.peerID) > 16 {
		u.peerID = u.peerID[:15]
	}
	ipPort := strings.Split(conn.LocalAddr().String(), ":")
	if length := len(ipPort); length > 1 {
		u.peerID = u.peerID + ":" + ipPort[length-1]
	}

	logger.Info("prepare conn",
		zap.String("id", u.peerID),
		zap.String("network", u.networkType),
		zap.String("laddr", conn.LocalAddr().String()),
		zap.String("server1", u.serverAddress1),
		zap.String("server2", u.serverAddress2),
		zap.Uint32("dial-timeout", u.dialTimeout),
	)

	reqData := data{
		ID: u.peerID,
		Op: "ping1",
	}

	err = u.writeData(conn, u.serverAddr1, &reqData)
	if err != nil {
		return nil, fmt.Errorf("ping1 %s err: %w", u.serverAddr1.String(), err)
	}

	rcvData1, sraddr1, err := u.readData(conn)
	if err != nil {
		return nil, fmt.Errorf("ping1 %s err: %w", u.serverAddr1.String(), err)
	}

	if rcvData1.Op != "pong1" {
		return nil, fmt.Errorf("ping1 %s got invalid response %s", sraddr1.String(), rcvData1.Op)
	}

	logger.Info("ping1 success",
		zap.String("raddr", sraddr1.String()),
		zap.String("public", rcvData1.Public),
		zap.Object("resp", &rcvData1),
	)

	reqData.Op = "ping2"
	err = u.writeData(conn, u.serverAddr2, &reqData)
	if err != nil {
		return nil, fmt.Errorf("ping2 %s err: %w", u.serverAddr2.String(), err)
	}

	rcvData2, sraddr2, err := u.readData(conn)
	if err != nil {
		return nil, fmt.Errorf("ping2 %s err: %w", u.serverAddr2.String(), err)
	}

	if rcvData2.Op != "pong2" {
		return nil, fmt.Errorf("ping2 %s got invalid response %s", sraddr2.String(), rcvData2.Op)
	}

	if rcvData1.Public != rcvData2.Public {
		return nil, fmt.Errorf("not a cone nat %s != %s", rcvData1.Public, rcvData2.Public)
	}

	logger.Info("ping2 success",
		zap.String("raddr", sraddr2.String()),
		zap.String("public", rcvData2.Public),
		zap.Object("resp", &rcvData2),
	)

	peerAddressMessage := make(chan string)
	punchedMessage := make(chan bool)
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		punched := false
		gotpong3 := false
		for {
			rcvData, raddr, err := u.readData(conn)
			if err != nil {
				logger.Warn("read error",
					zap.Error(err),
				)
				continue
			}

			switch rcvData.Op {
			case "sping": // peer server ping
				if !punched {
					punched = true
					punchedMessage <- true
				}
				continue
			case "pong3":
				if !gotpong3 && rcvData.Peer != "" {
					gotpong3 = true
					peerAddressMessage <- rcvData.Peer
				}
				continue
			case "cping": // peer client ping(peer server's reply)
				if rcvData.Msg == "byebye" {
					fmt.Println(rcvData.Op, rcvData.Msg)
					return
				}
			default:
				logger.Warn("recv unknown msg",
					zap.String("raddr", raddr.String()),
					zap.Object("data", &rcvData),
				)
				continue
			}

			logger.Info("recv msg",
				zap.String("raddr", raddr.String()),
				zap.Object("data", &rcvData),
			)
		}

	}()

	peerAddress := ""
	ticker := time.NewTicker(time.Duration(u.pingServerInterval) * time.Second)
requestLoop:
	for i := 0; i < 10; i++ {
		reqData.Op = "request" // request peer address
		err = u.writeData(conn, u.serverAddr1, &reqData)
		if err != nil {
			conn.Close()
			return nil, fmt.Errorf("write to server %s err: %w", u.serverAddr1.String(), err)
		}

		logger.Info("request",
			zap.String("server", u.serverAddr1.String()),
			zap.Object("req", &reqData),
		)

		select {
		case peerAddress = <-peerAddressMessage:
			logger.Info("got peer address",
				zap.String("raddr", u.serverAddr1.String()),
				zap.String("peer", peerAddress),
			)
			ticker.Stop()
			break requestLoop
		case <-ticker.C:
			continue
		}
	}

	if peerAddress == "" {
		conn.Close()
		return nil, fmt.Errorf("no peer address received")
	}
	peerAddr, err := net.ResolveUDPAddr(u.networkType, peerAddress)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("resolve peer %s err: %w", peerAddress, err)
	}

	ticker = time.NewTicker(time.Duration(u.pingPeerInterval+mrand.Uint32()%u.pingPeerInterval) * time.Millisecond)
	punched := false
	reqData.Op = "cping"
	reqData.Msg = "cping nat"
	reqData.Peer = peerAddr.String()
pingPeerLoop:
	for i := uint32(1); i <= u.pingPeerNum; i++ {
		err = u.writeData(conn, peerAddr, &reqData)
		if err != nil {
			logger.Warn("write to peer error",
				zap.String("paddr", peerAddr.String()),
				zap.String("op", reqData.Op),
				zap.Error(err),
			)
		} else {
			logger.Debug("write to peer success",
				zap.String("paddr", peerAddr.String()),
				zap.String("op", reqData.Op),
			)
		}
		select {
		case <-punchedMessage:
			logger.Info("PUNCH success",
				zap.String("raddr", u.serverAddr1.String()),
				zap.String("peer", peerAddress),
			)
			ticker.Stop()
			punched = true
			break pingPeerLoop
		case <-ticker.C:
			continue
		}
	}
	if !punched {
		conn.Close()
		return nil, fmt.Errorf("PUNCH failed")
	}
	u.remoteAddress = peerAddress
	return conn, nil
}

func (q *QuicClient) get(ctx context.Context, urlPath string, filename string) error {
	peerConn, err := q.connPrepare()
	if err != nil {
		return fmt.Errorf("prepare network error %w", err)
	}
	q.roundTripper.Dial = func(ctx context.Context, serverAddr string, tlsConf *tls.Config, config *quic.Config) (quic.EarlyConnection, error) {
		udpRemoteAddr, err := net.ResolveUDPAddr("udp", q.remoteAddress)
		if err != nil {
			return nil, err
		}

		qc, err := quic.DialContext(ctx, peerConn, udpRemoteAddr, q.remoteAddress, tlsConf, config)
		if err != nil {
			return nil, err
		}

		ec, ok := qc.(quic.EarlyConnection)
		if !ok {
			return nil, fmt.Errorf("dail error")
		}

		logger.Info("conn ready",
			zap.String("server", serverAddr),
			zap.Bool("nat", q.nat),
			zap.String("remote", q.remoteAddress),
			zap.String("local", ec.LocalAddr().String()),
		)

		return ec, nil
	}
	defer q.roundTripper.Close()

	if !strings.HasPrefix(urlPath, "/") {
		urlPath = "/" + urlPath
	}
	addr := "https://" + q.remoteAddress + urlPath

	if len(q.keyLogFile) > 0 {
		f, err := os.Create(q.keyLogFile)
		if err != nil {
			logger.Warn("create key log file error",
				zap.String("filename", q.keyLogFile),
				zap.Error(err),
			)
		} else {
			defer f.Close()
			q.roundTripper.TLSClientConfig.KeyLogWriter = f
		}
	}
	logger.Debug("get",
		zap.String("addr", addr),
		zap.String("path", urlPath),
		zap.String("key file", q.keyLogFile),
	)

	hclient := http.Client{
		Transport: q.roundTripper,
	}
	rsp, err := hclient.Get(addr)
	if err != nil {
		return fmt.Errorf("get error %w", err)
	}

	logger.Debug("get response",
		zap.String("addr", addr),
		zap.String("host", rsp.Request.Host),
		zap.String("url", rsp.Request.URL.String()),
		zap.Int("status", rsp.StatusCode),
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

func (q *QuicClient) put(ctx context.Context, addr string) error {

	return nil
}

func (q *QuicClient) post(ctx context.Context, addr string) error {

	return nil
}

func (q *QuicClient) delete(ctx context.Context, addr string) error {

	return nil
}
