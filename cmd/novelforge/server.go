package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/voocel/ainovel-cli/internal/compat"
	"github.com/voocel/ainovel-cli/internal/server"
)

func runServerCommand(argv []string) int {
	flags := flag.NewFlagSet("novelforge server", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	host := flags.String("host", "127.0.0.1", "listen host")
	port := flags.Int("port", 48090, "listen port")
	workspace := flags.String("workspace", ".", "directory containing NovelForge or ainovel projects")
	configPath := flags.String("config", "", "explicit server model configuration file")
	flags.Usage = func() {
		fmt.Fprintln(flags.Output(), "Usage: novelforge server [--host HOST] [--port PORT] [--workspace DIR] [--config FILE]")
		flags.PrintDefaults()
	}
	if err := flags.Parse(argv); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintf(os.Stderr, "server: unexpected arguments: %s\n", strings.Join(flags.Args(), " "))
		flags.Usage()
		return 2
	}

	if *configPath != "" {
		if err := compat.SetExplicitConfigPath(*configPath); err != nil {
			fmt.Fprintln(os.Stderr, "server: invalid explicit configuration path")
			return 2
		}
	}
	app, err := server.New(server.Config{
		Host:                 *host,
		Port:                 *port,
		Workspace:            *workspace,
		Version:              versionInfo().Version,
		QualityConfigEnabled: true,
		QualityConfigPath:    compat.ExplicitConfigPath(),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "server: %v\n", err)
		return 1
	}
	defer func() {
		if err := app.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "server close: %v\n", err)
		}
	}()
	if !isLoopbackHost(*host) {
		fmt.Fprintln(os.Stderr, "警告：NovelForge 正在监听非回环地址。请使用防火墙或可信反向代理限制访问，且不要把未受保护的本地工作区暴露到公网。")
	}
	fmt.Fprintf(os.Stdout, "NovelForge Web: http://%s\n", app.Address())
	fmt.Fprintf(os.Stdout, "Workspace: %s\n", *workspace)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := app.ListenAndServe(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "server: %v\n", err)
		return 1
	}
	return 0
}

func isLoopbackHost(host string) bool {
	host = strings.TrimSpace(host)
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
