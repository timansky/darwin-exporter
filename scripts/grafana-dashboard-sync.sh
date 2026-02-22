#!/usr/bin/env bash
set -euo pipefail

SCRIPT_NAME="$(basename "$0")"
DEFAULT_DASHBOARD_PATH="docs/grafana/dashboard.json"

die() {
  printf 'ERROR: %s\n' "$*" >&2
  exit 1
}

log() {
  printf '%s\n' "$*" >&2
}

usage() {
  cat <<EOF
Usage:
  $SCRIPT_NAME push --url <grafana_url> --org-ids "<id1 id2 ...>" [options]
  $SCRIPT_NAME pull --url <grafana_url> [--get-org-id 1] [options]

Modes:
  push  Push dashboard JSON to multiple Grafana organizations.
  pull  Pull dashboard JSON from source org (default: 1) back to local file.

Options:
  --dashboard <path>   Dashboard JSON path (default: ${DEFAULT_DASHBOARD_PATH})
  --uid <uid>          Dashboard UID. Default: read from dashboard file. Required for push if file uid is empty.
  --org-ids <list>     Space-separated org IDs for push (example: "1 2 3")
  --get-org-id <id>    Source org ID for pull (default: 1)
  --folder-id <id>     Destination folder ID for push (default: 0)
  --folder-uid <uid>   Destination folder UID for push (overrides --folder-id)
  --message <text>     Commit message for dashboard version history
  --insecure           Disable TLS certificate verification
  --dry-run            Print actions without writing to Grafana/local file
  -h, --help           Show this help

Auth (set one method):
  export GRAFANA_TOKEN=<token>
  OR
  export GRAFANA_USER=<user>
  export GRAFANA_PASS=<pass>
  OR
  export GRAFANA_ORG_TOKENS="1=token_org1 2=token_org2"
EOF
}

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || die "Required command not found: $1"
}

MODE="${1:-}"
if [[ -z "${MODE}" ]]; then
  usage
  exit 1
fi
shift || true

if [[ "${MODE}" == "-h" || "${MODE}" == "--help" ]]; then
  usage
  exit 0
fi

if [[ "${MODE}" != "push" && "${MODE}" != "pull" ]]; then
  die "Unknown mode: ${MODE}. Use push or pull."
fi

