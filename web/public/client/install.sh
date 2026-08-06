#!/bin/sh
set -eu

default_schedule="17 3 * * *"
server=""
certificate=""
file_specs=""
destination=""
schedule="$default_schedule"
reload_command=""

usage() {
  cat <<'EOF'
Install a recurring CertVault certificate download.

Usage:
  install.sh --server URL --certificate NAME --destination DIRECTORY [OPTIONS]

The API token is read from CERTVAULT_API_KEY or prompted for securely.

Options:
  --file ARTIFACT[=OUTPUT]
                          Download an artifact, optionally renaming it. Repeatable.
  --schedule CRON        Cron schedule. Defaults to "17 3 * * *".
  --reload-command CMD   Shell command to run after one or more files change.

Artifacts are certificate.crt, chain.crt, fullchain.crt, and private.key.
EOF
}

# Parse only simple flags here; file mappings are normalized and validated as a
# complete set below so duplicate artifacts and output names can be rejected.
while [ "$#" -gt 0 ]; do
  case "$1" in
    --server) server=${2-}; shift 2 ;;
    --certificate) certificate=${2-}; shift 2 ;;
    --file)
      spec=${2-}
      file_specs="${file_specs}${file_specs:+,}$spec"
      shift 2
      ;;
    --destination) destination=${2-}; shift 2 ;;
    --schedule) schedule=${2-}; shift 2 ;;
    --reload-command) reload_command=${2-}; shift 2 ;;
    --help|-h) usage; exit 0 ;;
    *) printf 'Unknown option: %s\n' "$1" >&2; usage >&2; exit 2 ;;
  esac
done

if [ -z "$server" ] || [ -z "$certificate" ] || [ -z "$destination" ]; then
  usage >&2
  exit 2
fi

# Convert every ARTIFACT[=OUTPUT] value into the explicit ARTIFACT=OUTPUT form
# consumed by the generated sync script. Output names are restricted to
# basenames so a mapping cannot escape the selected destination directory.
old_ifs=$IFS
IFS=,
set -- $file_specs
IFS=$old_ifs
if [ "$#" -eq 0 ]; then
  printf 'At least one file is required\n' >&2
  exit 2
