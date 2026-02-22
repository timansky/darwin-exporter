# darwin-exporter

macOS-specific Prometheus exporter. Supplements [node_exporter](https://github.com/prometheus/node_exporter) with metrics that are only available on macOS.

Default listen address: **127.0.0.1:10102**

Up-to-date CLI examples and scenarios: `USAGE.md`.

## Metrics

### WiFi (`darwin_wifi_*`)

| Metric | Type | Description |
| ------ | ---- | ----------- |
| `darwin_wifi_rssi_dbm` | Gauge | Signal strength (dBm) |
| `darwin_wifi_noise_dbm` | Gauge | Noise level (dBm) |
| `darwin_wifi_snr_db` | Gauge | Signal-to-noise ratio (dBm) |
| `darwin_wifi_tx_rate_mbps` | Gauge | Transmit rate (Mbps) |
| `darwin_wifi_channel` | Gauge | WiFi channel number |
| `darwin_wifi_connected` | Gauge | Connection status (0/1) |
| `darwin_wifi_info` | Gauge | Info labels: interface, ssid, security, band, phymode, country_code |

### Battery (`darwin_battery_*`)

| Metric | Type | Description |
| ------ | ---- | ----------- |
| `darwin_battery_cycle_count` | Gauge | Charge cycle count |
| `darwin_battery_health_percent` | Gauge | Health (MaxCapacity/DesignCapacity * 100) |
| `darwin_battery_voltage_volts` | Gauge | Voltage (V) |
| `darwin_battery_temperature_celsius` | Gauge | Battery temperature |

Battery level/charging/current/time/power source come from `node_exporter` (`node_power_supply_*`).

### Thermal (`darwin_thermal_*`)

| Metric | Type | Description |
| ------ | ---- | ----------- |
| `darwin_thermal_pressure` | Gauge | Thermal pressure state (label: nominal/fair/serious/critical) |
| `darwin_cpu_temperature_celsius` | Gauge | CPU temperature (best effort via built-in SMC, fallback `powermetrics`) |
| `darwin_gpu_temperature_celsius` | Gauge | GPU temperature (best effort via built-in SMC keys) |
| `darwin_disk_temperature_celsius{device="diskN"}` | Gauge | Disk/NAND temperature (best effort via built-in SMC keys) |

### Advanced WiFi (`darwin_wdutil_*`)

Advanced WiFi metrics from `wdutil` (requires root/sudo privileges).

| Metric | Type | Description |
| ------ | ---- | ----------- |
| `darwin_wdutil_available` | Gauge | 1 if `wdutil` access is available, otherwise 0 |
| `darwin_wdutil_wifi_cca_percent` | Gauge | Channel utilization (CCA, %) |
| `darwin_wdutil_wifi_nss` | Gauge | Number of spatial streams |
| `darwin_wdutil_wifi_guard_interval_ns` | Gauge | Guard interval (ns) |
| `darwin_wdutil_wifi_faults_last_hour` | Gauge | Fault count for the last hour |
| `darwin_wdutil_wifi_recoveries_last_hour` | Gauge | Recovery count for the last hour |
| `darwin_wdutil_wifi_link_tests_last_hour` | Gauge | Link test count for the last hour |
| `darwin_wdutil_wifi_info` | Gauge | Info labels: interface, bssid, ipv4_address, dns_server |

## Requirements

- macOS 12+
- Go 1.26+ (for building)
- Xcode Command Line Tools (`xcode-select --install`) for CGo build (required)
- **root/sudo** for advanced WiFi metrics (`wdutil` collector)
- Optional fallback: `powermetrics` (bundled with macOS) + **root/sudo** for `darwin_cpu_temperature_celsius` when SMC is unavailable

## Installation

### From source

```bash
cd apps/darwin-exporter
make build
make install          # installs to ~/.local/bin/darwin-exporter
make install-service  # installs and starts launchd service (sudo mode)
```

## Usage

### Run exporter

```bash
darwin-exporter run -c ~/.config/darwin-exporter/config.yml
# or simply:
darwin-exporter -c ~/.config/darwin-exporter/config.yml
```

Color output can be configured globally:

```bash
darwin-exporter --color=auto service status
darwin-exporter --color=never service status
```

Without `-c`, darwin-exporter reads `~/.config/darwin-exporter/config.yml`.

### Service installation modes

`darwin-exporter service install` supports two modes:

1. `--type=sudo`: user LaunchAgent + sudoers rule for passwordless `wdutil/ipconfig/powermetrics`
1. `--type=root`: system LaunchDaemon running as root

Service restart policy in launchd: restart on failure (`KeepAlive.SuccessfulExit=false`).

Temperature note:

- `darwin_cpu_temperature_celsius`, `darwin_gpu_temperature_celsius`, and `darwin_disk_temperature_celsius` are read from built-in SMC keys.
- If SMC CPU keys are unavailable, CPU temperature falls back to `powermetrics` (requires root/sudo).
- `service install --type=sudo` configures sudoers for `wdutil`, `ipconfig`, and `powermetrics`.

Examples:

```bash
# User mode (recommended for desktop session; default)
sudo darwin-exporter service install
sudo darwin-exporter service install --type=sudo

# Root daemon mode
sudo darwin-exporter service install --type=root
```

Optional install flags:

- `--config <path>`: pass config file path into plist
- `--log-dir <path>`: override log directory
- `--bin-path <path>`: explicit binary path

### macOS Security Approval (Required After Install)

If macOS shows: _"Apple could not verify darwin-exporter..."_, approve the **binary** (not the `.plist`).

1. Get binary path from the installed launchd plist:

```bash
plutil -extract ProgramArguments.0 raw ~/Library/LaunchAgents/kz.neko.darwin-exporter.plist
```

1. Trigger the security prompt once:

```bash
/absolute/path/to/darwin-exporter --help
```

1. Approve in `System Settings -> Privacy & Security -> Open Anyway`.

Alternative (CLI):

```bash
xattr -dr com.apple.quarantine /absolute/path/to/darwin-exporter
```

1. Restart service:

```bash
sudo darwin-exporter service restart --type=sudo
```

### Service uninstall

```bash
# Remove user mode service + sudoers
sudo darwin-exporter service uninstall --type=sudo

# Remove root daemon service
sudo darwin-exporter service uninstall --type=root

# Also delete logs/config (when provided)
sudo darwin-exporter service uninstall --type=sudo --purge --config ~/.config/darwin-exporter/config.yml
```

### Service lifecycle

```bash
sudo darwin-exporter service start --type=sudo
sudo darwin-exporter service status --type=sudo
sudo darwin-exporter service logs --type=sudo --lines=100
sudo darwin-exporter service stop --type=sudo
sudo darwin-exporter service restart --type=sudo
sudo darwin-exporter service enable --type=sudo
sudo darwin-exporter service disable --type=sudo
```

### Shell autocompletion

Generate completion script:

```bash
darwin-exporter completion bash
darwin-exporter completion zsh
```

Install for current user:

```bash
make install-completion-bash
make install-completion-zsh
```

## Configuration

Copy the example config and adjust:

```bash
mkdir -p ~/.config/darwin-exporter
cp config/config.yml.example ~/.config/darwin-exporter/config.yml
```

Override priority:

1. CLI flags
1. Environment variables
1. YAML config file
1. Built-in defaults

Examples:

```bash
# Defaults only
darwin-exporter --config=""

# YAML config
darwin-exporter --config ~/.config/darwin-exporter/config.yml

# ENV overrides YAML
DARWIN_EXPORTER_SERVER_LISTEN_ADDRESS=127.0.0.1:9090 \
  darwin-exporter --config ~/.config/darwin-exporter/config.yml

# CLI overrides ENV
DARWIN_EXPORTER_SERVER_LISTEN_ADDRESS=127.0.0.1:9090 \
  darwin-exporter --config ~/.config/darwin-exporter/config.yml \
  --server.listen-address=127.0.0.1:8080
```

Option mapping:

| YAML | CLI flag | ENV variable | Type | Default |
| ---- | -------- | ------------ | ---- | ------- |
| `server.listen_address` | `--server.listen-address` | `DARWIN_EXPORTER_SERVER_LISTEN_ADDRESS` | string | `127.0.0.1:10102` |
| `server.metrics_path` | `--server.metrics-path` | `DARWIN_EXPORTER_SERVER_METRICS_PATH` | string | `/metrics` |
| `server.health_path` | `--server.health-path` | `DARWIN_EXPORTER_SERVER_HEALTH_PATH` | string | `/health` |
| `server.ready_path` | `--server.ready-path` | `DARWIN_EXPORTER_SERVER_READY_PATH` | string | `/ready` |
| `server.read_timeout` | `--server.read-timeout` | `DARWIN_EXPORTER_SERVER_READ_TIMEOUT` | duration | `30s` |
| `server.write_timeout` | `--server.write-timeout` | `DARWIN_EXPORTER_SERVER_WRITE_TIMEOUT` | duration | `30s` |
| `logging.level` | `--logging.level` | `DARWIN_EXPORTER_LOGGING_LEVEL` | string | `info` |
| `logging.format` | `--logging.format` | `DARWIN_EXPORTER_LOGGING_FORMAT` | string | `logfmt` |
| `color` | `--color` | `DARWIN_EXPORTER_COLOR` | enum | `auto` |
| `collectors.wifi.enabled` | `--collectors.wifi.enabled` | `DARWIN_EXPORTER_COLLECTORS_WIFI_ENABLED` | bool | `true` |
| `collectors.battery.enabled` | `--collectors.battery.enabled` | `DARWIN_EXPORTER_COLLECTORS_BATTERY_ENABLED` | bool | `true` |
| `collectors.thermal.enabled` | `--collectors.thermal.enabled` | `DARWIN_EXPORTER_COLLECTORS_THERMAL_ENABLED` | bool | `true` |
| `collectors.wdutil.enabled` | `--collectors.wdutil.enabled` | `DARWIN_EXPORTER_COLLECTORS_WDUTIL_ENABLED` | bool | `false` |
| `instance.name` | `--instance.name` | `DARWIN_EXPORTER_INSTANCE_NAME` | string | `""` |
| `instance.instance_file` | `--instance.instance-file` | `DARWIN_EXPORTER_INSTANCE_INSTANCE_FILE` | string | `""` |
| `instance.labels` | not supported | `DARWIN_EXPORTER_INSTANCE_LABELS` | JSON map | `{}` |

`DARWIN_EXPORTER_INSTANCE_LABELS` must be JSON, for example:

```bash
DARWIN_EXPORTER_INSTANCE_LABELS='{"env":"prod","region":"us-west"}' darwin-exporter
```

YAML baseline:

```yaml
server:
  listen_address: "127.0.0.1:10102"
  metrics_path: "/metrics"
  health_path: "/health"
  ready_path: "/ready"

logging:
  level: "info"     # debug, info, warn, error
  format: "logfmt"  # logfmt, json

color: "auto"       # auto, always, never

collectors:
  wifi:
    enabled: true
  battery:
    enabled: true
  thermal:
    enabled: true
  wdutil:
    enabled: false   # requires root/sudo

instance:
  name: ""          # empty: read from ~/.instance
  labels:
    environment: "home"
```

## Running as launchd Service

```bash
# Check status
launchctl list kz.neko.darwin-exporter

# View logs (default sudo mode path)
tail -f ~/.local/state/darwin-exporter/darwin-exporter.log
```

## Integration with vmagent

Add to your vmagent scrape config (`~/.config/vmagent/scrape.yml`):

```yaml
scrape_configs:
- job_name: integrations/darwin
  static_configs:
  - targets:
    - '127.0.0.1:10102'
  relabel_configs:
  - target_label: instance
    replacement: 'macbook'
```

## Grafana Dashboard

Prebuilt dashboard JSON is provided at:

- `docs/grafana/dashboard.json`

Import steps:

1. Open Grafana: `Dashboards` -> `New` -> `Import`.
1. Upload `docs/grafana/dashboard.json`.
1. Select your metrics datasource in the `Data Source` variable.
1. Pick `Instance` and `Disk device` from dashboard variables.

Dashboard sync script (multi-org):

- Script: `scripts/grafana-dashboard-sync.sh`
- Auth: `GRAFANA_USER` + `GRAFANA_PASS` or `GRAFANA_ORG_TOKENS="1=... 2=..."`

Push dashboard to multiple orgs:

```bash
export GRAFANA_USER=reader
export GRAFANA_PASS=pass

scripts/grafana-dashboard-sync.sh push \
  --url https://grafana.example.com \
  --org-ids "1 2 3" \
  --dashboard docs/grafana/dashboard.json
```

Push with service account tokens (per-org):

```bash
export GRAFANA_ORG_TOKENS="1=token_org1 2=token_org2 3=token_org3"

scripts/grafana-dashboard-sync.sh push \
  --url https://grafana.example.com \
  --org-ids "1 2 3" \
  --dashboard docs/grafana/dashboard.json
```

Pull dashboard back from source org to local JSON (reverse sync):

```bash
export GRAFANA_USER=reader
export GRAFANA_PASS=pass

scripts/grafana-dashboard-sync.sh pull \
  --url https://grafana.example.com \
  --get-org-id 1 \
  --uid darwin-exporter-overview \
  --dashboard docs/grafana/dashboard.json
```

Notes:

- Dashboard combines `node_exporter` metrics (`node_*`) and `darwin-exporter` metrics (`darwin_*`).
- If Wi-Fi advanced panels are empty, enable `wdutil` collector and install service with `--type=sudo` or `--type=root`.
- Current datasource variable defaults to `victoriametrics-metrics-datasource`; adjust it after import if your stack uses another Prometheus-compatible datasource type.
- `GRAFANA_TOKEN` is org-scoped in Grafana service accounts and is suitable only for single-org sync.
- If `GRAFANA_ORG_TOKENS` is set, each org from `--org-ids` must be present in the token map.
- Push requires stable dashboard `uid`; otherwise Grafana creates new dashboards.

## Endpoints

| Endpoint       | Description            |
| -------------- | ---------------------- |
| `GET /metrics` | Prometheus metrics     |
| `GET /health`  | Liveness probe (JSON)  |
| `GET /ready`   | Readiness probe (JSON) |
| `GET /`        | Landing page           |

## Architecture

darwin-exporter is a **complement** to node_exporter, not a replacement.
It exports only macOS-specific metrics that node_exporter does not cover.

```ascii
vmagent
  ├── scrape node_exporter:9100      → node_cpu_*, node_memory_*, ...
  └── scrape darwin-exporter:10102   → darwin_wifi_*, darwin_battery_*, darwin_thermal_*, darwin_*_temperature_*
```

## Development

```bash
make test       # Run tests
make test-race  # Run tests with race detector
make lint       # golangci-lint
make fmt        # gofmt
make vet        # go vet
```

## Troubleshooting

### Build fails with CGO disabled

`darwin-exporter` requires CGo on macOS (`CGO_ENABLED=1`) because WiFi and SMC collectors use Apple frameworks.

If you see an error containing `darwinExporterRequiresCGOEnabled1AndBuildTagCgo`, build with CGo enabled:

```bash
CGO_ENABLED=1 go build -tags cgo ./...
# or
make build
```
