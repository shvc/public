package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	version     = "0.0.0"
	clientID    = ""
	logger      *zap.Logger
	debug       bool
	port        uint32 = 20019
	serverAddr1        = "47.100.31.117:20019"
	serverAddr2        = "47.103.138.1:20019"
	dialTimeout uint   = 5
)

func main() {
	var rootCmd = &cobra.Command{
		Use:     "qc",
		Short:   "qc",
		Long:    "quic tool",
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
	rootCmd.PersistentFlags().Uint32VarP(&port, "port", "p", port, "serve(listen) port")
	rootCmd.PersistentFlags().UintVar(&dialTimeout, "dial-timeout", dialTimeout, "client dial timeout")
	rootCmd.PersistentFlags().StringVar(&serverAddr1, "s1", serverAddr1, "server address1")
	rootCmd.PersistentFlags().StringVar(&serverAddr2, "s2", serverAddr2, "server address2")

	var rootDir = "."
	var certPath = "."
	serverCmd := &cobra.Command{
		Use:     "start",
		Aliases: []string{"s"},
		Short:   "qserver start",
		Long: `qc server:
* start tcp server
qc server
`,
		Args: cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			return QuicServer(rootDir, certPath, port)
		},
	}
	serverCmd.Flags().StringVar(&rootDir, "root", rootDir, "www root dir")
	serverCmd.Flags().StringVar(&certPath, "cert", certPath, "cert path")
	rootCmd.AddCommand(serverCmd)

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
