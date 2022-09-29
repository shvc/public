package main

import (
	"context"
	"encoding/json"
	"fmt"
	mrand "math/rand"
	"net"
	"time"

	"github.com/libp2p/go-reuseport"
	"go.uber.org/zap"
)

type TCPClient struct {
	clientID    string
	networkType string
}

func (c *TCPClient) TCPClient(ctx context.Context, port uint, serverAddress1, serverAddress2 string, dialTimeout uint32) (e error) {
	if c.networkType == "" {
		c.networkType = "tcp4"
	}
	c.clientID = fmt.Sprintf("%s:%d", c.clientID, port)

	if port < 8 {
		port = uint(mrand.Uint32()%20000) + 40000
	}
	nla, err := net.ResolveTCPAddr(c.networkType, fmt.Sprintf(":%v", port))
	if err != nil {
		return fmt.Errorf("resolve local addr err:%w", err)
	}

	dialer := net.Dialer{
		Control:   reuseport.Control,
		LocalAddr: nla,
		Timeout:   time.Duration(dialTimeout) * time.Second,
	}

	conn1, err := dialer.DialContext(ctx, c.networkType, serverAddress1)
	if err != nil {
		return fmt.Errorf("dial %s failed, err: %w", serverAddress1, err)
	}
	defer conn1.Close()

	reqData := &data{
		ID: c.clientID,
	}

	reqData.Op = "ping1"
	reqBuf, _ := json.Marshal(reqData)
	n, err := conn1.Write(reqBuf)
	if err != nil {
		e = fmt.Errorf("write to server %s err: %w", conn1.RemoteAddr().String(), err)
		return
	}

	logger.Debug("ping1 send success",
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

	logger.Info("ping1 success",
		zap.String("laddr", conn1.LocalAddr().String()),
		zap.String("raddr", conn1.RemoteAddr().String()),
		zap.Object("data", rcvData1),
	)

	if serverAddress2 != "" {
		time.Sleep(2 * time.Second)
		conn2, err := dialer.DialContext(ctx, c.networkType, serverAddress2)
		if err != nil {
			return fmt.Errorf("dial %s failed, err: %w", serverAddress2, err)
		}
		defer conn2.Close()

		reqData.Op = "ping2"
		reqBuf2, _ := json.Marshal(reqData)
		n, err = conn2.Write(reqBuf2)
		if err != nil {
			e = fmt.Errorf("write to server %s err: %w", conn2.RemoteAddr().String(), err)
			return
		}

		logger.Debug("send ping2 success",
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

		logger.Info("ping2 success",
			zap.String("laddr", conn2.LocalAddr().String()),
			zap.String("raddr", conn2.RemoteAddr().String()),
			zap.Object("data", rcvData2),
			zap.Bool("cone", rcvData1.Public == rcvData2.Public),
		)
	}

	return
}
