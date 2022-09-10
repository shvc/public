package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/libp2p/go-reuseport"
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

type store struct {
	expire int64
	data
}

func (f *data) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	if f.ID != "" {
		enc.AddString("id", f.ID)
	}
	if f.Local != "" {
		enc.AddString("local", f.Local)
	}
	if f.Remote != "" {
		enc.AddString("remote", f.Remote)
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

type UDPServer struct {
	sync.RWMutex
	v map[string]store
}

func (s *UDPServer) set(v data) {
	s.Lock()
	defer s.Unlock()
	s.v[v.ID] = store{expire: time.Now().Unix() + 15, data: v}
}

func (s *UDPServer) delete(k string) {
	s.Lock()
	defer s.Unlock()
	delete(s.v, k)
}

func (s *UDPServer) get(k string) (d data, ok bool) {
	s.RLock()
	defer s.RUnlock()
	if v, got := s.v[k]; got {
		d = v.data
		ok = got
	}
	return
}

func (s *UDPServer) initStore(ctx context.Context) {
	if s.v == nil {
		s.v = map[string]store{}
	}

	go func() {
		tick := time.Tick(2 * time.Second)
		for {
			now := time.Now().Unix()
			select {
			case <-tick:
				for k, v := range s.v {
					if v.expire < now {
						logger.Debug("record expired",
							zap.String("key", k),
						)
						delete(s.v, k)
					}
				}
			case <-ctx.Done():
				return
			}

		}
	}()
}

func (s *UDPServer) selectOnePeer(exclude string) (peerID, peerAddr string) {
	s.RLock()
	defer s.RUnlock()

	for k, v := range s.v {
		if k == exclude {
			continue
		}
		peerID = k
		peerAddr = v.Public
		return
	}

	return
}

func (s *UDPServer) startUDPServer(ctx context.Context, lc *net.ListenConfig, addr string) error {
	networkType := "udp4"
	conn, err := lc.ListenPacket(ctx, networkType, addr)
	if err != nil {
		return fmt.Errorf("listen addr %s fail, err: %w", addr, err)
	}
	defer conn.Close()

	logger.Info("udp server started",
		zap.String("addr", conn.LocalAddr().String()),
	)

	buf := make([]byte, 2048)
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
			n, raddr, err := conn.ReadFrom(buf)
			if err != nil {
				logger.Warn("ReadFrom error",
					zap.String("laddr", conn.LocalAddr().String()),
					zap.Error(err),
				)
				continue
			}

			rcvData := data{}
			if err := json.Unmarshal(buf[:n], &rcvData); err != nil {
				logger.Warn("readFrom success but decode error",
					zap.Int("len", n),
					zap.String("laddr", conn.LocalAddr().String()),
					zap.String("raddr", raddr.String()),
					zap.ByteString("content", buf[:n]),
					zap.Error(err),
				)
				continue
			}

			if rcvData.ID == "" {
				logger.Warn("readFrom success but no ID",
					zap.Int("len", n),
					zap.String("laddr", conn.LocalAddr().String()),
					zap.String("raddr", raddr.String()),
					zap.ByteString("content", buf[:n]),
				)
				continue
			}
			// set public addr
			rcvData.Public = raddr.String()

			rspData := &data{
				ID: rcvData.ID,
				//Local:  conn.LocalAddr().String(),
				Public: rcvData.Public,
			}

			var relayPeerAddr *net.UDPAddr
			switch rcvData.Op {
			case "ping1":
				s.set(rcvData)
				rspData.Op = "pong1"
			case "ping2":
				prevData, ok := s.get(rcvData.ID)
				if ok {
					if prevData.Public == rcvData.Public {
						rspData.Msg, rspData.Peer = s.selectOnePeer(rcvData.ID)
						logger.Debug("select peer",
							zap.String("peer addr", rspData.Peer),
							zap.String("peer id", rspData.Msg),
						)
					} else {
						s.delete(rcvData.ID)
						rspData.Msg = fmt.Sprintf("ping1 public:%s != ping2 public:%s", prevData.Public, rcvData.Public)
						logger.Debug("delete record",
							zap.String("record id", rcvData.ID),
							zap.String("public addr1", prevData.Public),
							zap.String("public addr2", rcvData.Public),
						)
					}
				} else {
					logger.Warn("not found ping1 record",
						zap.String("id", rcvData.ID),
						zap.String("laddr", conn.LocalAddr().String()),
						zap.String("raddr", raddr.String()),
						zap.String("op", rcvData.Op),
					)
				}
				rspData.Op = "pong2"
			case "relay":
				if rcvData.Peer == "" {
					logger.Warn("unknown peer to relay",
						zap.Int("len", n),
						zap.String("laddr", conn.LocalAddr().String()),
						zap.String("raddr", raddr.String()),
						zap.String("op", rcvData.Op),
						zap.ByteString("content", buf[:n]),
					)
					continue
				}
				relayPeerAddr, err = net.ResolveUDPAddr(networkType, rcvData.Peer)
				if err != nil {
					logger.Warn("invalid peer addr to relay",
						zap.Int("len", n),
						zap.String("laddr", conn.LocalAddr().String()),
						zap.String("raddr", raddr.String()),
						zap.String("op", rcvData.Op),
						zap.ByteString("content", buf[:n]),
						zap.Error(err),
					)
					continue
				}
				rspData.Op = rcvData.Op
				rspData.Peer = raddr.String() // reset peer address
				rspData.Msg = "server relay msg"
			default:
				logger.Warn("unknown op",
					zap.Int("len", n),
					zap.String("laddr", conn.LocalAddr().String()),
					zap.String("raddr", raddr.String()),
					zap.String("op", rcvData.Op),
					zap.ByteString("content", buf[:n]),
				)
				continue
			}

			if relayPeerAddr != nil {
				logger.Info("recv relay success",
					zap.Int("len", n),
					zap.String("laddr", conn.LocalAddr().String()),
					zap.String("raddr", raddr.String()),
					zap.String("peer", relayPeerAddr.String()),
					zap.Object("data", &rcvData),
				)
				raddr = relayPeerAddr
			} else {
				logger.Info("recv success",
					zap.Int("len", n),
					zap.String("laddr", conn.LocalAddr().String()),
					zap.String("raddr", raddr.String()),
					zap.Object("data", &rcvData),
				)
			}

			rspBuf, _ := json.Marshal(rspData)
			n, err = conn.WriteTo(rspBuf, raddr)
			if err != nil {
				logger.Warn("send error",
					zap.String("laddr", conn.LocalAddr().String()),
					zap.String("raddr", raddr.String()),
					zap.Error(err),
				)
				continue
			}

			// if peerKey != "" {
			// 	s.Lock()
			// 	delete(s.v, peerKey)
			// 	s.Unlock()
			// }

			logger.Debug("WriteTo client success",
				zap.Int("len", n),
				zap.String("laddr", conn.LocalAddr().String()),
				zap.String("raddr", raddr.String()),
			)
		}
	}
}

