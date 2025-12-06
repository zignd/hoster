# Hoster

A lightweight Go utility that automatically manages `/etc/hosts` entries for Docker containers. It monitors Docker events and dynamically updates the hosts file with container network information, making it easy to access containers by hostname.

## Features

- 🔄 **Automatic Updates**: Monitors Docker container start/stop events in real-time
- 🌐 **Network Awareness**: Supports multiple Docker networks and aliases
- 🧹 **Clean Shutdown**: Removes all entries on graceful exit
- ⚡ **Atomic Operations**: Uses atomic file writes to prevent corruption
- 🐳 **Bridge & Custom Networks**: Handles both default bridge and user-defined networks

## How It Works

Docker Hoster watches for container lifecycle events and automatically:
1. Extracts container IP addresses from all connected networks
2. Collects container names, hostnames, and network aliases
3. Updates `/etc/hosts` with mappings in a dedicated section
4. Cleans up entries when containers stop or the service exits

## Installation

### Prerequisites

- Docker running on the system
- Root/sudo access (required to modify `/etc/hosts`)

### Option 1: Download Pre-built Binary (Recommended)

Pre-built binaries are available for Linux, macOS, and Windows on the [releases page](https://github.com/zignd/hoster/releases).

**For Linux/macOS:**

```bash
# Download the appropriate binary for your platform
# For Linux x86_64:
wget https://github.com/zignd/hoster/releases/download/vX.Y.Z/hoster_linux_amd64.tar.gz
tar -xzf hoster_linux_amd64.tar.gz

# For Linux ARM64:
wget https://github.com/zignd/hoster/releases/download/vX.Y.Z/hoster_linux_arm64.tar.gz
tar -xzf hoster_linux_arm64.tar.gz

# For macOS Intel (x86_64):
wget https://github.com/zignd/hoster/releases/download/vX.Y.Z/hoster_darwin_amd64.tar.gz
tar -xzf hoster_darwin_amd64.tar.gz

# For macOS Apple Silicon (ARM64):
wget https://github.com/zignd/hoster/releases/download/vX.Y.Z/hoster_darwin_arm64.tar.gz
tar -xzf hoster_darwin_arm64.tar.gz

# Make it executable
chmod +x hoster

# Optionally, move to a location in your PATH
sudo mv hoster /usr/local/bin/
```

**For Windows:**

Download the `.zip` file for your architecture from the [releases page](https://github.com/zignd/hoster/releases), extract it, and place the executable in your preferred location or add it to your PATH.

**Check the installed version:**

```bash
hoster --version
```

### Option 2: Build from Source

Requires Go 1.16 or higher.

```bash
git clone <repository-url>
cd hoster
go mod download
go build -o hoster main.go
```

## Usage

### Basic Usage

Run with default settings (requires root):

```bash
sudo hoster
```

**Note:** If you haven't moved the binary to a directory in your PATH, use `./hoster` instead when running from the current directory.

### Command Line Options

View all available options:

```bash
hoster --help
```

Check the version:

```bash
hoster --version
```

Available flags:

- `--help` - Show help message with usage examples and exit
- `--version` - Display version information and exit
- `--hosts <path>` - Path to the hosts file (default: `/etc/hosts`)
- `--socket <path>` - Path to the Docker socket (default: `/var/run/docker.sock`)

Example with custom paths:

```bash
sudo hoster --hosts /custom/hosts --socket /var/run/docker.sock
```

### Running as a Service

The utility runs continuously, monitoring Docker events. It's designed to run as a background service or daemon.

### Stopping

Press `Ctrl+C` or send `SIGTERM` to gracefully shut down. The service will automatically clean up all hosts file entries before exiting.

## Configuration

The utility uses the following defaults:

- **Hosts File**: `/etc/hosts`
- **Docker Socket**: `/var/run/docker.sock`

You can override these defaults using command line flags:

```bash
sudo ./hoster --hosts /custom/hosts --socket /custom/docker.sock
```

Alternatively, modify the constants in `main.go` to change the defaults:

```go
const (
    defaultHostsPath = "/etc/hosts"
    defaultSocket    = "/var/run/docker.sock"
)
```

## Hosts File Format

Docker Hoster manages entries in a dedicated section:

```
# Your existing hosts entries
127.0.0.1    localhost

#-----------Docker-Hoster-Domains----------
172.17.0.2    mycontainer   webapp   abc123
172.18.0.3    database   db   postgres
#-----Do-not-add-hosts-after-this-line-----
```

⚠️ **Warning**: Do not manually add entries after the Docker Hoster section marker, as they will be removed.

## Example

Start a container with network aliases:

```bash
docker network create mynetwork
docker run -d --name webapp --network mynetwork --network-alias api nginx
```

Docker Hoster will automatically add entries like:

```
172.18.0.2    webapp   api   abc123def456
```

You can now access the container:

```bash
curl http://webapp
curl http://api
```

## Container Information Collected

For each running container, Docker Hoster extracts:

- Container name (without leading `/`)
- Container hostname
- Network-specific IP addresses
- Network aliases from all connected networks

## Permissions

This utility requires:

- **Read/Write access** to `/etc/hosts`
- **Read access** to Docker socket (`/var/run/docker.sock`)

Run with `sudo` or as root to ensure proper permissions.

## Signal Handling

- **SIGINT** (Ctrl+C): Graceful shutdown with cleanup
- **SIGTERM**: Graceful shutdown with cleanup

On shutdown, all Docker Hoster entries are removed from `/etc/hosts`.

## Architecture

- **Docker Client**: Uses official Docker Go SDK
- **Event Monitoring**: Subscribes to Docker events API
- **Atomic Writes**: Uses auxiliary file + rename for safe updates
- **Context Management**: Proper cancellation and cleanup

## Troubleshooting

### Permission Denied

```
Error: failed to read hosts file: permission denied
```

**Solution**: Run with sudo or as root user.

### Cannot Connect to Docker

```
Error: failed to create docker client
```

**Solution**: Ensure Docker is running and the socket path is correct.

### Entries Not Appearing

Check that:
1. Containers are actually running (`docker ps`)
2. Containers have network aliases or are on custom networks
3. The utility is running with proper permissions

## Development

### Project Structure

```
.
├── main.go       # Main application code
├── go.mod        # Go module dependencies
└── README.md     # This file
```

### Dependencies

- `github.com/docker/docker` - Docker Engine API client

### Building

```bash
go build -o hoster main.go
```

### Testing

Test with a simple container:

```bash
# Terminal 1: Run hoster
sudo ./hoster

# Terminal 2: Start a container
docker run -d --name test nginx

# Check /etc/hosts
grep test /etc/hosts
```

## Contributing

Contributions are welcome! Please feel free to submit issues or pull requests.
