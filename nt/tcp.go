package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/libp2p/go-reuseport"
	"go.uber.org/zap"
)

func TCPServer(port uint) error {
	networkType := "tcp4"
	addr := fmt.Sprintf(":%v", port)
	listener, err := reuseport.Listen(networkType, addr)
	if err != nil {
		return fmt.Errorf("listen fail, err: %w", err)
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
		go processTCPConn(conn)
	}
}

func processTCPConn(conn net.Conn) {
	defer conn.Close()
	buf := make([]byte, 2048)
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
			ID: rcvData.ID,
			//Local:  conn.LocalAddr().String(),
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

func TCPClient(ctx context.Context, port uint, raddr1, raddr2 string, dialTimeout uint) (e error) {
	networkType := "tcp4"
	var nla *net.TCPAddr
	var err error
	if port > 0 {
		nla, err = net.ResolveTCPAddr(networkType, fmt.Sprintf(":%v", port))
		if err != nil {
			return fmt.Errorf("resolve local addr err:%w", err)
		}
	}

	dialer := net.Dialer{
		Control:   reuseport.Control,
		LocalAddr: nla,
		Timeout:   time.Duration(dialTimeout) * time.Second,
	}

	conn1, err := dialer.DialContext(ctx, networkType, raddr1)
	if err != nil {
		return fmt.Errorf("dial %s failed, err: %w", raddr1, err)
	}
	defer conn1.Close()
	myID := RandomString(4)

	reqData := &data{
		ID: myID,
	}
	reqData.Remote = conn1.RemoteAddr().String()
	reqData.Op = "ping1"
	reqBuf, _ := json.Marshal(reqData)
	n, err := conn1.Write(reqBuf)
	if err != nil {
		e = fmt.Errorf("write to server %s err: %w", conn1.RemoteAddr().String(), err)
		return
	}

	logger.Info("send ping1 success",
		zap.String("laddr", conn1.LocalAddr().String()),
		zap.String("raddr", conn1.RemoteAddr().String()),
		zap.Int("len", n),
	)

	buf := make([]byte, 2048)
	n, err = conn1.Read(buf)
	if err != nil {
		e = fmt.Errorf("read server %s err: %w", conn1.RemoteAddr().String(), err)
		return
	}
	rcvData1 := &data{}
	if err := json.Unmarshal(buf[:n], rcvData1); err != nil {
		e = fmt.Errorf("decode server %s err: %w", conn1.RemoteAddr().String(), err)
		return
	}

	logger.Info("recv(ping1) success",
		zap.String("laddr", conn1.LocalAddr().String()),
		zap.String("raddr", conn1.RemoteAddr().String()),
		zap.Object("data", rcvData1),
	)

	if raddr2 != "" {
		conn2, err := dialer.DialContext(ctx, networkType, raddr2)
		if err != nil {
			return fmt.Errorf("dial %s failed, err: %w", raddr2, err)
		}
		defer conn2.Close()

		reqData.Remote = conn2.RemoteAddr().String()
		reqData.Op = "ping2"
		reqBuf2, _ := json.Marshal(reqData)
		n, err = conn2.Write(reqBuf2)
		if err != nil {
			e = fmt.Errorf("write to server %s err: %w", conn2.RemoteAddr().String(), err)
			return
		}

		logger.Info("send ping2 success",
			zap.String("laddr", conn2.LocalAddr().String()),
			zap.String("raddr", conn2.RemoteAddr().String()),
			zap.Int("len", n),
		)

		n, err = conn2.Read(buf)
		if err != nil {
			e = fmt.Errorf("read server %s err: %w", conn2.RemoteAddr().String(), err)
			return
		}
		rcvData2 := &data{}
		if err := json.Unmarshal(buf[:n], rcvData2); err != nil {
			e = fmt.Errorf("decode server %s err: %w", conn2.RemoteAddr().String(), err)
			return
		}

		logger.Info("recv(ping2) success",
			zap.String("laddr", conn2.LocalAddr().String()),
			zap.String("raddr", conn2.RemoteAddr().String()),
			zap.Object("data", rcvData2),
			zap.Bool("cone", rcvData1.Public == rcvData2.Public),
		)
	}

	return
}
