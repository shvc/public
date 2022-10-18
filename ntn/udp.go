package main

import (
	"context"
	"encoding/json"
	"fmt"
	mrand "math/rand"
	"net"
	"strings"
	"time"

	"github.com/libp2p/go-reuseport"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func init() {
	mrand.Seed(time.Now().Unix())
}

type data struct {
	ID      string `json:"id,omitempty"`
	Local   string `json:"local,omitempty"`
	Public  string `json:"public,omitempty"`
	Peer    string `json:"peer,omitempty"`
	Msg     string `json:"msg,omitempty"`
	Op      string `json:"op,omitempty"`
	PingNum uint32 `json:"pingnum,omitempty"`
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

func NewUdpPeer(id, server1, server2 string) *UDPPeer {
	p := &UDPPeer{
		networkType:    "udp4",
		peerID:         id,
		serverAddress1: server1,
		serverAddress2: server2,
	}

	return p
}

type UDPPeer struct {
	peerID         string
	networkType    string
	serverAddress1 string
	serverAddress2 string
	serverAddr1    *net.UDPAddr
	serverAddr2    *net.UDPAddr
}

func (u *UDPPeer) readData(conn net.PacketConn) (dat data, raddr net.Addr, e error) {
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

func (u *UDPPeer) writeData(conn net.PacketConn, raddr net.Addr, dat *data) error {
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

func (u *UDPPeer) prepare(port uint) (conn net.PacketConn, e error) {
	var err error
	u.serverAddr1, err = net.ResolveUDPAddr(u.networkType, u.serverAddress1)
	if err != nil {
		return nil, fmt.Errorf("resolve addr %s err: %w", u.serverAddress1, err)
	}

	u.serverAddr2, err = net.ResolveUDPAddr(u.networkType, u.serverAddress2)
	if err != nil {
		return nil, fmt.Errorf("resolve addr %s err: %w", u.serverAddress2, err)
	}

	conn, err = reuseport.ListenPacket("udp4", fmt.Sprintf(":%v", port))
	if err != nil {
		return nil, fmt.Errorf("listen addr %s err: %w", fmt.Sprintf(":%v", port), err)
	}

	if len(u.peerID) > 16 {
		u.peerID = u.peerID[:15]
	}
	ipPort := strings.Split(conn.LocalAddr().String(), ":")
	if length := len(ipPort); length > 1 {
		u.peerID = u.peerID + ":" + ipPort[length-1]
	}

	logger.Info("udp peer start",
		zap.String("id", u.peerID),
		zap.String("laddr", conn.LocalAddr().String()),
		zap.String("server1", u.serverAddress1),
		zap.String("server2", u.serverAddress2),
		zap.Uint32("dial-timeout", dialTimeout),
	)

	reqData := data{
		ID: u.peerID,
		Op: "ping1",
	}

	err = u.writeData(conn, u.serverAddr1, &reqData)
	if err != nil {
		e = fmt.Errorf("ping1 %s err: %w", u.serverAddr1.String(), err)
		return
	}

	rcvData1, sraddr1, err := u.readData(conn)
	if err != nil {
		e = fmt.Errorf("ping1 %s err: %w", u.serverAddr1.String(), err)
		return
	}

	if rcvData1.Op != "pong1" {
		e = fmt.Errorf("ping1 %s got invalid response %s", sraddr1.String(), rcvData1.Op)
		return
	}

	logger.Info("ping1 success",
		zap.String("raddr", sraddr1.String()),
		zap.String("public", rcvData1.Public),
		zap.Object("resp", &rcvData1),
	)

	reqData.Op = "ping2"
	err = u.writeData(conn, u.serverAddr2, &reqData)
	if err != nil {
		e = fmt.Errorf("ping2 %s err: %w", u.serverAddr2.String(), err)
		return
	}

	rcvData2, sraddr2, err := u.readData(conn)
	if err != nil {
		e = fmt.Errorf("ping2 %s err: %w", u.serverAddr2.String(), err)
		return
	}

	if rcvData2.Op != "pong2" {
		e = fmt.Errorf("ping2 %s got invalid response %s", sraddr2.String(), rcvData2.Op)
		return
	}

	if rcvData1.Public != rcvData2.Public {
		e = fmt.Errorf("not a cone nat %s != %s", rcvData1.Public, rcvData2.Public)
		return
	}

	logger.Info("ping2 success",
		zap.String("raddr", sraddr2.String()),
		zap.String("public", rcvData2.Public),
		zap.Object("resp", &rcvData2),
	)

	return
}

func (u *UDPPeer) UDPPeerServer(ctx context.Context, port uint, dialTimeout, reportInterval, pingPeerInterval, pingPeerDelay uint32) (e error) {
	conn, err := u.prepare(port)
	if err != nil {
		fmt.Println(err)
		return
	}
	reqData := &data{
		ID: u.peerID,
	}

	go func() {
		for {
			rcvData, raddr, err := u.readData(conn)
			if err != nil {
				logger.Warn("read error",
					zap.Error(err),
				)
				continue
			}

			switch rcvData.Op {
			case "pong3": // response of report from server
				if rcvData.Peer != "" {
					go func(peerAddress string) {
						peerAddr, err := net.ResolveUDPAddr(u.networkType, peerAddress)
						if err != nil {
							logger.Warn("resolve peer address faled",
								zap.String("address", peerAddress),
								zap.Error(err),
							)
							return
						}
						reqData := &data{
							ID: u.peerID,
						}
						if pingPeerDelay > 0 {
							time.Sleep(time.Duration(pingPeerDelay) * time.Millisecond)
						}
						ticker := time.NewTicker(time.Duration(mrand.Uint32()%pingPeerInterval) * time.Millisecond)
						defer ticker.Stop()
						if rcvData.PingNum < 1 {
							rcvData.PingNum = 10
						}
						for i := uint32(0); i < rcvData.PingNum; i++ {
							reqData.Op = "sping"
							reqData.Msg = "sping peer"
							reqData.Peer = peerAddr.String()
							reqData.PingNum = i
							err = u.writeData(conn, peerAddr, reqData)
							if err != nil {
								logger.Warn("sping error",
									zap.String("paddr", peerAddr.String()),
									zap.Error(err),
								)
							} else {
								logger.Debug("sping success",
									zap.String("paddr", peerAddr.String()),
									zap.Uint32("num", i),
								)
							}
							select {
							case <-ticker.C:
								continue
							}
						}
					}(rcvData.Peer)
				}
			case "cping": // peer client ping
				go func(clientAddr net.Addr, rcvd data) {
					rspData := &data{
						ID:  rcvd.ID,
						Op:  rcvd.Op,
						Msg: rcvd.Msg,
					}
					err = u.writeData(conn, clientAddr, rspData)
					if err != nil {
						logger.Warn("response cping error",
							zap.String("paddr", clientAddr.String()),
							zap.Error(err),
						)
						return
					}
					logger.Debug("response cping success",
						zap.String("paddr", clientAddr.String()),
						zap.String("op", rspData.Op),
						zap.String("msg", rspData.Msg),
					)
				}(raddr, rcvData)
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

	for {
		reqData.Op = "report"
		err = u.writeData(conn, u.serverAddr1, reqData)
		if err != nil {
			e = fmt.Errorf("report to server %s err: %w", u.serverAddr1.String(), err)
			return
		}

		logger.Info("report",
			zap.String("raddr", u.serverAddr1.String()),
			zap.Object("req", reqData),
		)

		time.Sleep(time.Duration(reportInterval) * time.Second)
	}

}

func (u *UDPPeer) UDPPeerClient(ctx context.Context, port uint, dialTimeout, requestInterval, pingPeerInterval, pingPeerNum, pingPeerDelay, helloInterval uint32) (e error) {
	conn, err := u.prepare(port)
	if err != nil {
		fmt.Println(err)
		return
	}

	punchedMessage := make(chan bool)
	peerAddressMessage := make(chan string)
	go func() {
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
					peerAddressMessage <- rcvData.Peer
					gotpong3 = true
				}
				continue
			case "cping": // peer client ping(peer server's reply)
				if rcvData.Msg == "byebye" {
					fmt.Println(rcvData.Op, rcvData.Msg)
					return
				}
			default:
				logger.Warn("unknown op",
					zap.String("raddr", raddr.String()),
					zap.Object("data", &rcvData),
				)
				continue
			}

			logger.Info("recv",
				zap.String("raddr", raddr.String()),
				zap.Object("data", &rcvData),
			)
		}
	}()

	reqData := &data{
		ID:      u.peerID,
		PingNum: pingPeerNum,
	}

	peerAddress := ""
	ticker := time.NewTicker(time.Duration(requestInterval) * time.Second)
requestLoop:
	for i := 0; i < 10; i++ {
		reqData.Op = "request" // request peer address
		err = u.writeData(conn, u.serverAddr1, reqData)
		if err != nil {
			return fmt.Errorf("write to server %s err: %w", u.serverAddr1.String(), err)
		}

		logger.Info("request",
			zap.String("server", u.serverAddr1.String()),
			zap.Object("req", reqData),
		)

		select {
		case peerAddress = <-peerAddressMessage:
			ticker.Stop()
			break requestLoop
		case <-ticker.C:
			continue
		}
	}

	if peerAddress == "" {
		fmt.Println("no peer address received")
		return
	}
	logger.Info("got peer address",
		zap.String("raddr", u.serverAddr1.String()),
		zap.String("peer", peerAddress),
	)

	peerAddr, err := net.ResolveUDPAddr(u.networkType, peerAddress)
	if err != nil {
		return fmt.Errorf("resolve peer %s err: %w", peerAddress, err)
	}

	punched := false
	reqData.Op = "cping"
	reqData.Msg = "cping nat"
	reqData.Peer = peerAddr.String()

	if pingPeerDelay > 0 {
		logger.Info("ping peer delay millisecond",
			zap.Uint32("delay", pingPeerDelay),
		)
		time.Sleep(time.Duration(pingPeerDelay) * time.Millisecond)
	}
	ticker = time.NewTicker(time.Duration(mrand.Uint32()%pingPeerInterval) * time.Millisecond)
	//defer ticker.Stop()
	for i := uint32(1); i <= pingPeerNum; i++ {
		if punched {
			if i == pingPeerNum {
				reqData.Msg = "byebye"
			} else {
				reqData.Msg = u.peerID + ":HELLO@" + time.Now().Format("01-02T15:04:05Z")
			}
		}

		err = u.writeData(conn, peerAddr, reqData)
		if err != nil {
			logger.Warn("write to peer error",
				zap.String("paddr", peerAddr.String()),
				zap.String("op", reqData.Op),
				zap.Error(err),
			)
		} else {
			logger.Debug("write to peer success",
				zap.Uint32("num", i),
				zap.String("op", reqData.Op),
				zap.String("msg", reqData.Msg),
				zap.String("paddr", peerAddr.String()),
			)
		}
		select {
		case <-punchedMessage:
			logger.Info("PUNCH success",
				zap.String("raddr", u.serverAddr1.String()),
				zap.String("peer", peerAddress),
			)
			ticker.Reset(time.Duration(helloInterval) * time.Second)
			punched = true
		case <-ticker.C:
			continue
		}
	}

	return
}
