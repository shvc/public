package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/libp2p/go-reuseport"
	"go.uber.org/zap"
)

type store struct {
	expire int64
	status int
	data
}

type PublicServer struct {
	reportInterval uint32
	sync.RWMutex
	v map[string]store
}

func (s *PublicServer) Start(ctx context.Context, port uint) error {
	go func() {
		if err := s.TCPServer(port); err != nil {
			panic(fmt.Sprintf("start tcp server error %s", err))
		}
	}()

	if err := s.UDPServer(ctx, port); err != nil {
		panic(fmt.Sprintf("start udp server error %s", err))
	}

	return nil
}

func (s *PublicServer) TCPServer(port uint) error {
	addr := fmt.Sprintf(":%v", port)
	listener, err := reuseport.Listen("tcp4", addr)
	if err != nil {
		return fmt.Errorf("listen failed, error: %w", err)
	}

	logger.Info("tcp server started",
		zap.String("addr", listener.Addr().String()),
	)

	for {
		conn, err := listener.Accept()
		if err != nil {
			logger.Warn("accept error",
				zap.String("addr", listener.Addr().String()),
				zap.Error(err),
			)
			continue
		}
		go s.processTCPConn(conn)
	}
}

func (s *PublicServer) processTCPConn(conn net.Conn) {
	defer conn.Close()
	buf := make([]byte, 2048)
	for {
		n, err := conn.Read(buf)
		if err != nil {
			if err == io.EOF {
				logger.Info("tcp conn closed",
					zap.String("laddr", conn.LocalAddr().String()),
					zap.String("raddr", conn.RemoteAddr().String()),
				)
			} else {
				logger.Warn("tcp recv error",
					zap.String("laddr", conn.LocalAddr().String()),
					zap.String("raddr", conn.RemoteAddr().String()),
					zap.Error(err),
				)
			}
			break
		}

		rcvData := &data{}
		if err := json.Unmarshal(buf[:n], rcvData); err != nil {
			logger.Warn("tcp decode json error",
				zap.Int("len", n),
				zap.String("laddr", conn.LocalAddr().String()),
				zap.String("raddr", conn.RemoteAddr().String()),
				zap.ByteString("content", buf[:n]),
				zap.Error(err),
			)
			continue
		}

		if rcvData.ID == "" {
			logger.Warn("tcp req data no ID",
				zap.Int("len", n),
				zap.String("laddr", conn.LocalAddr().String()),
				zap.String("raddr", conn.RemoteAddr().String()),
				zap.ByteString("content", buf[:n]),
			)
			continue
		}
		// set public addr
		rcvData.Public = conn.RemoteAddr().String()

		rspData := &data{
			ID:     rcvData.ID,
			Public: rcvData.Public,
		}

		switch rcvData.Op {
		case "ping1":
			rspData.Op = "pong1"
		case "ping2":
			rspData.Op = "pong2"
		}

		rspBuf, _ := json.Marshal(rspData)
		n, err = conn.Write(rspBuf)
		if err != nil {
			logger.Warn("tcp send resp error",
				zap.String("laddr", conn.LocalAddr().String()),
				zap.String("raddr", conn.RemoteAddr().String()),
				zap.Error(err),
			)
			break
		}

		logger.Debug("tcp send resp success",
			zap.String("laddr", conn.LocalAddr().String()),
			zap.String("raddr", conn.RemoteAddr().String()),
			zap.Int("len", n),
		)
	}
}

func (s *PublicServer) set(v data, status int) {
	s.Lock()
	defer s.Unlock()
	s.v[v.ID] = store{expire: time.Now().Unix() + int64(s.reportInterval) + 10, data: v, status: status}
}

func (s *PublicServer) delete(k string) {
	s.Lock()
	defer s.Unlock()
	delete(s.v, k)
}

func (s *PublicServer) get(k string) (d data, ok bool) {
	s.RLock()
	defer s.RUnlock()
	if v, got := s.v[k]; got {
		d = v.data
		ok = got
	}
	return
}

