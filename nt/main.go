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
	version     = "0.0.0"
	logger      *zap.Logger
	debug       bool
	port        uint = 20019
	serverAddr1      = "47.100.31.117:20019"
	serverAddr2      = "47.103.138.1:20019"
	dialTimeout uint = 5
)

func main() {
	var rootCmd = &cobra.Command{
		Use:   "nt",
		Short: "nt",
		Long: "nat traversal tool",
		Version: version,
		Hidden:  true,
		PersistentPreRunE: func(*cobra.Command, []string) error {
			var err error
			initLogger(debug)
			return err
		},
	}
	rootCmd.PersistentFlags().BoolVarP(&debug, "debug", "", false, "show debug log")
	rootCmd.PersistentFlags().UintVarP(&port, "port", "p", port, "serve(listen) port")
	rootCmd.PersistentFlags().UintVar(&dialTimeout, "dial-timeout", dialTimeout, "client dial timeout")
	rootCmd.PersistentFlags().StringVar(&serverAddr1, "s1", serverAddr1, "server address1")
	rootCmd.PersistentFlags().StringVar(&serverAddr2, "s2", serverAddr2, "server address2")

	tcpServerCmd := &cobra.Command{
		Use:     "tcp-server",
		Aliases: []string{"ts"},
		Short:   "tcp server",
		Long: `tcp server:
* start tcp server
nt ts
`,
		Args: cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			return TCPServer(port)
		},
	}
	rootCmd.AddCommand(tcpServerCmd)

	tcpClientCmd := &cobra.Command{
		Use:     "tcp-client",
		Aliases: []string{"tc"},
		Short:   "tcp client",
		Long: `tcp client:
* run tcp client
nt tc
`,
		Args: cobra.MinimumNArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
			defer stop()

			return TCPClient(ctx, port, serverAddr1, serverAddr2, dialTimeout)
		},
	}
	rootCmd.AddCommand(tcpClientCmd)

	udpServerCmd := &cobra.Command{
		Use:     "udp-server",
		Aliases: []string{"us"},
		Short:   "udp server",
		Long: `udp server:
* start udp server
nt us
`,
		Args: cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			us := &UDPServer{}
			return us.UDPServer(ctx, port)
		},
	}
	rootCmd.AddCommand(udpServerCmd)

	var pingPeerInterval uint = 2000
	var pingServerInterval uint = 2000
	var pongPeerDelay uint = 2000
	udpClientCmd := &cobra.Command{
		Use:     "udp-client",
		Aliases: []string{"uc"},
		Short:   "udp client",
		Long: `udp client:
* run udp client
nt uc
`,
		Args: cobra.MinimumNArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			uc := UDPClient{}
			return uc.UDPClient(ctx, port, serverAddr1, serverAddr2, dialTimeout, pingPeerInterval, pingServerInterval, pongPeerDelay)
		},
	}
	udpClientCmd.Flags().UintVar(&pingPeerInterval, "ping-peer-interval", pingPeerInterval, "ping peer interval in millisecond")
	udpClientCmd.Flags().UintVar(&pingServerInterval, "ping-server-interval", pingServerInterval, "ping server interval in millisecond")
	udpClientCmd.Flags().UintVar(&pongPeerDelay, "pong-peer-delay", pongPeerDelay, "pong(response) peer delay in millisecond")
	rootCmd.AddCommand(udpClientCmd)

	udpSendCmd := &cobra.Command{
		Use:   "udp-send <data> <server-addr> [client-addr]",
		Short: "udp client send data",
		Long: `udp client send data:
* udp client send data
nt udp-send test-data-001 172.16.1.1:9234
`,
		Args: cobra.RangeArgs(2, 3),
		RunE: func(cmd *cobra.Command, args []string) error {
			laddr := ""
			if len(args) == 3 {
				laddr = args[2]
			}
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
			defer stop()
			return UDPSend(ctx, laddr, args[1], args[0], 3)
		},
	}
	rootCmd.AddCommand(udpSendCmd)

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