fi
artifacts=""
normalized_specs=""
seen_artifacts="|"
seen_outputs="|"
for spec in "$@"; do
  case "$spec" in
    *=*)
      file=${spec%%=*}
      output=${spec#*=}
      ;;
    *)
      file=$spec
      output=$spec
      ;;
  esac
  case "$file" in
    certificate.crt|chain.crt|fullchain.crt|private.key) ;;
    *) printf 'Unsupported file: %s\n' "$file" >&2; exit 2 ;;
  esac
  case "$output" in
    ""|.|..|*/*|*[!A-Za-z0-9._-]*)
      printf 'Invalid output filename: %s\n' "$output" >&2
      exit 2
      ;;
  esac
  case "$seen_artifacts" in
    *"|$file|"*) printf 'Duplicate artifact: %s\n' "$file" >&2; exit 2 ;;
  esac
  case "$seen_outputs" in
    *"|$output|"*) printf 'Duplicate output filename: %s\n' "$output" >&2; exit 2 ;;
  esac
  seen_artifacts="$seen_artifacts$file|"
  seen_outputs="$seen_outputs$output|"
  artifacts="${artifacts}${artifacts:+,}$file"
  normalized_spec="$file=$output"
  normalized_specs="${normalized_specs}${normalized_specs:+,}$normalized_spec"
done
file_specs=$normalized_specs

# Cron expressions are stored verbatim in the generated crontab. Reject shell
# metacharacters and require the traditional five-field format.
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

# Prefer a token supplied by automation, falling back to a non-echoing terminal
# prompt for an interactive installation.
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

# The artifact set identifies a job. Reinstalling the same set intentionally
# replaces its destination, token, schedule, mappings, and reload command.
job=$(printf '%s-%s' "$certificate" "$artifacts" | tr -c 'A-Za-z0-9._-' '-')
config_dir=${CERTVAULT_CLIENT_CONFIG_DIR:-"$HOME/.config/certvault"}
script_dir=${CERTVAULT_CLIENT_SCRIPT_DIR:-"$HOME/.local/libexec"}
token_file="$config_dir/$job.token"
etag_dir="$config_dir/$job.etags"
sync_script="$script_dir/certvault-sync"
job_script="$script_dir/certvault-$job"

install -d -m 700 "$config_dir" "$script_dir" "$etag_dir"
if [ ! -d "$destination" ]; then
  install -d -m 755 "$destination"
fi
printf '%s\n' "$token" >"$token_file"
chmod 600 "$token_file"

# Installing again replaces the job configuration. Discard validators from the
# previous configuration so the immediate sync populates a changed destination
# instead of treating its files as already current.
for file in certificate.crt chain.crt fullchain.crt private.key; do
  rm -f "$etag_dir/$file"
done

cat >"$sync_script" <<'EOF'
#!/bin/sh
set -eu
token_file=$1
server=$2
certificate=$3
destination=$4
etag_dir=$5
reload_command=$6
shift 6
token=$(cat "$token_file")

# Prefer staging beside the destination so deployment is an atomic rename.
# Some special filesystems, notably Proxmox pmxcfs, allow writes but not chmod;
# detect that case and stage in the private ETag directory instead.
deployment=move
temporary=$(mktemp -d "$destination/.certvault.tmp.XXXXXX" 2>/dev/null || true)
if [ -n "$temporary" ]; then
  permission_probe="$temporary/.permission-test"
  : >"$permission_probe"
  if ! chmod 600 "$permission_probe" 2>/dev/null; then
    rm -rf "$temporary"
    temporary=""
  else
    rm -f "$permission_probe"
  fi
fi
if [ -z "$temporary" ]; then
  temporary=$(mktemp -d "$etag_dir/.certvault.tmp.XXXXXX")
  deployment=copy
fi
trap 'rm -rf "$temporary"' EXIT HUP INT TERM

# Download and validate every changed artifact before deploying any of them.
# ETags avoid rewriting files—and avoid service reloads—when nothing changed.
for spec in "$@"; do
  file=${spec%%=*}
  output=${spec#*=}
  url="${server%/}/api/v1/certificates/$certificate/$file"
  headers="$temporary/$file.headers"
  etag_file="$etag_dir/$file"
  if [ -s "$etag_file" ]; then
    etag=$(cat "$etag_file")
    status=$(curl -fsSL -D "$headers" -w '%{http_code}' \
      -H "Authorization: Bearer $token" -H "If-None-Match: $etag" \
      "$url" -o "$temporary/$output")
  else
    status=$(curl -fsSL -D "$headers" -w '%{http_code}' \
      -H "Authorization: Bearer $token" \
      "$url" -o "$temporary/$output")
  fi
  case "$status" in
    200) ;;
    304)
      rm -f "$temporary/$output" "$headers"
      continue
      ;;
    *)
      printf 'Unexpected HTTP status %s for %s\n' "$status" "$file" >&2
      exit 1
      ;;
  esac
  case "$file" in
    private.key) chmod 600 "$temporary/$output" ;;
    *) chmod 644 "$temporary/$output" ;;
  esac
  awk 'tolower($1) == "etag:" { sub(/\r$/, "", $2); value = $2 } END { if (value != "") print value }' \
    "$headers" >"$temporary/$file.etag"
  rm -f "$headers"
done

# Same-filesystem staging uses atomic moves. The fallback uses ordinary copies
# because chmod-limited destinations cannot host the protected staging files.
changed=0
for spec in "$@"; do
  file=${spec%%=*}
  output=${spec#*=}
  if [ -f "$temporary/$output" ]; then
    if [ "$deployment" = "copy" ]; then
      cp -f "$temporary/$output" "$destination/$output"
      rm -f "$temporary/$output"
    else
      mv -f "$temporary/$output" "$destination/$output"
    fi
    changed=1
    if [ -s "$temporary/$file.etag" ]; then
      mv -f "$temporary/$file.etag" "$etag_dir/$file"
      chmod 600 "$etag_dir/$file"
    else
      rm -f "$temporary/$file.etag" "$etag_dir/$file"
    fi
  fi
done
rmdir "$temporary"
trap - EXIT HUP INT TERM

# Reload only after the complete changed set has been deployed successfully.
if [ "$changed" -eq 1 ] && [ -n "$reload_command" ]; then
  /bin/sh -c "$reload_command"
fi
EOF
chmod 700 "$sync_script"

shell_quote() {
  printf "'%s'" "$(printf '%s' "$1" | sed "s/'/'\\\\''/g")"
}

# Persist a small wrapper with every value shell-quoted. Cron invokes this file
# rather than embedding credentials or complex arguments in the crontab.
{
  printf '#!/bin/sh\nexec '
  shell_quote "$sync_script"
  printf ' '
  shell_quote "$token_file"
  printf ' '
  shell_quote "$server"
  printf ' '
  shell_quote "$certificate"
  printf ' '
  shell_quote "$destination"
  printf ' '
  shell_quote "$etag_dir"
  printf ' '
  shell_quote "$reload_command"
  old_ifs=$IFS
  IFS=,
  set -- $file_specs
  IFS=$old_ifs
  for spec in "$@"; do
    printf ' '
    shell_quote "$spec"
  done
  printf '\n'
} >"$job_script"
chmod 700 "$job_script"

# Replace this job's existing cron entry without disturbing unrelated entries.
marker="# certvault:$job"
existing=$(crontab -l 2>/dev/null || true)
{
  printf '%s\n' "$existing" | grep -Fv "$marker" || true
  printf '%s ' "$schedule"
  shell_quote "$job_script"
  printf ' %s\n' "$marker"
} | crontab -

"$job_script"
printf 'Installed CertVault sync job %s -> %s\n' "$job" "$destination"
