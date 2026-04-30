// Package main provides a GB28181 device simulator for testing
package main

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/rivision/gb28181-sim/internal/device"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var (
	serverIP       string
	serverPort     int
	serverID       string
	domain         string
	agentID        string
	agentPassword  string
	channels       []string // 多通道支持: "channelID:source,channelID2:source2"
	localIP        string
	verbose        bool
	useTCP         bool
	configFile     string
)

// yamlConfig matches config.yaml structure
type yamlConfig struct {
	Server struct {
		IP   string `yaml:"ip"`
		Port int    `yaml:"port"`
		ID   string `yaml:"id"`
	} `yaml:"server"`
	Agent struct {
		ID       string `yaml:"id"`
		Password string `yaml:"password"`
	} `yaml:"agent"`
	Local struct {
		IP string `yaml:"ip"`
	} `yaml:"local"`
	TCP    bool `yaml:"tcp"`
	Global struct {
		Domain    string `yaml:"domain"`
		LocalIP   string `yaml:"local_ip"`
		Verbose   bool   `yaml:"verbose"`
		UseTCP    bool   `yaml:"use_tcp"`
	} `yaml:"global"`
	Channels []struct {
		ID     string `yaml:"id"`
		Name   string `yaml:"name"`
		Source string `yaml:"source"`
	} `yaml:"channels"`
}

func main() {
	rootCmd := &cobra.Command{
		Use:   "gb28181-sim",
		Short: "GB28181 Device Simulator",
		Long: `A GB28181 device simulator that registers to a platform and pushes video streams.

Supports multiple channels with different video sources:
  - test: Test pattern
  - rtsp://user:pass@host/path: RTSP stream
  - file:///path/to/video.h264: H.264 raw file
  - file:///path/to/video.h265: H.265 raw file
  - file:///path/to/video.mp4: MP4 container

Examples:
  # Single channel with test pattern
  gb28181-sim --server-ip 192.168.1.100 --server-id 34020000002000000001 \
    --agent-id 34020000001320000001 --channel "34020000001310000001:test"

  # Multiple channels from config file
  gb28181-sim -c config.yaml
`,
		Run: run,
	}

	rootCmd.Flags().StringVar(&serverIP, "server-ip", "", "GB28181 server IP address (required if no config)")
	rootCmd.Flags().IntVar(&serverPort, "server-port", 5060, "GB28181 server SIP port")
	rootCmd.Flags().StringVar(&serverID, "server-id", "", "GB28181 server ID (required if no config)")
	rootCmd.Flags().StringVar(&domain, "domain", "", "SIP domain (default: first 10 chars of server-id)")
	rootCmd.Flags().StringVar(&agentID, "agent-id", "", "Device/Agent ID (required if no config)")
	rootCmd.Flags().StringVar(&agentPassword, "agent-password", "000000", "Device password")
	rootCmd.Flags().StringArrayVar(&channels, "channel", nil, "Channel config: 'channelID:source' (can specify multiple)")
	rootCmd.Flags().StringVar(&localIP, "local-ip", "", "Local IP (auto-detect if empty)")
	rootCmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose logging")
	rootCmd.Flags().BoolVar(&useTCP, "tcp", false, "Use TCP for SIP signaling (default: UDP)")
	rootCmd.Flags().StringVarP(&configFile, "config", "c", "", "Config file (YAML)")

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(cmd *cobra.Command, args []string) {
	var cfg device.Config
	var channelConfigs []device.ChannelConfig

	// Load from YAML if provided
	if configFile != "" {
		yamlCfg, err := loadYAML(configFile)
		if err != nil {
			logrus.Fatalf("Failed to load config file: %v", err)
		}
		cfg = yamlToDeviceConfig(yamlCfg)
		channelConfigs = cfg.Channels
		logrus.Infof("Loaded config from: %s", configFile)
	} else {
		// Use command-line flags
		if domain == "" {
			if len(serverID) >= 10 {
				domain = serverID[:10]
			} else {
				domain = serverID
			}
		}

		channelConfigs = parseChannels(channels)
		if len(channelConfigs) == 0 {
			logrus.Fatal("At least one --channel is required (or specify --config)")
		}

		cfg = device.Config{
			ServerIP:      serverIP,
			ServerPort:    serverPort,
			ServerID:      serverID,
			Domain:        domain,
			AgentID:       agentID,
			AgentPassword: agentPassword,
			Channels:      channelConfigs,
			LocalIP:       localIP,
			UseTCP:        useTCP,
		}
	}

	if verbose {
		logrus.SetLevel(logrus.DebugLevel)
	} else {
		logrus.SetLevel(logrus.InfoLevel)
	}
	logrus.SetFormatter(&logrus.TextFormatter{
		FullTimestamp:   true,
		TimestampFormat: "15:04:05",
	})

	dev, err := device.New(cfg)
	if err != nil {
		logrus.Fatalf("Failed to create device: %v", err)
	}

	// Handle shutdown signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		logrus.Info("Shutting down...")
		dev.Stop()
	}()

	logrus.Infof("Starting GB28181 simulator: %s -> %s:%d (%d channels)",
		cfg.AgentID, cfg.ServerIP, cfg.ServerPort, len(cfg.Channels))
	for _, ch := range cfg.Channels {
		logrus.Infof("  Channel %s (%s): %s", ch.ID, ch.Name, ch.Source)
	}

	if err := dev.Run(); err != nil {
		logrus.Errorf("Device error: %v", err)
	}
}

