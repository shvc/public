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
	buf := make([]byte, 1892)
	for {
		n, err := conn.Read(buf)
		if err != nil {
			if err == io.EOF {
				logger.Info("conn closed",
					zap.String("laddr", conn.LocalAddr().String()),
					zap.String("raddr", conn.RemoteAddr().String()),
				)
			} else {
				logger.Warn("recv error",
					zap.String("laddr", conn.LocalAddr().String()),
					zap.String("raddr", conn.RemoteAddr().String()),
					zap.Error(err),
				)
			}
			break
		}

		rcvData := &data{}
		if err := json.Unmarshal(buf[:n], rcvData); err != nil {
			logger.Warn("readFrom success but decode error",
				zap.Int("len", n),
				zap.String("laddr", conn.LocalAddr().String()),
				zap.String("raddr", conn.RemoteAddr().String()),
				zap.ByteString("content", buf[:n]),
				zap.Error(err),
			)
			continue
		}

		if rcvData.ID == "" {
			logger.Warn("readFrom success but no ID",
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
			logger.Warn("send error",
				zap.String("laddr", conn.LocalAddr().String()),
				zap.String("raddr", conn.RemoteAddr().String()),
				zap.Error(err),
			)
			break
		}

		logger.Debug("send success",
			zap.String("laddr", conn.LocalAddr().String()),
			zap.String("raddr", conn.RemoteAddr().String()),
			zap.Int("len", n),
		)
	}
}

func (s *PublicServer) set(v data, status int) {
	s.Lock()
	defer s.Unlock()
	s.v[v.ID] = store{expire: time.Now().Unix() + 10, data: v, status: status}
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

func (s *PublicServer) notify(conn net.PacketConn, ID, addr, peerAddr string) error {
	pAddr, err := net.ResolveUDPAddr("udp4", addr)
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

func (s *PublicServer) startUDPServer(ctx context.Context, lc *net.ListenConfig, addr string) error {
	conn, err := lc.ListenPacket(ctx, "udp4", addr)
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
			case "report":
				s.set(rcvData, 3)
				logger.Info("report status",
					zap.String("raddr", raddr.String()),
					zap.Object("data", &rcvData),
				)
				continue
			case "request":
				rspData.Op = "pong3"
				rspData.Msg, rspData.Peer = s.selectOnePeer(rcvData.ID, 3)
				if rspData.Msg != "" || rspData.Peer != "" {
					if err := s.notify(conn, rspData.Msg, rspData.Peer, rcvData.Public); err != nil {
						logger.Warn("notify peer error",
							zap.String("laddr", conn.LocalAddr().String()),
							zap.String("raddr", rspData.Peer),
							zap.String("peer address", rcvData.Public),
							zap.String("peer id", rspData.Msg),
							zap.Error(err),
						)
					} else {
						logger.Info("notify peer success",
							zap.String("laddr", conn.LocalAddr().String()),
							zap.String("raddr", rspData.Peer),
							zap.String("peer address", rcvData.Public),
							zap.String("peer id", rspData.Msg),
						)
					}
				} else {
					logger.Warn("peer not found",
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
			rspBuf, _ := json.Marshal(rspData)
			_, err = conn.WriteTo(rspBuf, raddr)
			if err != nil {
				logger.Warn("send response error",
					zap.String("laddr", conn.LocalAddr().String()),
					zap.String("raddr", raddr.String()),
					zap.Object("data", rspData),
					zap.Error(err),
				)
				continue
			}

			logger.Info("response success",
				zap.String("laddr", conn.LocalAddr().String()),
				zap.String("raddr", raddr.String()),
				zap.Object("data", rspData),
			)
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