GRAFANA_URL=""
DASHBOARD_PATH="${DEFAULT_DASHBOARD_PATH}"
DASHBOARD_UID=""
ORG_IDS_RAW=""
GET_ORG_ID="1"
FOLDER_ID="0"
FOLDER_UID=""
MESSAGE="sync dashboard from git"
INSECURE="false"
DRY_RUN="false"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --url)
      [[ $# -ge 2 ]] || die "--url requires a value"
      GRAFANA_URL="$2"
      shift 2
      ;;
    --dashboard)
      [[ $# -ge 2 ]] || die "--dashboard requires a value"
      DASHBOARD_PATH="$2"
      shift 2
      ;;
    --uid)
      [[ $# -ge 2 ]] || die "--uid requires a value"
      DASHBOARD_UID="$2"
      shift 2
      ;;
    --org-ids)
      [[ $# -ge 2 ]] || die "--org-ids requires a value"
      ORG_IDS_RAW="$2"
      shift 2
      ;;
    --get-org-id)
      [[ $# -ge 2 ]] || die "--get-org-id requires a value"
      GET_ORG_ID="$2"
      shift 2
      ;;
    --folder-id)
      [[ $# -ge 2 ]] || die "--folder-id requires a value"
      FOLDER_ID="$2"
      shift 2
      ;;
    --folder-uid)
      [[ $# -ge 2 ]] || die "--folder-uid requires a value"
      FOLDER_UID="$2"
      shift 2
      ;;
    --message)
      [[ $# -ge 2 ]] || die "--message requires a value"
      MESSAGE="$2"
      shift 2
      ;;
    --insecure)
      INSECURE="true"
      shift
      ;;
    --dry-run)
      DRY_RUN="true"
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      die "Unknown argument: $1"
      ;;
  esac
done

[[ -n "${GRAFANA_URL}" ]] || die "--url is required"

require_cmd curl
require_cmd jq

DEFAULT_AUTH_KIND="none"
DEFAULT_AUTH_ARGS=()
if [[ -n "${GRAFANA_USER:-}" || -n "${GRAFANA_PASS:-}" ]]; then
  [[ -n "${GRAFANA_USER:-}" && -n "${GRAFANA_PASS:-}" ]] || die "Set both GRAFANA_USER and GRAFANA_PASS"
  DEFAULT_AUTH_KIND="basic"
  DEFAULT_AUTH_ARGS=(-u "${GRAFANA_USER}:${GRAFANA_PASS}")
elif [[ -n "${GRAFANA_TOKEN:-}" ]]; then
  DEFAULT_AUTH_KIND="token"
  DEFAULT_AUTH_ARGS=(-H "Authorization: Bearer ${GRAFANA_TOKEN}")
fi

ORG_TOKEN_ORGS=()
ORG_TOKEN_VALUES=()
parse_org_tokens() {
  local raw="$1"
  local pair org token i
  for pair in ${raw}; do
    [[ "${pair}" == *=* ]] || die "Invalid GRAFANA_ORG_TOKENS entry: ${pair}. Expected <org>=<token>"
    org="${pair%%=*}"
    token="${pair#*=}"
    [[ "${org}" =~ ^[0-9]+$ ]] || die "Invalid org id in GRAFANA_ORG_TOKENS: ${org}"
    [[ -n "${token}" ]] || die "Empty token for org ${org} in GRAFANA_ORG_TOKENS"
    for (( i=0; i<${#ORG_TOKEN_ORGS[@]}; i++ )); do
      [[ "${ORG_TOKEN_ORGS[$i]}" == "${org}" ]] && die "Duplicate org id in GRAFANA_ORG_TOKENS: ${org}"
    done
    ORG_TOKEN_ORGS+=("${org}")
    ORG_TOKEN_VALUES+=("${token}")
  done
}

lookup_org_token() {
  local target_org="$1"
  local i
  for (( i=0; i<${#ORG_TOKEN_ORGS[@]}; i++ )); do
    if [[ "${ORG_TOKEN_ORGS[$i]}" == "${target_org}" ]]; then
      printf '%s' "${ORG_TOKEN_VALUES[$i]}"
      return 0
    fi
  done
  return 1
}

REQUEST_AUTH_ARGS=()
set_request_auth() {
  local org_id="$1"
  local token=""
  REQUEST_AUTH_ARGS=()
  if [[ ${#ORG_TOKEN_ORGS[@]} -gt 0 ]]; then
    token="$(lookup_org_token "${org_id}")" || return 1
    REQUEST_AUTH_ARGS=(-H "Authorization: Bearer ${token}")
    return 0
  fi
  if [[ ${#DEFAULT_AUTH_ARGS[@]} -gt 0 ]]; then
    REQUEST_AUTH_ARGS=("${DEFAULT_AUTH_ARGS[@]}")
    return 0
  fi
  return 1
}

parse_org_tokens "${GRAFANA_ORG_TOKENS:-}"
if [[ ${#DEFAULT_AUTH_ARGS[@]} -eq 0 && ${#ORG_TOKEN_ORGS[@]} -eq 0 ]]; then
  die "Set auth via GRAFANA_USER/GRAFANA_PASS, GRAFANA_TOKEN, or GRAFANA_ORG_TOKENS"
fi

CURL_BASE=(curl -sS)
if [[ "${INSECURE}" == "true" ]]; then
  CURL_BASE+=(-k)
fi

TMP_FILES=()
cleanup() {
  if [[ ${#TMP_FILES[@]} -gt 0 ]]; then
    rm -f "${TMP_FILES[@]}"
  fi
}
trap cleanup EXIT

make_tmp() {
  local file
  file="$(mktemp)"
  TMP_FILES+=("${file}")
  printf '%s' "${file}"
}

api_request() {
  local method="$1"
  local path="$2"
  local org_id="$3"
  local data_file="${4:-}"
  local url="${GRAFANA_URL%/}${path}"

  local cmd=("${CURL_BASE[@]}" -X "${method}" "${url}" -H "Accept: application/json" -H "X-Grafana-Org-Id: ${org_id}")
  cmd+=("${REQUEST_AUTH_ARGS[@]}")
  if [[ -n "${data_file}" ]]; then
    cmd+=(-H "Content-Type: application/json" --data-binary "@${data_file}")
  fi

  local response http_code body
  response="$("${cmd[@]}" -w $'\n%{http_code}')"
  http_code="${response##*$'\n'}"
  body="${response%$'\n'*}"

  if (( http_code < 200 || http_code >= 300 )); then
    log "HTTP ${http_code} for ${method} ${path} (org=${org_id})"
    [[ -n "${body}" ]] && log "${body}"
    return 1
  fi

  printf '%s' "${body}"
}

resolve_uid_from_file() {
  local file="$1"
  jq -r 'if (has("dashboard") and (.dashboard|type=="object")) then (.dashboard.uid // "") else (.uid // "") end' "${file}"
}

sanitize_dashboard() {
  local input_file="$1"
  local output_file="$2"
  jq 'if (has("dashboard") and (.dashboard|type=="object")) then .dashboard else . end | .id = null' "${input_file}" > "${output_file}"
}

ORG_IDS=()
parse_org_ids() {
  local raw="$1"
  local filtered=()
  local org_id
  for org_id in ${raw}; do
    [[ -z "${org_id}" ]] && continue
    [[ "${org_id}" =~ ^[0-9]+$ ]] || die "Invalid org id in --org-ids: ${org_id}"
    filtered+=("${org_id}")
  done
  ORG_IDS=("${filtered[@]}")
  [[ ${#ORG_IDS[@]} -gt 0 ]] || die "--org-ids is required for push"
}

push_dashboard() {
  [[ -f "${DASHBOARD_PATH}" ]] || die "Dashboard file not found: ${DASHBOARD_PATH}"
  parse_org_ids "${ORG_IDS_RAW}"

  if [[ "${DEFAULT_AUTH_KIND}" == "token" && ${#ORG_TOKEN_ORGS[@]} -eq 0 && ${#ORG_IDS[@]} -gt 1 ]]; then
    die "GRAFANA_TOKEN is org-scoped. For multi-org push set GRAFANA_ORG_TOKENS=\"1=... 2=...\" or use GRAFANA_USER/GRAFANA_PASS."
  fi

  if [[ ${#ORG_TOKEN_ORGS[@]} -gt 0 ]]; then
    local missing_orgs=()
    local org_id
    for org_id in "${ORG_IDS[@]}"; do
      lookup_org_token "${org_id}" >/dev/null || missing_orgs+=("${org_id}")
    done
    if [[ ${#missing_orgs[@]} -gt 0 ]]; then
      die "Missing per-org token(s) for org id(s): ${missing_orgs[*]}. Set GRAFANA_ORG_TOKENS with all target orgs."
    fi
  fi

  if [[ -z "${FOLDER_UID}" && ! "${FOLDER_ID}" =~ ^[0-9]+$ ]]; then
    die "--folder-id must be numeric"
  fi

  local dashboard_file payload_file dashboard_with_uid_file
  dashboard_file="$(make_tmp)"
  payload_file="$(make_tmp)"

  sanitize_dashboard "${DASHBOARD_PATH}" "${dashboard_file}"

  if [[ -z "${DASHBOARD_UID}" ]]; then
    DASHBOARD_UID="$(resolve_uid_from_file "${dashboard_file}")"
  fi

  if [[ -z "${DASHBOARD_UID}" ]]; then
    die "Push requires stable dashboard uid. Set --uid or define uid in ${DASHBOARD_PATH}."
  fi

  # Force stable uid in payload to update existing dashboard instead of creating a new one.
  dashboard_with_uid_file="$(make_tmp)"
  jq --arg uid "${DASHBOARD_UID}" '.uid = $uid | .id = null' "${dashboard_file}" > "${dashboard_with_uid_file}"
  dashboard_file="${dashboard_with_uid_file}"

  local org_id response status url version
  for org_id in "${ORG_IDS[@]}"; do
    set_request_auth "${org_id}" || die "No auth configured for org ${org_id}. Set GRAFANA_ORG_TOKENS or default auth."
    if [[ -n "${FOLDER_UID}" ]]; then
      jq -n \
        --slurpfile dashboard "${dashboard_file}" \
        --arg folderUid "${FOLDER_UID}" \
        --arg message "${MESSAGE}" \
        '{dashboard: $dashboard[0], folderUid: $folderUid, overwrite: true, message: $message}' > "${payload_file}"
    else
      jq -n \
        --slurpfile dashboard "${dashboard_file}" \
        --argjson folderId "${FOLDER_ID}" \
        --arg message "${MESSAGE}" \
        '{dashboard: $dashboard[0], folderId: $folderId, overwrite: true, message: $message}' > "${payload_file}"
    fi

    if [[ "${DRY_RUN}" == "true" ]]; then
      log "[DRY-RUN] push org=${org_id} dashboard=${DASHBOARD_PATH}"
      continue
    fi

    response="$(api_request "POST" "/api/dashboards/db" "${org_id}" "${payload_file}")" || die "Push failed for org ${org_id}"
    status="$(jq -r '.status // ""' <<< "${response}")"
    [[ "${status}" == "success" ]] || die "Grafana returned non-success for org ${org_id}: ${response}"

    url="$(jq -r '.url // "-"' <<< "${response}")"
    version="$(jq -r '.version // "-"' <<< "${response}")"
    printf 'org=%s status=%s version=%s url=%s\n' "${org_id}" "${status}" "${version}" "${url}"
  done
}

pull_dashboard() {
  [[ "${GET_ORG_ID}" =~ ^[0-9]+$ ]] || die "--get-org-id must be numeric"
  set_request_auth "${GET_ORG_ID}" || die "No auth configured for org ${GET_ORG_ID}. Set GRAFANA_ORG_TOKENS or default auth."

  if [[ -z "${DASHBOARD_UID}" && -f "${DASHBOARD_PATH}" ]]; then
    DASHBOARD_UID="$(resolve_uid_from_file "${DASHBOARD_PATH}")"
  fi
  [[ -n "${DASHBOARD_UID}" ]] || die "--uid is required for pull when dashboard file does not contain uid"

  local response title version output_file
  response="$(api_request "GET" "/api/dashboards/uid/${DASHBOARD_UID}" "${GET_ORG_ID}")" || die "Pull failed for uid=${DASHBOARD_UID} org=${GET_ORG_ID}"
  title="$(jq -r '.dashboard.title // ""' <<< "${response}")"
  version="$(jq -r '.dashboard.version // ""' <<< "${response}")"
  [[ -n "${title}" ]] || die "Dashboard uid=${DASHBOARD_UID} not found in org=${GET_ORG_ID}"

  if [[ "${DRY_RUN}" == "true" ]]; then
    printf '[DRY-RUN] pull org=%s uid=%s title=%s version=%s -> %s\n' "${GET_ORG_ID}" "${DASHBOARD_UID}" "${title}" "${version}" "${DASHBOARD_PATH}"
    return
  fi

  output_file="$(make_tmp)"
  jq '.dashboard | .id = null' <<< "${response}" > "${output_file}"
  mkdir -p "$(dirname "${DASHBOARD_PATH}")"
  mv "${output_file}" "${DASHBOARD_PATH}"
  printf 'pulled uid=%s title=%s version=%s org=%s -> %s\n' "${DASHBOARD_UID}" "${title}" "${version}" "${GET_ORG_ID}" "${DASHBOARD_PATH}"
}

case "${MODE}" in
  push)
    push_dashboard
    ;;
  pull)
    pull_dashboard
    ;;
  *)
    die "Unexpected mode: ${MODE}"
    ;;
esac
