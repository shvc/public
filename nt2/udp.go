package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
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
	status int
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

func (s *UDPServer) set(v data, status int) {
	s.Lock()
	defer s.Unlock()
	s.v[v.ID] = store{expire: time.Now().Unix() + 10, data: v, status: status}
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

func (s *UDPServer) selectOnePeer(exclude string, status int) (string, string) {
	s.RLock()
	defer s.RUnlock()

	for k, v := range s.v {
		if k == exclude {
			continue
		}
		if v.status != status {
			continue
		}

		return k, v.data.Public
	}

	return "", ""
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
		Op:     "pong3",
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
			logger.Info("recv success",
				zap.Int("len", n),
				zap.String("laddr", conn.LocalAddr().String()),
				zap.String("raddr", raddr.String()),
				zap.Object("data", &rcvData),
			)

			rspData := &data{
				ID:     rcvData.ID,
				Public: rcvData.Public,
			}

			switch rcvData.Op {
			case "ping1":
				s.set(rcvData, 1)
				rspData.Op = "pong1"
			case "ping2":
				s.set(rcvData, 2)
				rspData.Op = "pong2"
			case "ping3":
				s.set(rcvData, 3)
				rspData.Op = "pong3"
				rspData.Msg, rspData.Peer = s.selectOnePeer(rcvData.ID, 3)
				if rspData.Msg == "" || rspData.Peer == "" {
					continue
				}
				if err := s.notify(conn, rspData.Msg, rspData.Peer, rcvData.Public); err != nil {
					logger.Warn("notify peer error",
						zap.String("peer addr", rspData.Peer),
						zap.String("peer id", rspData.Msg),
						zap.Error(err),
					)
					continue
				}
				s.delete(rcvData.ID)
				s.delete(rspData.Msg)
				logger.Debug("notify peer success",
					zap.String("peer addr", rspData.Peer),
					zap.String("peer id", rspData.Msg),
				)
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
			rspBuf, _ := json.Marshal(rspData)
			n, err = conn.WriteTo(rspBuf, raddr)
			if err != nil {
				logger.Warn("send error",
					zap.String("laddr", conn.LocalAddr().String()),
					zap.String("raddr", raddr.String()),
					zap.Object("data", rspData),
					zap.Error(err),
				)
				continue
			}

			logger.Info("WriteTo client success",
				zap.Int("len", n),
				zap.String("laddr", conn.LocalAddr().String()),
				zap.String("raddr", raddr.String()),
				zap.Object("data", rspData),
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
	punched      atomic.Bool
	sync.RWMutex // protect following var
	peerAddress  string
}

func (u *UDPClient) getPeerAddress() string {
	u.RLock()
	defer u.RUnlock()

	return u.peerAddress
}

func (u *UDPClient) setPeerAddress(paddr string) {
	u.Lock()
	defer u.Unlock()
	u.peerAddress = paddr
}

func (u *UDPClient) UDPClient(ctx context.Context, port uint, raddr1, raddr2 string, dialTimeout, pingServerInterval, pingPeerInterval, helloInterval uint) (e error) {
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

	reqData.Remote = remoteAddr1.String()
	reqData.Op = "ping1"
	reqBuf, _ := json.Marshal(reqData)
	_, err = conn.WriteTo(reqBuf, remoteAddr1)
	if err != nil {
		e = fmt.Errorf("ping1 WriteTo server %s err: %w", remoteAddr1.String(), err)
		return
	}
	n, sraddr1, err := conn.ReadFrom(buf)
	if err != nil {
		e = fmt.Errorf("ping1 ReadFrom server %s err: %w", remoteAddr1.String(), err)
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

	reqData.Remote = remoteAddr2.String()
	reqData.Op = "ping2"
	reqBuf2, _ := json.Marshal(reqData)
	_, err = conn.WriteTo(reqBuf2, remoteAddr2)
	if err != nil {
		e = fmt.Errorf("WriteTo server %s err: %w", remoteAddr2.String(), err)
		return
	}

	n, sraddr2, err := conn.ReadFrom(buf)
	if err != nil {
		e = fmt.Errorf("ping2 ReadFrom server %s err: %w", remoteAddr1.String(), err)
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
		e = fmt.Errorf("not cone, public addr %s != %s", rcvData1.Public, rcvData2.Public)
		return
	}

	logger.Info("ping2 success",
		zap.String("raddr", sraddr2.String()),
		zap.String("public", rcvData2.Public),
		zap.Object("response", rcvData2),
	)

	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			n, nraddr, err := conn.ReadFrom(buf)
			if err != nil {
				logger.Warn("ReadFrom error",
					zap.String("laddr", conn.LocalAddr().String()),
					zap.String("raddr", nraddr.String()),
					zap.Error(err),
				)
				break
			}
			rcvData := &data{}
			if err := json.Unmarshal(buf[:n], rcvData); err != nil {
				logger.Warn("Unmarshal response error",
					zap.String("laddr", conn.LocalAddr().String()),
					zap.String("raddr", nraddr.String()),
					zap.Error(err),
				)
				continue
			}

			switch rcvData.Op {
			case "pong3": // from server
				if rcvData.Peer != "" {
					u.setPeerAddress(rcvData.Peer)
				}
			case "pping": // from peer
				u.punched.Store(true)
			case "hello": // from peer
				// p2p comunication
			default:
				logger.Warn("recv unknown msg",
					zap.String("laddr", conn.LocalAddr().String()),
					zap.String("raddr", nraddr.String()),
					zap.Object("data", rcvData),
				)
				continue
			}

			logger.Info("recv msg",
				zap.String("laddr", conn.LocalAddr().String()),
				zap.String("raddr", nraddr.String()),
				zap.Object("data", rcvData),
			)
		}

	}()

	peerAddress := ""
	for {
		reqData.Remote = remoteAddr1.String()
		reqData.Op = "ping3"
		reqBuf, _ := json.Marshal(reqData)
		_, err = conn.WriteTo(reqBuf, remoteAddr1)
		if err != nil {
			e = fmt.Errorf("ping1 WriteTo server %s err: %w", remoteAddr1.String(), err)
			return
		}

		peerAddress = u.getPeerAddress()
		if peerAddress != "" {
			break
		}

		logger.Info("ping3 success",
			zap.String("raddr", remoteAddr1.String()),
			zap.Object("data", reqData),
		)

		time.Sleep(time.Duration(pingServerInterval) * time.Second)
	}

	peerAddr, err := net.ResolveUDPAddr(u.networkType, peerAddress)
	if err != nil {
		return fmt.Errorf("resolve peer %s err: %w", peerAddress, err)
	}

	for {
		//reqData.Remote = peerAddr.String()
		reqData.Op = "pping"
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

		if u.punched.Load() {
			break
		}
		logger.Debug("ping peer success",
			zap.String("laddr", conn.LocalAddr().String()),
			zap.String("paddr", peerAddr.String()),
		)
		time.Sleep(time.Duration(pingPeerInterval) * time.Millisecond)
	}

	for {
		reqData.Remote = peerAddr.String()
		reqData.Op = "hello"
		reqData.Msg = "Hello @ " + time.Now().Format(time.RFC3339)
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
		time.Sleep(time.Duration(helloInterval) * time.Second)
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