func (s *PublicServer) initStore(ctx context.Context) {
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

func (s *PublicServer) selectOnePeer(exclude string, status int) (string, string) {
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

func (s *PublicServer) notify(conn net.PacketConn, ID, addr, peerAddr string, pingNum uint32) error {
	pAddr, err := net.ResolveUDPAddr("udp4", addr)
	if err != nil {
		return fmt.Errorf("resolve notify addr %s err: %w", addr, err)
	}
	rspData := &data{
		ID:      ID,
		Public:  addr,
		Peer:    peerAddr,
		Op:      "pong3",
		PingNum: pingNum,
	}

	return s.writeData(conn, pAddr, rspData)
}

func (u *PublicServer) readData(conn net.PacketConn) (dat data, raddr net.Addr, e error) {
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

func (u *PublicServer) writeData(conn net.PacketConn, raddr net.Addr, dat *data) error {
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

func (s *PublicServer) startUDPServer(ctx context.Context, lc *net.ListenConfig, addr string) error {
	conn, err := lc.ListenPacket(ctx, "udp4", addr)
	if err != nil {
		return fmt.Errorf("listen addr %s fail, err: %w", addr, err)
	}
	defer conn.Close()

	logger.Info("udp server started",
		zap.String("addr", conn.LocalAddr().String()),
	)

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
			rcvData, raddr, err := s.readData(conn)
			if err != nil {
				logger.Warn("readData error",
					zap.String("laddr", conn.LocalAddr().String()),
					zap.Error(err),
				)
				continue
			}

			if rcvData.ID == "" {
				logger.Warn("readData success but no ID",
					zap.String("laddr", conn.LocalAddr().String()),
					zap.String("raddr", raddr.String()),
				)
				continue
			}

			go func() {
				// set public addr
				rcvData.Public = raddr.String()
				rspData := &data{
					ID:     rcvData.ID,
					Public: rcvData.Public,
				}

				switch rcvData.Op {
				case "ping1":
					//s.set(rcvData, 1)
					logger.Debug("recv ping1",
						zap.Object("req", &rcvData),
					)
					rspData.Op = "pong1"
				case "ping2":
					//s.set(rcvData, 2)
					logger.Debug("recv ping2",
						zap.Object("req", &rcvData),
					)
					rspData.Op = "pong2"
				case "report":
					s.set(rcvData, 3)
					logger.Info("report",
						zap.String("raddr", raddr.String()),
						zap.Object("req", &rcvData),
					)
					return
				case "request":
					logger.Debug("recv request",
						zap.Object("req", &rcvData),
					)
					rspData.Op = "pong3"
					rspData.Msg, rspData.Peer = s.selectOnePeer(rcvData.ID, 3)
					if rspData.Peer == "" {
						logger.Warn("no peer server candidate",
							zap.String("raddr", raddr.String()),
							zap.Object("req", &rcvData),
						)
						return
					}
					if err := s.notify(conn, rspData.Msg, rspData.Peer, rcvData.Public, rcvData.PingNum); err != nil {
						logger.Warn("notify peer server error",
							zap.String("laddr", conn.LocalAddr().String()),
							zap.String("peer server", rspData.Peer),
							zap.String("peer client", rcvData.Public),
							zap.String("peer id", rspData.Msg),
							zap.Error(err),
						)
						return
					}
					logger.Info("notify peer server success",
						zap.String("laddr", conn.LocalAddr().String()),
						zap.String("raddr", rspData.Peer),
						zap.String("peer address", rcvData.Public),
						zap.String("peer id", rspData.Msg),
					)
				default:
					logger.Warn("unknown op",
						zap.String("laddr", conn.LocalAddr().String()),
						zap.String("raddr", raddr.String()),
						zap.String("op", rcvData.Op),
					)
					return
				}

				err = s.writeData(conn, raddr, rspData)
				if err != nil {
					logger.Warn("send response error",
						zap.Object("req", &rcvData),
						zap.Object("resp", rspData),
						zap.Error(err),
					)
					return
				}

				logger.Info("response success",
					zap.Object("req", &rcvData),
					zap.Object("resp", rspData),
				)
			}()

		}
	}
}

func (s *PublicServer) UDPServer(ctx context.Context, port uint) error {
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
