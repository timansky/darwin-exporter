# darwin-exporter Usage

Current CLI guide.

## Quick Start

```bash
mkdir -p ~/.local/bin
cp darwin-exporter ~/.local/bin
chmod +x ~/.local/bin/darwin-exporter

./darwin-exporter run

~/.local/bin/darwin-exporter service install
```

## Command Model

```bash
darwin-exporter [global flags] <command> [subcommand/flags]
```

## Service Lifecycle

```bash
sudo darwin-exporter service start --type=sudo
sudo darwin-exporter service status --type=sudo
sudo darwin-exporter service logs --type=sudo --lines=100
sudo darwin-exporter service stop --type=sudo
sudo darwin-exporter service restart --type=sudo
sudo darwin-exporter service enable --type=sudo
sudo darwin-exporter service disable --type=sudo
```

`service start` is idempotent: if the service is already running, no restart is performed.

## Temperature Metrics (SMC)

`darwin_cpu_temperature_celsius`, `darwin_gpu_temperature_celsius`, and `darwin_disk_temperature_celsius` are read from built-in SMC keys.
Default `make build` uses CGo, so these metrics are enabled by default.

Permissions:

- `service install --type=sudo`: sudoers is configured for `wdutil`, `ipconfig`, and `powermetrics`.
- SMC temperature collection does not require sudoers/root.

## macOS Security Approval (After Install)

If macOS blocks launch with "Apple could not verify darwin-exporter", approve the **binary** path from `ProgramArguments[0]`:

```bash
plutil -extract ProgramArguments.0 raw ~/Library/LaunchAgents/kz.neko.darwin-exporter.plist
```

Then:

1. Run binary once:

```bash
/absolute/path/to/darwin-exporter --help
```

1. Click `Open Anyway` in `System Settings -> Privacy & Security`.

Or use CLI workaround:

```bash
xattr -dr com.apple.quarantine /absolute/path/to/darwin-exporter
```

Finally restart service:

```bash
sudo darwin-exporter service restart --type=sudo
```

Disable colors:

```bash
darwin-exporter --color=never service status
```

Via config:

```yaml
color: "auto"   # auto, always, never
```

## Service Uninstall

```bash
sudo darwin-exporter service uninstall
sudo darwin-exporter service uninstall --type=root
```

With log/config cleanup:

```bash
sudo darwin-exporter service uninstall --purge \
  --config ~/.config/darwin-exporter/config.yml \
  --log-dir ~/.local/state/darwin-exporter
```

## Scrape Configuration (vmagent / Prometheus)

Expose exporter locally (default):

- target: `127.0.0.1:10102`
- metrics path: `/metrics`

`vmagent`(scrape.yml) or `Prometheus`(prometheus.yml) example:

```yaml
scrape_configs:
- job_name: integrations/darwin
  static_configs:
  - targets:
    - "127.0.0.1:10102"
    labels:
      instance: "macbook"
```

Quick check:

```bash
curl -fsS http://127.0.0.1:10102/metrics | head
```

## Grafana Dashboard

Dashboard file:

- `docs/grafana/dashboard.json`

Import:

1. Grafana -> `Dashboards` -> `New` -> `Import`.
1. Upload `docs/grafana/dashboard.json`.
1. Select datasource in dashboard variable `Data Source`.
1. Set `Instance` and `Disk device`.

Sync script (multi-org + reverse sync):

```bash
export GRAFANA_USER=reader
export GRAFANA_PASS=pass

# Push dashboard to orgs 1 2 3
scripts/grafana-dashboard-sync.sh push \
  --url https://grafana.example.com \
  --org-ids "1 2 3" \
  --dashboard docs/grafana/dashboard.json

# Push with service account tokens (per-org)
export GRAFANA_ORG_TOKENS="1=token_org1 2=token_org2 3=token_org3"
scripts/grafana-dashboard-sync.sh push \
  --url https://grafana.example.com \
  --org-ids "1 2 3" \
  --dashboard docs/grafana/dashboard.json

# Pull dashboard from source org back to local file
scripts/grafana-dashboard-sync.sh pull \
  --url https://grafana.example.com \
  --get-org-id 1 \
  --uid darwin-exporter-overview \
  --dashboard docs/grafana/dashboard.json
```

`GRAFANA_TOKEN` подходит только для single-org sync (service account token в Grafana org-scoped).
Если задан `GRAFANA_ORG_TOKENS`, в нем должны быть токены для всех org из `--org-ids`.
Для `push` обязателен стабильный `uid`, иначе Grafana создаёт новый dashboard.

Requirements:

- Metrics from both `node_exporter` and `darwin-exporter`.
- For advanced Wi-Fi panels (`darwin_wdutil_*`), enable `wdutil` collector and use `service install --type=sudo` or `--type=root`.

## Autocompletion

Generate scripts:

```bash
darwin-exporter completion bash
darwin-exporter completion zsh
```