func (s *UDPServer) UDPServer(ctx context.Context, port uint) error {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return fmt.Errorf("get interfaces addrs err:%w", err)
	}
	s.initStore(ctx)
	lc := &net.ListenConfig{
		Control: reuseport.Control,
	}
	wg := sync.WaitGroup{}
	for _, address := range addrs { // Start UDP server on all address
		if ipnet, ok := address.(*net.IPNet); ok {
			if ipnet.IP.To4() != nil {
				wg.Add(1)
				go func(ip string) {
					addr := fmt.Sprintf("%s:%v", ip, port)
					if err := s.startUDPServer(ctx, lc, addr); err != nil {
						logger.Warn("start udp server error",
							zap.String("laddr", addr),
							zap.Error(err),
						)
					}
					wg.Done()
				}(ipnet.IP.String())

			}
		}
	}
	wg.Wait()
	return nil
}

type UDPClient struct {
	punched        atomic.Bool
	sync.RWMutex   // protect following var
	publicAddress1 string
	publicAddress2 string
	peerAddress    string
}

func (u *UDPClient) UDPClient(ctx context.Context, port uint, raddr1, raddr2 string, dialTimeout, pingPeerInterval, pingServerInterval, pongPeerDelay uint) (e error) {
	networkType := "udp4"
	remoteAddr1, err := net.ResolveUDPAddr(networkType, raddr1)
	if err != nil {
		return fmt.Errorf("resolve addr %s err: %w", raddr1, err)
	}

	remoteAddr2, err := net.ResolveUDPAddr(networkType, raddr2)
	if err != nil {
		return fmt.Errorf("resolve addr %s err: %w", raddr2, err)
	}

	conn, err := reuseport.ListenPacket("udp4", fmt.Sprintf(":%v", port))
	if err != nil {
		return fmt.Errorf("listen addr %s err: %w", fmt.Sprintf(":%v", port), err)
	}
	defer conn.Close()

	myID := RandomString(4)
	logger.Info("udp client start",
		zap.String("id", myID),
		zap.String("laddr", conn.LocalAddr().String()),
		zap.String("raddr1", raddr1),
		zap.String("raddr2", raddr2),
		zap.Uint("dial-timeout", dialTimeout),
	)

	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		buf := make([]byte, 1440)
		for {
			n, raddr, err := conn.ReadFrom(buf)
			if err != nil {
				logger.Warn("ReadFrom error",
					zap.String("laddr", conn.LocalAddr().String()),
					zap.Error(err),
				)
				continue
			}
			rcvData := &data{}
			if err := json.Unmarshal(buf[:n], rcvData); err != nil {
				logger.Warn("Unmarshal error",
					zap.String("raddr", raddr.String()),
					zap.ByteString("data", buf[:n]),
					zap.Error(err),
				)
				continue
			}

			rspData := &data{
				ID: myID,
				//Local: conn.LocalAddr().String(),
			}
			var relayPeerAddr *net.UDPAddr
			switch rcvData.Op {
			case "ping": // from peer
				logger.Info("got peer ping",
					zap.String("raddr", raddr.String()),
					zap.Object("data", rcvData),
				)
				rspData.Op = "pong"
				rspData.Msg = rcvData.Msg
			case "pong": // ping response from peer
				rspData.Op = "pong"
				rspData.Msg = "---------" + time.Now().Format(time.RFC3339) + "---------"
				u.punched.Store(true)
				time.Sleep(time.Duration(pongPeerDelay) * time.Millisecond)
			case "pong1": // ping1 response from server
				u.Lock()
				u.publicAddress1 = rcvData.Public
				u.Unlock()

				logger.Info("got pong1",
					zap.String("raddr", raddr.String()),
					zap.Object("data", rcvData),
				)
				if u.notCone() {
					fmt.Println("not a cone nat")
					os.Exit(0)
				}
				continue
			case "pong2": // ping2 response from server
				u.Lock()
				u.publicAddress2 = rcvData.Public
				u.peerAddress = rcvData.Peer
				u.Unlock()
				logger.Info("got pong2",
					zap.String("raddr", raddr.String()),
					zap.Object("data", rcvData),
				)
				if u.notCone() {
					fmt.Println("not a cone nat")
					os.Exit(0)
				}
				continue
			case "relay": // from server
				if rcvData.Peer == "" {
					logger.Warn("unknown realy peer addr",
						zap.Int("len", n),
						zap.String("laddr", conn.LocalAddr().String()),
						zap.String("raddr", raddr.String()),
						zap.String("op", rcvData.Op),
						zap.ByteString("content", buf[:n]),
					)
					continue
				}
				relayPeerAddr, err = net.ResolveUDPAddr(networkType, rcvData.Peer)
				if err != nil {
					logger.Warn("invalid realy peer addr",
						zap.Int("len", n),
						zap.String("laddr", conn.LocalAddr().String()),
						zap.String("raddr", raddr.String()),
						zap.String("op", rcvData.Op),
						zap.ByteString("content", buf[:n]),
						zap.Error(err),
					)
					continue
				}
				rspData.Msg = "relay " + rcvData.Msg
				rspData.Op = "ping"
			default:
				logger.Warn("unknown op",
					zap.Int("len", n),
					zap.String("laddr", conn.LocalAddr().String()),
					zap.String("raddr", raddr.String()),
					zap.String("op", rcvData.Op),
					zap.ByteString("content", buf[:n]),
				)
				continue
			}

			if relayPeerAddr != nil {
				logger.Info("recv relay success",
					zap.Int("len", n),
					zap.String("laddr", conn.LocalAddr().String()),
					zap.String("raddr", raddr.String()),
					zap.String("peer", relayPeerAddr.String()),
					zap.Object("data", rcvData),
				)
				raddr = relayPeerAddr
			} else {
				logger.Info("recv success",
					zap.Int("len", n),
					zap.String("laddr", conn.LocalAddr().String()),
					zap.String("raddr", raddr.String()),
					zap.Object("data", rcvData),
				)
			}

			rspBuf, _ := json.Marshal(rspData)
			n, err = conn.WriteTo(rspBuf, raddr)
			if err != nil {
				logger.Warn("WriteTo error",
					zap.String("laddr", conn.LocalAddr().String()),
					zap.String("raddr", raddr.String()),
					zap.Object("data", rcvData),
					zap.Error(err),
				)
				continue
			}
			logger.Info("WriteTo success",
				zap.Int("len", n),
				zap.String("laddr", conn.LocalAddr().String()),
				zap.String("raddr", raddr.String()),
				zap.Object("data", rcvData),
			)

		}
	}()

	reqData := &data{
		ID: myID,
	}
	for {
		reqData.Remote = remoteAddr1.String()
		reqData.Op = "ping1"
		reqBuf, _ := json.Marshal(reqData)
		n, err := conn.WriteTo(reqBuf, remoteAddr1)
		if err != nil {
			e = fmt.Errorf("WriteTo server %s err: %w", remoteAddr1.String(), err)
			return
		}

		logger.Debug("send ping1 success",
			zap.String("raddr", remoteAddr1.String()),
			zap.Int("len", n),
		)

		reqData.Remote = remoteAddr2.String()
		reqData.Op = "ping2"
		reqBuf2, _ := json.Marshal(reqData)
		n, err = conn.WriteTo(reqBuf2, remoteAddr2)
		if err != nil {
			e = fmt.Errorf("WriteTo server %s err: %w", remoteAddr2.String(), err)
			return
		}

		logger.Debug("send ping2 success",
			zap.String("raddr", remoteAddr2.String()),
			zap.Int("len", n),
		)

		if u.gotPeer() {
			logger.Info("got peer")
			break
		}

		if u.punched.Load() {
			logger.Info("traversal success")
			break
		}

		time.Sleep(time.Duration(pingServerInterval) * time.Millisecond)
	}

	u.RLock()
	peerAddress := u.peerAddress
	u.RUnlock()
	if len(peerAddress) < 3 {
		return fmt.Errorf("invalid peer addr: %v", peerAddress)
	}

	if !u.isCone() {
		return fmt.Errorf("you are not in cone nat")
	}

	peerAddr, err := net.ResolveUDPAddr(networkType, peerAddress)
	if err != nil {
		return fmt.Errorf("resolve peer %s err: %w", peerAddress, err)
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			if u.punched.Load() {
				logger.Info("punching success")
				return
			}
			reqData.Remote = peerAddr.String()
			reqData.Op = "ping"
			reqData.Msg = RandomString(5)
			reqBuf3, _ := json.Marshal(reqData)
			n, err := conn.WriteTo(reqBuf3, peerAddr)
			if err != nil {
				logger.Warn("WriteTo peer(ping) error",
					zap.String("laddr", conn.LocalAddr().String()),
					zap.String("paddr", peerAddr.String()),
					zap.Error(err),
				)
			} else {
				logger.Info("WriteTo peer(ping) success",
					zap.Int("len", n),
					zap.String("laddr", conn.LocalAddr().String()),
					zap.String("paddr", peerAddr.String()),
					zap.Object("data", reqData),
				)
			}

			reqData.Remote = remoteAddr1.String()
			reqData.Peer = peerAddr.String()
			reqData.Op = "relay"
			reqBuf4, _ := json.Marshal(reqData)
			n, err = conn.WriteTo(reqBuf4, remoteAddr1)
			if err != nil {
				logger.Warn("WriteTo server(relay) error",
					zap.String("raddr", remoteAddr1.String()),
					zap.String("peer", reqData.Peer),
					zap.Object("data", reqData),
					zap.Error(err),
				)
			} else {
				logger.Info("WriteTo server(relay) success",
					zap.String("raddr", remoteAddr1.String()),
					zap.String("peer", reqData.Peer),
					zap.Object("data", reqData),
					zap.Int("len", n),
				)
			}

			time.Sleep(time.Duration(pingPeerInterval) * time.Millisecond)
		}
	}()

	wg.Wait()
	return
}

