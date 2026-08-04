#!/bin/sh
set -eu

default_schedule="17 3 * * *"
server=""
certificate=""
artifact="fullchain.pem"
output=""
schedule="$default_schedule"

usage() {
  cat <<'EOF'
Install a recurring CertVault certificate download.

Usage:
  install.sh --server URL --certificate NAME --artifact FILE --output PATH [--schedule CRON]

The API token is read from CERTVAULT_API_KEY or prompted for securely.
EOF
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --server) server=${2-}; shift 2 ;;
    --certificate) certificate=${2-}; shift 2 ;;
    --artifact) artifact=${2-}; shift 2 ;;
    --output) output=${2-}; shift 2 ;;
    --schedule) schedule=${2-}; shift 2 ;;
    --help|-h) usage; exit 0 ;;
    *) printf 'Unknown option: %s\n' "$1" >&2; usage >&2; exit 2 ;;
  esac
done

if [ -z "$server" ] || [ -z "$certificate" ] || [ -z "$output" ]; then
  usage >&2
  exit 2
fi

case "$artifact" in
  certificate.pem|chain.pem|fullchain.pem|private-key.pem) ;;
  *) printf 'Unsupported artifact: %s\n' "$artifact" >&2; exit 2 ;;
esac

case "$schedule" in
  *[!A-Za-z0-9,'*/?_-\ ']*|*'
'*) printf 'Invalid cron schedule\n' >&2; exit 2 ;;
esac
set -f
set -- $schedule
set +f
if [ "$#" -ne 5 ]; then
  printf 'Cron schedule must contain five fields\n' >&2
  exit 2
fi

token=${CERTVAULT_API_KEY-}
if [ -z "$token" ]; then
  if [ ! -r /dev/tty ]; then
    printf 'Set CERTVAULT_API_KEY when no interactive terminal is available\n' >&2
    exit 2
  fi
  printf 'CertVault API key: ' >/dev/tty
  stty -echo </dev/tty
  IFS= read -r token </dev/tty
  stty echo </dev/tty
  printf '\n' >/dev/tty
fi
if [ -z "$token" ]; then
  printf 'API key cannot be empty\n' >&2
  exit 2
fi

job=$(printf '%s-%s' "$certificate" "$artifact" | tr -c 'A-Za-z0-9._-' '-')
config_dir=${CERTVAULT_CLIENT_CONFIG_DIR:-"$HOME/.config/certvault"}
script_dir=${CERTVAULT_CLIENT_SCRIPT_DIR:-"$HOME/.local/libexec"}
token_file="$config_dir/$job.token"
sync_script="$script_dir/certvault-sync"
job_script="$script_dir/certvault-$job"
url="${server%/}/api/v1/certificates/$certificate/$artifact"

install -d -m 700 "$config_dir" "$script_dir"
install -d -m 755 "$(dirname "$output")"
printf '%s\n' "$token" >"$token_file"
chmod 600 "$token_file"

cat >"$sync_script" <<'EOF'
#!/bin/sh
set -eu
token_file=$1
url=$2
output=$3
artifact=$4
token=$(cat "$token_file")
temporary=$(mktemp "${output}.tmp.XXXXXX")
trap 'rm -f "$temporary"' EXIT HUP INT TERM
curl -fsSL -H "Authorization: Bearer $token" "$url" -o "$temporary"
case "$artifact" in
  private-key.pem) chmod 600 "$temporary" ;;
  *) chmod 644 "$temporary" ;;
esac
mv -f "$temporary" "$output"
trap - EXIT HUP INT TERM
EOF
chmod 700 "$sync_script"

shell_quote() {
  printf "'%s'" "$(printf '%s' "$1" | sed "s/'/'\\\\''/g")"
}

{
  printf '#!/bin/sh\nexec '
  shell_quote "$sync_script"
  printf ' '
  shell_quote "$token_file"
  printf ' '
  shell_quote "$url"
  printf ' '
  shell_quote "$output"
  printf ' '
  shell_quote "$artifact"
  printf '\n'
} >"$job_script"
chmod 700 "$job_script"

marker="# certvault:$job"
existing=$(crontab -l 2>/dev/null || true)
{
  printf '%s\n' "$existing" | grep -Fv "$marker" || true
  printf '%s ' "$schedule"
  shell_quote "$job_script"
  printf ' %s\n' "$marker"
} | crontab -

"$job_script"
printf 'Installed CertVault sync job %s -> %s\n' "$job" "$output"
