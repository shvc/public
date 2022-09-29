package main

import (
	"context"
	"encoding/json"
	"fmt"
	mrand "math/rand"
	"net"
	"sync"
	"time"

	"github.com/libp2p/go-reuseport"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func init() {
	mrand.Seed(time.Now().Unix())
}

type data struct {
	ID     string `json:"id,omitempty"`
	Local  string `json:"local,omitempty"`
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

func (u *UDPPeer) prepare() (conn net.PacketConn, e error) {
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

	u.peerID = fmt.Sprintf("%s:%d", u.peerID, port)

	logger.Info("udp peer start",
		zap.String("id", u.peerID),
		zap.String("laddr", conn.LocalAddr().String()),
		zap.String("server1", u.serverAddress1),
		zap.String("server2", u.serverAddress2),
		zap.Uint32("dial-timeout", dialTimeout),
	)

	reqData := &data{
		ID: u.peerID,
	}
	buf := make([]byte, 2048)

	reqData.Op = "ping1"
	reqBuf, _ := json.Marshal(reqData)
	_, err = conn.WriteTo(reqBuf, u.serverAddr1)
	if err != nil {
		e = fmt.Errorf("ping1 WriteTo server %s err: %w", u.serverAddr1.String(), err)
		return
	}
	n, sraddr1, err := conn.ReadFrom(buf)
	if err != nil {
		e = fmt.Errorf("ping1 ReadFrom server %s err: %w", u.serverAddr1.String(), err)
		return
	}

	rcvData1 := &data{}
	if err := json.Unmarshal(buf[:n], rcvData1); err != nil {
		e = fmt.Errorf("ping1 %s Unmarshal %s err: %w", sraddr1.String(), buf[:n], err)
		return
	}
	if rcvData1.Op != "pong1" {
		e = fmt.Errorf("ping1 %s got invalid response %s", sraddr1.String(), rcvData1.Op)
		return
	}

	logger.Info("ping1 success",
		zap.String("raddr", sraddr1.String()),
		zap.String("public", rcvData1.Public),
		zap.Object("response", rcvData1),
	)

	reqData.Op = "ping2"
	reqBuf2, _ := json.Marshal(reqData)
	_, err = conn.WriteTo(reqBuf2, u.serverAddr2)
	if err != nil {
		e = fmt.Errorf("WriteTo server %s err: %w", u.serverAddr2.String(), err)
		return
	}

	n, sraddr2, err := conn.ReadFrom(buf)
	if err != nil {
		e = fmt.Errorf("ping2 ReadFrom server %s err: %w", u.serverAddr1.String(), err)
		return
	}

	rcvData2 := &data{}
	if err := json.Unmarshal(buf[:n], rcvData2); err != nil {
		e = fmt.Errorf("ping2 %s Unmarshal %s err: %w", sraddr2.String(), buf[:n], err)
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
		zap.Object("response", rcvData2),
	)

	return
}

func (u *UDPPeer) UDPPeerServer(ctx context.Context, port uint, dialTimeout, reportInterval, pingPeerInterval, pingPeerNum uint32) (e error) {
	conn, err := u.prepare()
	if err != nil {
		fmt.Println(err)
		return
	}
	reqData := &data{
		ID: u.peerID,
	}
	buf := make([]byte, 2048)

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
			rcvData := data{}
			if err := json.Unmarshal(buf[:n], &rcvData); err != nil {
				logger.Warn("Unmarshal response error",
					zap.String("raddr", raddr.String()),
					zap.Error(err),
				)
				continue
			}

			switch rcvData.Op {
			case "pong3": // response of ping3 from server
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
						ticker := time.NewTicker(time.Duration(pingPeerInterval+mrand.Uint32()%pingPeerInterval) * time.Millisecond)
						defer ticker.Stop()
						for i := uint32(1); i <= pingPeerNum; i++ {
							reqData.Op = "sping"
							reqData.Msg = "sping peer"
							reqData.Peer = peerAddr.String()
							reqBuf3, _ := json.Marshal(reqData)
							_, err := conn.WriteTo(reqBuf3, peerAddr)
							if err != nil {
								logger.Warn("sping error",
									zap.String("paddr", peerAddr.String()),
									zap.Error(err),
								)
							} else {
								logger.Debug("sping success",
									zap.String("paddr", peerAddr.String()),
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
					rspBuf, _ := json.Marshal(rspData)
					_, err := conn.WriteTo(rspBuf, clientAddr)
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
		reqBuf, _ := json.Marshal(reqData)
		_, err = conn.WriteTo(reqBuf, u.serverAddr1)
		if err != nil {
			e = fmt.Errorf("report to server %s err: %w", u.serverAddr1.String(), err)
			return
		}
		logger.Info("report success",
			zap.String("raddr", u.serverAddr1.String()),
			zap.Object("data", reqData),
		)
		time.Sleep(time.Duration(reportInterval) * time.Second)
	}

}

func (u *UDPPeer) PeerClientUDP(ctx context.Context, port uint, dialTimeout, pingServerInterval, pingPeerInterval, pingPeerCount, helloInterval uint32) (e error) {
	conn, err := u.prepare()
	if err != nil {
		fmt.Println(err)
		return
	}
	reqData := &data{
		ID: u.peerID,
	}
	buf := make([]byte, 2048)

	peerAddress := ""
	for i := 1; i <= 10; i++ {
		reqData.Op = "request" // request peer address
		reqBuf, _ := json.Marshal(reqData)
		_, err = conn.WriteTo(reqBuf, u.serverAddr1)
		if err != nil {
			return fmt.Errorf("send request to server %s err: %w", u.serverAddr1.String(), err)
		}

		n, raddr, err := conn.ReadFrom(buf)
		if err != nil {
			return fmt.Errorf("read response from server %s err: %w", u.serverAddr1.String(), err)
		}

		rcvData := &data{}
		if err := json.Unmarshal(buf[:n], rcvData); err != nil {
			return fmt.Errorf("decode response from server %s err: %w", raddr.String(), err)
		}

		logger.Info("request success",
			zap.String("server", u.serverAddr1.String()),
			zap.String("raddr", raddr.String()),
			zap.Int("seq", i),
			zap.Object("data", reqData),
		)

		if rcvData.Op == "pong3" && rcvData.Peer != "" {
			peerAddress = rcvData.Peer
			break
		}

		time.Sleep(time.Duration(pingServerInterval) * time.Second)
	}

	if peerAddress == "" {
		fmt.Println("no peer address received")
		return
	}

	punchedMessage := make(chan bool)
	go func() {
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
			case "sping": // peer server ping
				punchedMessage <- true
			case "cping": // peer client ping
				if rcvData.Msg == "byebye" {
					fmt.Println(rcvData.Op, rcvData.Msg)
					return
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

	peerAddr, err := net.ResolveUDPAddr(u.networkType, peerAddress)
	if err != nil {
		return fmt.Errorf("resolve peer %s err: %w", peerAddress, err)
	}
	ticker := time.NewTicker(time.Duration(pingPeerInterval+mrand.Uint32()%pingPeerInterval) * time.Millisecond)
	punched := false
	reqData.Op = "cping"
	reqData.Msg = "cping nat"
	reqData.Peer = peerAddr.String()
	for i := uint32(1); i <= pingPeerCount; i++ {
		if punched {
			if i == pingPeerCount {
				reqData.Msg = "byebye"
			} else {
				reqData.Msg = u.peerID + " say hello@" + time.Now().Format(time.RFC3339)
			}
		}
		reqBuf3, _ := json.Marshal(reqData)
		_, err := conn.WriteTo(reqBuf3, peerAddr)
		if err != nil {
			logger.Warn("send msg to peer error",
				zap.String("paddr", peerAddr.String()),
				zap.String("op", reqData.Op),
				zap.Error(err),
			)
		} else {
			logger.Debug("send msg to peer success",
				zap.String("op", reqData.Op),
				zap.String("msg", reqData.Msg),
				zap.String("paddr", peerAddr.String()),
			)
		}
		select {
		case <-punchedMessage:
			logger.Info("nat traversal success",
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
