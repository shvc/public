package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sync"
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
	networkType string
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

func (s *UDPServer) notify(conn net.PacketConn, ID, addr, peerAddr string) error {
	pAddr, err := net.ResolveUDPAddr(s.networkType, addr)
	if err != nil {
		return fmt.Errorf("resolve notify addr %s err: %w", addr, err)
	}
	rspData := &data{
		ID:     ID,
		Public: addr,
		Peer:   peerAddr,
		Op:     "pong",
	}
	rspBuf, _ := json.Marshal(rspData)
	_, err = conn.WriteTo(rspBuf, pAddr)
	return err
}

func (s *UDPServer) startUDPServer(ctx context.Context, lc *net.ListenConfig, addr string) error {
	conn, err := lc.ListenPacket(ctx, s.networkType, addr)
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
				ID:     rcvData.ID,
				Public: rcvData.Public,
				Op:     "pong",
			}

			switch rcvData.Op {
			case "ping1":
				s.set(rcvData)
			case "ping2":
				prevData, ok := s.get(rcvData.ID)
				if ok {
					if prevData.Public == rcvData.Public {
						rspData.Msg, rspData.Peer = s.selectOnePeer(rcvData.ID)
						if rspData.Msg != "" && rspData.Peer != "" {
							if err := s.notify(conn, rspData.Msg, rspData.Peer, rcvData.Public); err != nil {
								logger.Warn("notify peer error",
									zap.String("peer addr", rspData.Peer),
									zap.String("peer id", rspData.Msg),
								)
							} else {
								s.delete(rcvData.ID)
								s.delete(rspData.Msg)
								logger.Debug("notify peer success",
									zap.String("peer addr", rspData.Peer),
									zap.String("peer id", rspData.Msg),
								)
							}
						}
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
						zap.Object("data", &rcvData),
					)
				}
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

			logger.Info("recv success",
				zap.Int("len", n),
				zap.String("laddr", conn.LocalAddr().String()),
				zap.String("raddr", raddr.String()),
				zap.Object("data", &rcvData),
			)

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

			logger.Debug("WriteTo client success",
				zap.Int("len", n),
				zap.String("laddr", conn.LocalAddr().String()),
				zap.String("raddr", raddr.String()),
			)
		}
	}
}

func (s *UDPServer) UDPServer(ctx context.Context, port uint) error {
	s.networkType = "udp4"
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
	clientID     string
	networkType  string
	sync.RWMutex // protect following var
	peerAddress  string
}