func (u *UDPClient) notCone() bool {
	u.RLock()
	defer u.RUnlock()
	if u.publicAddress1 == "" { // unknown
		return false
	}

	if u.publicAddress2 == "" { // unknown
		return false
	}

	return u.publicAddress1 != u.publicAddress2
}

func (u *UDPClient) isCone() bool {
	u.RLock()
	defer u.RUnlock()
	if u.publicAddress1 == "" {
		return false
	}

	return u.publicAddress1 == u.publicAddress2
}

func (u *UDPClient) gotPeer() bool {
	u.RLock()
	defer u.RUnlock()

	return len(u.peerAddress) > 3
}

func UDPSend(ctx context.Context, laddr, raddr, data string, dialTimeout uint) (e error) {
	networkType := "udp4"
	var nla *net.UDPAddr
	var err error
	if laddr != "" {
		nla, err = net.ResolveUDPAddr(networkType, laddr)
		if err != nil {
			return fmt.Errorf("resolve local addr err:%w", err)
		}
	}

	d := net.Dialer{
		Control:   reuseport.Control,
		LocalAddr: nla,
		Timeout:   time.Duration(dialTimeout) * time.Second,
	}

	conn, err := d.DialContext(ctx, networkType, raddr)
	if err != nil {
		return fmt.Errorf("dial %s failed, err: %w", raddr, err)
	}
	defer conn.Close()

	logger.Debug("dial success",
		zap.String("raddr", conn.RemoteAddr().String()),
		zap.String("laddr", conn.LocalAddr().String()),
	)

	n, err := conn.Write([]byte(data))
	if err != nil {
		e = fmt.Errorf("send to %s err: %w", conn.RemoteAddr(), err)
		return
	}

	logger.Info("send success",
		zap.String("raddr", conn.RemoteAddr().String()),
		zap.String("data", data),
		zap.Int("len", n),
	)

	buff := make([]byte, 1440)

	n, err = conn.Read(buff)
	if err != nil {
		e = fmt.Errorf("read err: %w", err)
		return
	}

	logger.Info("recv success",
		zap.Int("len", n),
		zap.String("laddr", conn.LocalAddr().String()),
		zap.String("raddr", conn.RemoteAddr().String()),
		zap.ByteString("content", buff[:n]),
	)

	return
}
