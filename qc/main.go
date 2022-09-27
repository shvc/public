package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"os/signal"

	"github.com/lucas-clemente/quic-go"
	"github.com/lucas-clemente/quic-go/http3"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	version        = "0.0.0"
	logger         *zap.Logger
	serverAddress1      = "47.100.31.117:20019"
	serverAddress2      = "47.103.138.1:20019"
	dialTimeout    uint = 5
)

func main() {
	qc := QuicClient{
		networkType: "udp4",
	}
	var rootCmd = &cobra.Command{
		Use:     "qc",
		Short:   "qc",
		Long:    "quic client tool",
		Version: version,
		Hidden:  true,
		PersistentPreRunE: func(*cobra.Command, []string) error {
			var err error
			initLogger(qc.debug)
			qc.clientID, _ = os.Hostname()

			pool, err := x509.SystemCertPool()
			if err != nil {
				panic(fmt.Sprintf("x509 cert error %s", err))
			}

			qc.roundTripper = &http3.RoundTripper{
				TLSClientConfig: &tls.Config{
					RootCAs:            pool,
					InsecureSkipVerify: !qc.secure,
				},
				QuicConfig: &quic.Config{},
			}
			return err
		},
	}
	rootCmd.PersistentFlags().BoolVarP(&qc.debug, "debug", "", false, "show debug log")
	rootCmd.PersistentFlags().BoolVarP(&qc.nat, "nat", "", false, "nat traversal")
	rootCmd.PersistentFlags().IntVarP(&qc.port, "port", "p", 0, "local port")
	rootCmd.PersistentFlags().UintVar(&qc.dialTimeout, "dial-timeout", dialTimeout, "client dial timeout")
	rootCmd.PersistentFlags().StringVar(&qc.serverAddress1, "s1", serverAddress1, "server address1")
	rootCmd.PersistentFlags().StringVar(&qc.serverAddress2, "s2", serverAddress2, "server address2")

	qcGetCmd := &cobra.Command{
		Use:   "get",
		Short: "get file",
		Long: `get file:
* get to stdout
qc get https://127.0.0.1:6121
* get a file to localfile
qc get https://192.168.1.6:6121/file localfile
`,
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
			defer stop()
			filename := ""
			if len(args) == 2 {
				filename = args[1]
			}
			return qc.get(ctx, args[0], dialTimeout, filename)
		},
	}
	rootCmd.AddCommand(qcGetCmd)

	qcPutCmd := &cobra.Command{
		Use:   "put",
		Short: "put file",
		Long: `put file:
* put a file to localfile
qc put https://192.168.1.6:6121/file localfile
`,
		Args: cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
			defer stop()
			return qc.put(ctx, args[0], dialTimeout)
		},
	}
	rootCmd.AddCommand(qcPutCmd)

	qcPostCmd := &cobra.Command{
		Use:   "post",
		Short: "post file",
		Long: `post file:
* post a file to localfile
qc post https://192.168.1.6:6121/file localfile
`,
		Args: cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
			defer stop()
			return qc.post(ctx, args[0], dialTimeout)
		},
	}
	rootCmd.AddCommand(qcPostCmd)

	qcDeleteCmd := &cobra.Command{
		Use:   "del",
		Short: "del file",
		Long: `delete file:
* delete a file
qc del https://192.168.1.6:6121/file localfile
`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
			defer stop()
			return qc.delete(ctx, args[0], dialTimeout)
		},
	}
	rootCmd.AddCommand(qcDeleteCmd)

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
