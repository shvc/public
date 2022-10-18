package main

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/denisbrodbeck/machineid"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	version          = "0.0.0"
	clientID         = ""
	logger           *zap.Logger
	debug            bool
	serverPort       uint   = 20018
	localPort        uint   = 0
	serverAddress1          = fmt.Sprintf("47.100.31.117:%v", serverPort)
	serverAddress2          = fmt.Sprintf("47.103.138.1:%v", serverPort)
	dialTimeout      uint32 = 5
	reportInterval   uint32 = 20
	pingPeerInterval uint32 = 100
)

func main() {
	var rootCmd = &cobra.Command{
		Use:     "nt",
		Short:   "nt",
		Long:    "nat traversal tool",
		Version: version,
		Hidden:  true,
		PersistentPreRunE: func(*cobra.Command, []string) error {
			initLogger(debug)
			ID, err := machineid.ID()
			if err != nil {
				return err
			}
			h := md5.New()
			h.Write([]byte(ID))
			clientID = hex.EncodeToString(h.Sum(nil))
			return nil
		},
	}
	rootCmd.PersistentFlags().BoolVarP(&debug, "debug", "", false, "show debug log")
	rootCmd.PersistentFlags().UintVarP(&serverPort, "port", "p", serverPort, "serve(listen) port")
	rootCmd.PersistentFlags().UintVarP(&localPort, "local-port", "P", localPort, "local port")
	rootCmd.PersistentFlags().Uint32Var(&dialTimeout, "dial-timeout", dialTimeout, "client dial timeout")
	rootCmd.PersistentFlags().StringVar(&serverAddress1, "s1", serverAddress1, "server address1")
	rootCmd.PersistentFlags().StringVar(&serverAddress2, "s2", serverAddress2, "server address2")
	rootCmd.PersistentFlags().Uint32Var(&reportInterval, "report-interval", reportInterval, "report status to public server interval in second")
	rootCmd.PersistentFlags().Uint32Var(&pingPeerInterval, "ping-peer-interval", pingPeerInterval, "ping peer random interval in millsecond")

	serverCmd := &cobra.Command{
		Use:   "server",
		Short: "public server",
		Long: `public server:
* start udp server
ntn server
`,
		Args: cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			sv := &PublicServer{
				reportInterval: reportInterval,
			}
			return sv.Start(ctx, serverPort)
		},
	}
	rootCmd.AddCommand(serverCmd)

	tcpClientCmd := &cobra.Command{
		Use:     "tcp-client",
		Aliases: []string{"tc"},
		Short:   "tcp client",
		Long: `tcp client:
* run tcp client
ntn tc
`,
		Args: cobra.MinimumNArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
			defer stop()
			ts := TCPClient{
				clientID: clientID,
			}
			return ts.TCPClient(ctx, localPort, serverAddress1, serverAddress2, dialTimeout)
		},
	}
	rootCmd.AddCommand(tcpClientCmd)

	udpPeerServerCmd := &cobra.Command{
		Use:     "udp-peer-server",
		Aliases: []string{"us"},
		Short:   "udp peer server",
		Long: `udp peer server:
* start udp peer server
ntn us
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			u := NewUdpPeer(clientID, serverAddress1, serverAddress2)
			return u.UDPPeerServer(ctx, localPort, dialTimeout, reportInterval, pingPeerInterval)
		},
	}
	rootCmd.AddCommand(udpPeerServerCmd)

	var helloInterval uint32 = 10
	var pingPeerNum uint32 = 20
	udpPeerClientCmd := &cobra.Command{
		Use:     "udp-peer-client",
		Aliases: []string{"uc"},
		Short:   "udp peer client",
		Long: `udp peer client:
* run udp client
ntn uc
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			u := NewUdpPeer(clientID, serverAddress1, serverAddress2)
			return u.UDPPeerClient(ctx, localPort, dialTimeout, reportInterval, pingPeerInterval, pingPeerNum, helloInterval)
		},
	}
	udpPeerClientCmd.Flags().Uint32Var(&helloInterval, "hello-interval", helloInterval, "say hello interval in second")
	udpPeerClientCmd.Flags().Uint32Var(&pingPeerNum, "ping-peer-num", pingPeerNum, "ping peer total num")
	rootCmd.AddCommand(udpPeerClientCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

// init logger
func initLogger(debug bool) *zap.AtomicLevel {
	zcfg := zap.NewProductionConfig()

	// zcfg.EncoderConfig.EncodeTime = zapcore.RFC3339TimeEncoder
	zcfg.EncoderConfig.EncodeTime = zapcore.TimeEncoder(func(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
		enc.AppendString(t.UTC().Format("2006-01-02T15:04:05.000Z"))
	})

	var err error
	logger, err = zcfg.Build()
	if err != nil {
		panic(fmt.Sprintf("initLooger error %s", err))
	}

	if debug {
		zcfg.Level.SetLevel(zap.DebugLevel)
	}

	zap.ReplaceGlobals(logger)
	return &zcfg.Level
}
