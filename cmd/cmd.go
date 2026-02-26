package cmd

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"sync"

	"github.com/spf13/cobra"

	"github.com/leaktk/leaktk/pkg/config"
	"github.com/leaktk/leaktk/pkg/fs"
	"github.com/leaktk/leaktk/pkg/hooks"
	"github.com/leaktk/leaktk/pkg/id"
	"github.com/leaktk/leaktk/pkg/logger"
	"github.com/leaktk/leaktk/pkg/proto"
	"github.com/leaktk/leaktk/pkg/scanner"
	"github.com/leaktk/leaktk/pkg/version"
)

var cfg *config.Config

func initLogger() {
	if err := logger.SetLoggerLevel("INFO"); err != nil {
		logger.Warning("could not set log level to INFO")
	}
}

func runHelp(cmd *cobra.Command, args []string) {
	if err := cmd.Help(); err != nil {
		logger.Fatal("%v", err)
	}
}

func runLogin(cmd *cobra.Command, args []string) {
	logger.Info("logging in: pattern_server=%q", cfg.Scanner.Patterns.Server.URL)

	fmt.Printf("Enter %s auth token: ", cfg.Scanner.Patterns.Server.URL)

	var authToken string
	if _, err := fmt.Scanln(&authToken); err != nil {
		logger.Fatal("could not login: %v", err)
	}

	if err := config.SavePatternServerAuthToken(authToken); err != nil {
		logger.Fatal("could not login: %v", err)
	}

	logger.Info("token saved")
}

func runLogout(cmd *cobra.Command, args []string) {
	logger.Info("logging out: pattern_server=%q", cfg.Scanner.Patterns.Server.URL)

	if err := config.RemovePatternServerAuthToken(); err != nil {
		logger.Fatal("could not logout: %v", err)
	}

	logger.Info("token removed")
}

func loginCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "login",
		Short: "Log into a pattern server",
		Run:   runLogin,
	}
}

func logoutCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Log out of a pattern server",
		Run:   runLogout,
	}
}

func runScan(cmd *cobra.Command, args []string) {
	leakExitCode, err := cmd.Flags().GetInt("leak-exit-code")
	if err != nil {
		logger.Fatal("invalid leak-exit-code: %v", err)
	}

	gitleaksConfig, err := cmd.Flags().GetString("gitleaks-config")
	if err != nil {
		logger.Fatal("invalid gitleaks-config: error=%q", err.Error())
	}

	// Providing a gitleaks-config via command line arguments takes
	// precedence over gitleaks config set in the leaktk config file
	if len(gitleaksConfig) != 0 {
		cfg.Scanner.Patterns.Gitleaks.ConfigPath = gitleaksConfig
		logger.Debug("using provided gitleaks config: path=%s", gitleaksConfig)

		// Providing a config automatically disables pattern autofetch
		cfg.Scanner.Patterns.Autofetch = false
		logger.Debug("disabling pattern autofetch with custom gitleaks config")

		// and disables pattern expiredafter checks
		cfg.Scanner.Patterns.ExpiredAfter = 0
		cfg.Scanner.Patterns.RefreshAfter = 0
		logger.Debug("disabling pattern expiredafter/refreshafter with custom gitleaks config")
	}

	request, err := scanCommandToRequest(cmd, args)
	if err != nil {
		logger.Fatal("could not generate scan request: %v", err)
	}

	formatter, err := NewFormatter(cfg.Formatter)
	if err != nil {
		logger.Fatal("%v", err)
	}

	var wg sync.WaitGroup
	leaktkScanner := scanner.NewScanner(cfg)
	leaksFound := false

	// Prints the output of the scanner as they come
	go leaktkScanner.Recv(func(response *proto.Response) {
		if !leaksFound && len(response.Results) > 0 {
			leaksFound = true
		}
		fmt.Println(formatter.Format(response))
		if response.Error != nil {
			logger.Fatal("response contains error: %w", response.Error)
		}
		wg.Done()
	})

	wg.Add(1)
	leaktkScanner.Send(request)
	wg.Wait()

	if leaksFound {
		os.Exit(leakExitCode)
	}
}

func scanCommandToRequest(cmd *cobra.Command, args []string) (*proto.Request, error) {
	flags := cmd.Flags()

	id, err := flags.GetString("id")
	if err != nil || len(id) == 0 {
		return nil, errors.New("missing required field: field=\"id\"")
	}

	kind, err := flags.GetString("kind")
	if err != nil || len(kind) == 0 {
		return nil, errors.New("missing required field: field=\"kind\"")
	}

	if len(args) == 0 || len(args[0]) == 0 {
		return nil, errors.New("missing required field: field=\"resource\"")
	}

	requestResource := args[0]
	if requestResource[0] == '@' {
		if fs.FileExists(requestResource[1:]) {
			data, err := os.ReadFile(requestResource[1:])
			if err != nil {
				return nil, fmt.Errorf("could not read resource: path=%q error=%q", requestResource[1:], err)
			}

			requestResource = string(data)
		} else {
			return nil, fmt.Errorf("resource path does not exist: path=%q", requestResource[1:])
		}
	}

	rawOpts, err := flags.GetString("options")
	if err != nil {
		return nil, fmt.Errorf("there was an issue with the options flag: error=%q", err)
	}

	// Convert kind string to enum
	requestKind, isValidKind := proto.GetRequestKind(kind)
	if !isValidKind {
		return nil, fmt.Errorf("unsupported request kind: kind=%q", kind)
	}

	// Parse options once directly into proto.Opts struct
	var opts proto.Opts
	if rawOpts != "{}" && len(rawOpts) > 0 {
		if err := json.Unmarshal([]byte(rawOpts), &opts); err != nil {
			return nil, fmt.Errorf("could not parse options: error=%q", err)
		}
	}

	// automatically set the is local flag
	if requestKind == proto.GitRepoRequestKind && !opts.Local {
		opts.Local = fs.PathExists(requestResource)
	}

	// Create the request
	request := &proto.Request{
		ID:       id,
		Kind:     requestKind,
		Resource: requestResource,
		Opts:     opts,
	}

	return request, nil
}