// loadYAML reads and parses a YAML config file
func loadYAML(path string) (*yamlConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	var cfg yamlConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse yaml: %w", err)
	}

	return &cfg, nil
}

// yamlToDeviceConfig converts YAML config to device.Config
func yamlToDeviceConfig(yc *yamlConfig) device.Config {
	var channelConfigs []device.ChannelConfig
	for _, ch := range yc.Channels {
		src := ch.Source
		name := ch.Name
		if name == "" {
			// Default to last 4 chars of ID
			id := ch.ID
			if len(id) >= 4 {
				name = id[len(id)-4:]
			} else {
				name = id
			}
		}
		channelConfigs = append(channelConfigs, device.ChannelConfig{
			ID:     ch.ID,
			Name:   name,
			Source: src,
		})
	}

	if len(channelConfigs) == 0 {
		logrus.Fatal("Config must have at least one channel")
	}

	agentID := yc.Agent.ID
	if agentID == "" {
		logrus.Fatal("Config must specify agent.id")
	}

	serverIP := yc.Server.IP
	if serverIP == "" {
		logrus.Fatal("Config must specify server.ip")
	}

	serverID := yc.Server.ID
	if serverID == "" {
		logrus.Fatal("Config must specify server.id")
	}

	domain := yc.Global.Domain
	if domain == "" {
		if len(serverID) >= 10 {
			domain = serverID[:10]
		} else {
			domain = serverID
		}
	}

	serverPort := yc.Server.Port
	if serverPort == 0 {
		serverPort = 5060
	}

	cfg := device.Config{
		ServerIP:      serverIP,
		ServerPort:    serverPort,
		ServerID:      serverID,
		Domain:        domain,
		AgentID:       agentID,
		AgentPassword: yc.Agent.Password,
		Channels:      channelConfigs,
		LocalIP:       yc.Global.LocalIP,
		UseTCP:        yc.Global.UseTCP || yc.TCP,
	}

	if yc.Global.Verbose {
		verbose = true
	}

	return cfg
}

func parseChannels(channels []string) []device.ChannelConfig {
	var configs []device.ChannelConfig
	for _, ch := range channels {
		parts := strings.SplitN(ch, ":", 2)
		name := ""
		id := parts[0]
		src := "test"
		if len(parts) == 2 {
			src = parts[1]
		}
		// id may contain : for name, e.g. 34020000001310000001:Entrance
		if strings.Contains(id, ":") {
			idParts := strings.SplitN(id, ":", 2)
			id = idParts[0]
			name = idParts[1]
		}
		if name == "" {
			if len(id) >= 4 {
				name = id[len(id)-4:]
			} else {
				name = id
			}
		}
		configs = append(configs, device.ChannelConfig{
			ID:     id,
			Name:   name,
			Source: src,
		})
	}
	return configs
}