func (u *UDPClient) UDPClient(ctx context.Context, port uint, raddr1, raddr2 string, dialTimeout, pingPeerInterval, pingServerInterval, pongPeerDelay uint) (e error) {
	u.networkType = "udp4"
	remoteAddr1, err := net.ResolveUDPAddr(u.networkType, raddr1)
	if err != nil {
		return fmt.Errorf("resolve addr %s err: %w", raddr1, err)
	}

	remoteAddr2, err := net.ResolveUDPAddr(u.networkType, raddr2)
	if err != nil {
		return fmt.Errorf("resolve addr %s err: %w", raddr2, err)
	}

	conn, err := reuseport.ListenPacket("udp4", fmt.Sprintf(":%v", port))
	if err != nil {
		return fmt.Errorf("listen addr %s err: %w", fmt.Sprintf(":%v", port), err)
	}
	defer conn.Close()

	if u.clientID == "" {
		u.clientID = RandomString(5)
	}

	logger.Info("udp client start",
		zap.String("id", u.clientID),
		zap.String("laddr", conn.LocalAddr().String()),
		zap.String("raddr1", raddr1),
		zap.String("raddr2", raddr2),
		zap.Uint("dial-timeout", dialTimeout),
	)

	reqData := &data{
		ID: u.clientID,
	}
	buf := make([]byte, 2048)
	for {
		reqData.Remote = remoteAddr1.String()
		reqData.Op = "ping1"
		reqBuf, _ := json.Marshal(reqData)
		_, err := conn.WriteTo(reqBuf, remoteAddr1)
		if err != nil {
			e = fmt.Errorf("ping1 WriteTo server %s err: %w", remoteAddr1.String(), err)
			return
		}

		conn.SetReadDeadline(time.Now().Add(20 * time.Second))
		n, raddr1, err := conn.ReadFrom(buf)
		if err != nil {
			e = fmt.Errorf("ping1 ReadFrom server %s err: %w", remoteAddr1.String(), err)
			return
		}
		if raddr1.String() != remoteAddr1.String() && raddr1.String() != remoteAddr2.String() {
			logger.Info("ping1 recv msg not from server",
				zap.String("raddr", raddr1.String()),
			)
			u.peerAddress = raddr1.String()
			break
		}
		rcvData1 := &data{}
		if err := json.Unmarshal(buf[:n], rcvData1); err != nil {
			e = fmt.Errorf("ping1 %s Unmarshal %s err: %w", raddr1.String(), buf[:n], err)
			return
		}
		if rcvData1.Op != "pong" {
			e = fmt.Errorf("ping1 %s got invalid response %s", raddr1.String(), rcvData1.Op)
			return
		}
		if rcvData1.Peer != "" {
			logger.Info("ping1 got peer",
				zap.String("raddr", remoteAddr1.String()),
				zap.Object("data", rcvData1),
			)
			u.peerAddress = rcvData1.Peer
			break
		}

		logger.Debug("ping1 server success",
			zap.String("raddr", remoteAddr1.String()),
			zap.Object("data", rcvData1),
		)

		reqData.Remote = remoteAddr2.String()
		reqData.Op = "ping2"
		reqBuf2, _ := json.Marshal(reqData)
		_, err = conn.WriteTo(reqBuf2, remoteAddr2)
		if err != nil {
			e = fmt.Errorf("WriteTo server %s err: %w", remoteAddr2.String(), err)
			return
		}
		conn.SetReadDeadline(time.Now().Add(20 * time.Second))
		n, raddr2, err := conn.ReadFrom(buf)
		if err != nil {
			e = fmt.Errorf("ping2 ReadFrom server %s err: %w", remoteAddr1.String(), err)
			return
		}
		if raddr2.String() != remoteAddr1.String() && raddr2.String() != remoteAddr2.String() {
			logger.Info("ping1 recv msg not from server",
				zap.String("raddr", raddr2.String()),
			)
			u.peerAddress = raddr2.String()
			break
		}
		rcvData2 := &data{}
		if err := json.Unmarshal(buf[:n], rcvData2); err != nil {
			e = fmt.Errorf("ping2 %s Unmarshal %s err: %w", raddr2.String(), buf[:n], err)
			return
		}
		if rcvData2.Op != "pong" {
			e = fmt.Errorf("ping2 %s got invalid response %s", raddr2.String(), rcvData2.Op)
			return
		}
		if rcvData2.Peer != "" {
			logger.Info("ping2 got peer",
				zap.String("raddr", remoteAddr2.String()),
				zap.Object("data", rcvData2),
			)
			u.peerAddress = rcvData2.Peer
			break
		}

		if rcvData1.Public != rcvData2.Public {
			logger.Info("not cone nat",
				zap.String("public1", rcvData1.Public),
				zap.String("public2", rcvData2.Public),
			)
			return
		}

		logger.Info("ping1 ping2 and response success",
			zap.String("raddr1", remoteAddr1.String()),
			zap.String("raddr2", remoteAddr2.String()),
			zap.String("public1", rcvData1.Public),
			zap.String("public2", rcvData2.Public),
			zap.Object("data", rcvData2),
		)

		time.Sleep(time.Duration(pingServerInterval) * time.Millisecond)
	}

	peerAddr, err := net.ResolveUDPAddr(u.networkType, u.peerAddress)
	if err != nil {
		return fmt.Errorf("resolve peer %s err: %w", u.peerAddress, err)
	}
	wg := sync.WaitGroup{}
	for {
		reqData.Remote = peerAddr.String()
		reqData.Op = "ping"
		reqData.Msg = "ping peer"
		reqBuf3, _ := json.Marshal(reqData)
		_, err := conn.WriteTo(reqBuf3, peerAddr)
		if err != nil {
			logger.Warn("ping(WriteTo) peer error",
				zap.String("laddr", conn.LocalAddr().String()),
				zap.String("paddr", peerAddr.String()),
				zap.Error(err),
			)
			continue
		}

		logger.Debug("ping peer",
			zap.String("laddr", conn.LocalAddr().String()),
			zap.String("paddr", peerAddr.String()),
			zap.Object("data", reqData),
		)

		conn.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
		n, praddr, err := conn.ReadFrom(buf)
		if err != nil {
			// !strings.Contains(err.Error(), "i/o timeout") {
			logger.Warn("ping ReadFrom peer error",
				zap.String("laddr", conn.LocalAddr().String()),
				zap.String("paddr", peerAddr.String()),
				zap.Error(err),
			)

			continue
		}
		conn.SetReadDeadline(time.Time{})

		rcvData := &data{}
		if err := json.Unmarshal(buf[:n], rcvData); err != nil {
			logger.Warn("ping Unmarshal peer response error",
				zap.String("laddr", conn.LocalAddr().String()),
				zap.String("raddr", praddr.String()),
				zap.String("paddr", peerAddr.String()),
				zap.Error(err),
			)
			continue
		}

		logger.Info("ping peer and read success",
			zap.String("laddr", conn.LocalAddr().String()),
			zap.String("raddr", praddr.String()),
			zap.String("paddr", peerAddr.String()),
			zap.Object("data", rcvData),
		)

		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				n, nraddr, err := conn.ReadFrom(buf)
				if err != nil {
					logger.Warn("ReadFrom peer error",
						zap.String("laddr", conn.LocalAddr().String()),
						zap.String("raddr", nraddr.String()),
						zap.Error(err),
					)
					break
				}
				rcvData := &data{}
				if err := json.Unmarshal(buf[:n], rcvData); err != nil {
					logger.Warn("Unmarshal peer response error",
						zap.String("laddr", conn.LocalAddr().String()),
						zap.String("raddr", nraddr.String()),
						zap.Error(err),
					)
					continue
				}
				logger.Info("peer msg",
					zap.String("laddr", conn.LocalAddr().String()),
					zap.String("raddr", nraddr.String()),
					zap.Object("data", rcvData),
				)
			}

		}()

		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				reqData.Remote = peerAddr.String()
				reqData.Op = "data"
				reqData.Msg = "Hello " + time.Now().Format(time.RFC3339)
				reqBuf3, _ := json.Marshal(reqData)
				_, err := conn.WriteTo(reqBuf3, peerAddr)
				if err != nil {
					logger.Warn("ping WriteTo peer error",
						zap.String("laddr", conn.LocalAddr().String()),
						zap.String("raddr", peerAddr.String()),
						zap.Error(err),
					)
					break
				}
				time.Sleep(time.Duration(pingPeerInterval) * time.Millisecond)
			}
		}()

		break

	}

	wg.Wait()

	return
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