func runHook(cmd *cobra.Command, args []string) {
	hookName := args[0]

	if !slices.Contains(cmd.ValidArgs, hookName) {
		logger.Fatal("invalid hookname: hookname=%q", hookName)
	}

	statusCode, err := hooks.Run(cfg, hookName, args[1:])
	if err != nil {
		logger.Fatal("error running hook: %v hookname=%q", err, hookName)
	}

	os.Exit(statusCode)
}

func hookCommand() *cobra.Command {
	return &cobra.Command{
		Use:       "hook [flags] <hookname> [hookargs]...",
		Short:     "Hook leaktk into existng workflows",
		Args:      cobra.MinimumNArgs(1),
		ValidArgs: hooks.Names,
		Run:       runHook,
	}
}

func scanCommand() *cobra.Command {
	scanCommand := &cobra.Command{
		Use:                   "scan [flags] <resource>",
		DisableFlagsInUseLine: true,
		Short:                 "Perform ad-hoc scans",
		Args:                  cobra.MaximumNArgs(1),
		Run:                   runScan,
	}

	flags := scanCommand.Flags()
	flags.String("id", id.ID(), "Set the ID request ID that will be displayed in the response and logs")
	flags.StringP("kind", "k", "GitRepo", "Specify the kind of resource being scanned")
	flags.StringP("options", "o", "{}", "Provide scan specific options formatted as JSON")
	flags.Int("leak-exit-code", 0, "Exit with this code when leaks are detected (default 0)")
	flags.String("gitleaks-config", "", "Load a custom gitleaks config")

	return scanCommand
}

func readLine(reader *bufio.Reader) ([]byte, error) {
	var buf bytes.Buffer

	for {
		line, isPrefix, err := reader.ReadLine()
		buf.Write(line)

		if err != nil || !isPrefix {
			return buf.Bytes(), err
		}
	}
}

func runListen(cmd *cobra.Command, args []string) {
	var wg sync.WaitGroup

	stdinReader := bufio.NewReader(os.Stdin)
	leaktkScanner := scanner.NewScanner(cfg)

	// Prints the output of the scanner as they come
	go leaktkScanner.Recv(func(response *proto.Response) {
		fmt.Println(formatJSON(response))
		wg.Done()
	})

	// Listen for requests
	for {
		line, err := readLine(stdinReader)

		if err != nil {
			if err == io.EOF {
				break
			}

			logger.Error("error reading from stdin: error=%q", err)

			continue
		}

		var request proto.Request
		err = json.Unmarshal(line, &request)

		if err != nil {
			logger.Error("could not unmarshal request: error=%q", err)

			continue
		}

		if len(request.Resource) == 0 {
			logger.Error("no resource provided: request_id=%q", request.ID)

			continue
		}

		wg.Add(1)
		leaktkScanner.Send(&request)
	}

	// Wait for all of the scans to complete and responses to be sent
	wg.Wait()
}

func listenCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "listen",
		Short: "Listen for scan requests on stdin",
		Run:   runListen,
	}
}

func runVersion(cmd *cobra.Command, args []string) {
	version.PrintVersion()
}

func versionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Display the version",
		Run:   runVersion,
	}
}

func configure(cmd *cobra.Command, args []string) error {
	switch cmd.Use {
	case "listen":
		if err := logger.SetLoggerFormat(logger.JSON); err != nil {
			return err
		}
	default:
		if err := logger.SetLoggerFormat(logger.HUMAN); err != nil {
			return err
		}
	}
	path, err := cmd.Flags().GetString("config")

	if err == nil {
		// If path == "", this will look other places
		cfg, err = config.LocateAndLoadConfig(path)

		if err == nil {
			err = logger.SetLoggerLevel(cfg.Logger.Level)
		}
		if err != nil {
			return err
		}
	}

	// If a format is specified on the command line update the application config.
	format, err := cmd.Flags().GetString("format")
	if err == nil && format != "" {
		cfg.Formatter = config.Formatter{Format: format}
	}

	// Check if the OutputFormat is valid
	_, err = getOutputFormat(cfg.Formatter.Format)
	if err != nil {
		logger.Fatal("%v", err)
	}

	return err
}

func rootCommand() *cobra.Command {
	cobra.OnInitialize(initLogger)

	rootCommand := &cobra.Command{
		Use:               "leaktk",
		Short:             "LeakTK: The Leak ToolKit",
		Run:               runHelp,
		PersistentPreRunE: configure,
	}

	flags := rootCommand.PersistentFlags()
	flags.StringP("config", "c", "", "Load a custom leaktk config")
	flags.StringP("format", "f", "", "Change the output format [json, human, csv, toml, yaml] (default \"json\")")

	rootCommand.AddCommand(scanCommand())
	rootCommand.AddCommand(installCommand())
	rootCommand.AddCommand(loginCommand())
	rootCommand.AddCommand(logoutCommand())
	rootCommand.AddCommand(hookCommand())
	rootCommand.AddCommand(listenCommand())
	rootCommand.AddCommand(versionCommand())

	return rootCommand
}

// Execute the command and parse the args
func Execute() {
	if err := rootCommand().Execute(); err != nil {
		if strings.Contains(err.Error(), "unknown flag") {
			os.Exit(config.ExitCodeBlockingError)
		}
		logger.Fatal("%v", err)
	}
}
