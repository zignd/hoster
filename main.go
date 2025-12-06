package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/client"
)

var Version = "dev" // Will be overridden at build time

const (
	enclosingPattern = "#-----------Docker-Hoster-Domains----------\n"
	defaultHostsPath = "/etc/hosts"
	defaultSocket    = "/var/run/docker.sock"
)

type ContainerAddress struct {
	IP      string
	Name    string
	Domains []string
}

type Hoster struct {
	client    *client.Client
	hostsPath string
	hosts     map[string][]ContainerAddress
}

func NewHoster(hostsPath, dockerSocket string) (*Hoster, error) {
	cli, err := client.NewClientWithOpts(
		client.WithHost("unix://"+dockerSocket),
		client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create docker client: %w", err)
	}

	return &Hoster{
		client:    cli,
		hostsPath: hostsPath,
		hosts:     make(map[string][]ContainerAddress),
	}, nil
}

func (h *Hoster) getContainerData(ctx context.Context, containerID string) ([]ContainerAddress, error) {
	info, err := h.client.ContainerInspect(ctx, containerID)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect container: %w", err)
	}

	containerHostname := info.Config.Hostname
	containerName := strings.TrimPrefix(info.Name, "/")
	containerIP := info.NetworkSettings.IPAddress

	var result []ContainerAddress

	// Extract IPs and aliases from all networks
	for _, network := range info.NetworkSettings.Networks {
		if network.Aliases == nil || len(network.Aliases) == 0 {
			continue
		}

		// Create a set to avoid duplicates
		domainsSet := make(map[string]bool)
		for _, alias := range network.Aliases {
			domainsSet[alias] = true
		}
		domainsSet[containerName] = true
		domainsSet[containerHostname] = true

		// Convert set to slice
		domains := make([]string, 0, len(domainsSet))
		for domain := range domainsSet {
			domains = append(domains, domain)
		}

		result = append(result, ContainerAddress{
			IP:      network.IPAddress,
			Name:    containerName,
			Domains: domains,
		})
	}

	// Add default bridge network IP if available
	if containerIP != "" {
		result = append(result, ContainerAddress{
			IP:      containerIP,
			Name:    containerName,
			Domains: []string{containerName, containerHostname},
		})
	}

	return result, nil
}

func (h *Hoster) updateHostsFile() error {
	if len(h.hosts) == 0 {
		fmt.Println("Removing all hosts before exit...")
	} else {
		fmt.Println("Updating hosts file with:")
	}

	for id, addresses := range h.hosts {
		_ = id // containerID for debugging if needed
		for _, addr := range addresses {
			fmt.Printf("ip: %s domains: %v\n", addr.IP, addr.Domains)
		}
	}

	// Read all lines from the original file
	data, err := os.ReadFile(h.hostsPath)
	if err != nil {
		return fmt.Errorf("failed to read hosts file: %w", err)
	}

	lines := strings.Split(string(data), "\n")

	// Remove all lines after the known pattern
	for i, line := range lines {
		if line+"\n" == enclosingPattern {
			lines = lines[:i]
			break
		}
	}

	// Remove trailing empty lines
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}

	// Append all domain lines
	if len(h.hosts) > 0 {
		lines = append(lines, "", strings.TrimSuffix(enclosingPattern, "\n"))

		for _, addresses := range h.hosts {
			for _, addr := range addresses {
				domainsStr := strings.Join(addr.Domains, "   ")
				lines = append(lines, fmt.Sprintf("%s    %s", addr.IP, domainsStr))
			}
		}

		lines = append(lines, "#-----Do-not-add-hosts-after-this-line-----", "")
	}

	// Write to auxiliary file
	auxFilePath := h.hostsPath + ".aux"
	content := strings.Join(lines, "\n")
	if err := os.WriteFile(auxFilePath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write aux file: %w", err)
	}

	// Atomic replace using rename
	if err := os.Rename(auxFilePath, h.hostsPath); err != nil {
		return fmt.Errorf("failed to rename aux file: %w", err)
	}

	return nil
}

func (h *Hoster) handleShutdown() {
	h.hosts = make(map[string][]ContainerAddress)
	if err := h.updateHostsFile(); err != nil {
		fmt.Fprintf(os.Stderr, "Error cleaning up hosts file: %v\n", err)
	}
}

func (h *Hoster) Run(ctx context.Context) error {
	// Get running containers
	containers, err := h.client.ContainerList(ctx, container.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list containers: %w", err)
	}

	for _, c := range containers {
		containerData, err := h.getContainerData(ctx, c.ID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error getting container data for %s: %v\n", c.ID, err)
			continue
		}
		h.hosts[c.ID] = containerData
	}

	if err := h.updateHostsFile(); err != nil {
		return fmt.Errorf("failed to update hosts file: %w", err)
	}

	// Listen for Docker events
	eventsChan, errChan := h.client.Events(ctx, types.EventsOptions{})

	for {
		select {
		case event := <-eventsChan:
			if event.Type != events.ContainerEventType {
				continue
			}

			switch event.Action {
			case "start":
				containerData, err := h.getContainerData(ctx, event.Actor.ID)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error getting container data: %v\n", err)
					continue
				}
				h.hosts[event.Actor.ID] = containerData
				if err := h.updateHostsFile(); err != nil {
					fmt.Fprintf(os.Stderr, "Error updating hosts file: %v\n", err)
				}

			case "stop", "die", "destroy":
				if _, exists := h.hosts[event.Actor.ID]; exists {
					delete(h.hosts, event.Actor.ID)
					if err := h.updateHostsFile(); err != nil {
						fmt.Fprintf(os.Stderr, "Error updating hosts file: %v\n", err)
					}
				}
			}

		case err := <-errChan:
			if err != nil && err != io.EOF {
				return fmt.Errorf("docker events error: %w", err)
			}
			return nil

		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func main() {
	// Define command line flags
	hostsPath := flag.String("hosts", defaultHostsPath, "Path to the hosts file")
	dockerSocket := flag.String("socket", defaultSocket, "Path to the Docker socket")
	showHelp := flag.Bool("help", false, "Show help message")
	showVersion := flag.Bool("version", false, "Display version and exit")

	flag.Parse()

	if *showVersion {
		fmt.Println("hoster version", Version)
		os.Exit(0)
	}

	if *showHelp {
		fmt.Println("Docker Hoster - Automatically manage /etc/hosts entries for Docker containers")
		fmt.Println()
		fmt.Println("Usage: hoster [options]")
		fmt.Println()
		fmt.Println("Options:")
		flag.PrintDefaults()
		fmt.Println()
		fmt.Println("Example:")
		fmt.Println("  sudo hoster --hosts /etc/hosts --socket /var/run/docker.sock")
		os.Exit(0)
	}

	hoster, err := NewHoster(*hostsPath, *dockerSocket)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create hoster: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		fmt.Println("\nReceived shutdown signal...")
		hoster.handleShutdown()
		cancel()
	}()

	if err := hoster.Run(ctx); err != nil && err != context.Canceled {
		fmt.Fprintf(os.Stderr, "Error running hoster: %v\n", err)
		os.Exit(1)
	}
}
