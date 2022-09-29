package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	version            = "0.0.0"
	clientID           = ""
	logger             *zap.Logger
	debug              bool
	port               uint   = 20017
	serverAddr1               = "47.100.31.117:20019"
	serverAddr2               = "47.103.138.1:20019"
	dialTimeout        uint32 = 5
	pingServerInterval uint32 = 10
	pingPeerInterval   uint32 = 100
	pingPeerNum        uint32 = 100
)

func main() {
	var rootCmd = &cobra.Command{
		Use:     "nt",
		Short:   "nt",
		Long:    "nat traversal tool",
		Version: version,
		Hidden:  true,
		PersistentPreRunE: func(*cobra.Command, []string) error {
			var err error
			initLogger(debug)
			clientID, _ = os.Hostname()
			return err
		},
	}
	rootCmd.PersistentFlags().BoolVarP(&debug, "debug", "", false, "show debug log")
	rootCmd.PersistentFlags().UintVarP(&port, "port", "p", port, "serve(listen) port")
	rootCmd.PersistentFlags().Uint32Var(&dialTimeout, "dial-timeout", dialTimeout, "client dial timeout")
	rootCmd.PersistentFlags().StringVar(&serverAddr1, "s1", serverAddr1, "server address1")
	rootCmd.PersistentFlags().StringVar(&serverAddr2, "s2", serverAddr2, "server address2")
	rootCmd.Flags().Uint32Var(&pingServerInterval, "ping-server-interval", pingServerInterval, "ping server interval in second")
	rootCmd.Flags().Uint32Var(&pingPeerInterval, "ping-peer-interval", pingPeerInterval, "ping peer interval in millsecond")
	rootCmd.Flags().Uint32Var(&pingPeerNum, "ping-peer-num", pingPeerNum, "ping peer total num")

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
			us := &PublicServer{}
			return us.UDPServer(ctx, port)
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
			return ts.TCPClient(ctx, port, serverAddr1, serverAddr2, dialTimeout)
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
		Args: cobra.RangeArgs(2, 3),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			u := NewUdpPeer(clientID, serverAddr1, serverAddr2)
			return u.UDPPeerServer(ctx, port, dialTimeout, pingServerInterval, pingPeerInterval, pingPeerNum)
		},
	}
	rootCmd.AddCommand(udpPeerServerCmd)

	var helloInterval uint32 = 10
	udpPeerClientCmd := &cobra.Command{
		Use:     "udp-peer-client",
		Aliases: []string{"uc"},
		Short:   "udp peer client",
		Long: `udp peer client:
* run udp client
ntn uc
`,
		Args: cobra.MinimumNArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			u := NewUdpPeer(clientID, serverAddr1, serverAddr2)
			return u.PeerClientUDP(ctx, port, dialTimeout, pingServerInterval, pingPeerInterval, pingPeerNum, helloInterval)
		},
	}
	udpPeerClientCmd.Flags().Uint32Var(&helloInterval, "hello-interval", helloInterval, "say hello interval in second")
	rootCmd.AddCommand(udpPeerClientCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

// init logger
func initLogger(debug bool) *zap.AtomicLevel {
	zcfg := zap.NewProductionConfig()

	zcfg.EncoderConfig.EncodeTime = zapcore.RFC3339TimeEncoder

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
