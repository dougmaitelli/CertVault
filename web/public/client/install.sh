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
  --reload-command CMD   Shell command to run after a new bundle is deployed.

Artifacts are certificate.crt, chain.crt, fullchain.crt, and private.key.
Requires Linux with curl, tar, OpenSSL, flock, and GNU mv.
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

# Requires Linux utilities: curl, tar, OpenSSL, flock, and mv with -T.
# Installing again replaces the job configuration. Discard validators from the
# previous configuration so the immediate sync populates a changed destination
# instead of treating its files as already current.
for file in certificate.crt chain.crt fullchain.crt private.key; do
  rm -f "$etag_dir/$file"
done
rm -f "$etag_dir/bundle"

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

# Serialize the whole sync, including reload, across jobs for this destination.
lock_id=$(printf '%s' "$(cd "$destination" && pwd -P)" | openssl dgst -sha256 | awk '{print $NF}')
exec 9>"$(dirname "$etag_dir")/destination-$lock_id.lock"
flock -x 9
umask 077

# Detect filesystems such as pmxcfs which require fixed-file copies.
deployment=atomic
temporary=$(mktemp -d "$destination/.certvault.tmp.XXXXXX" 2>/dev/null || true)
if [ -n "$temporary" ]; then
  if ! chmod 700 "$temporary" 2>/dev/null || ! touch "$temporary/.permission-test" 2>/dev/null || ! chmod 600 "$temporary/.permission-test" 2>/dev/null || ! ln -s .permission-test "$temporary/.symlink-test" 2>/dev/null; then
    rm -rf "$temporary"
    temporary=""
  else
    rm -f "$temporary/.permission-test" "$temporary/.symlink-test"
  fi
fi
if [ -z "$temporary" ]; then
  temporary=$(mktemp -d "$etag_dir/.certvault.tmp.XXXXXX")
  deployment=copy
fi
rollback=0
cleanup() {
  if [ "$rollback" -eq 1 ]; then
    for spec in "$@"; do
      output=${spec#*=}
      if [ -f "$temporary/backup/$output" ]; then
        cp -f "$temporary/backup/$output" "$destination/$output" || true
      else
        rm -f "$destination/$output"
      fi
    done
  fi
  rm -rf "$temporary"
}
trap 'cleanup "$@"' EXIT
trap 'exit 1' HUP INT TERM

files=""
for spec in "$@"; do
  file=${spec%%=*}
  files="${files}${files:+,}$file"
done
etag_file="$etag_dir/bundle"
etag=""
if [ -s "$etag_file" ]; then etag=$(cat "$etag_file"); fi
for spec in "$@"; do
  if [ ! -f "$destination/${spec#*=}" ]; then etag=""; fi
done
status=$(curl -fsSL -D "$temporary/headers" -w '%{http_code}' \
  -H "Authorization: Bearer $token" -H "If-None-Match: $etag" \
  "${server%/}/api/v1/certificates/$certificate/bundle.tar?files=$files" \
  -o "$temporary/bundle.tar")
case "$status" in
  304) exit 0 ;;
  200) ;;
  *) printf 'Unexpected bundle HTTP status %s\n' "$status" >&2; exit 1 ;;
esac

mkdir "$temporary/files"
key_file=""
for spec in "$@"; do
  file=${spec%%=*}
  output=${spec#*=}
  # Extract only the requested entry as bytes, never archive paths or symlinks.
  tar -xOf "$temporary/bundle.tar" "$file" >"$temporary/files/$output"
  case "$file" in
    private.key)
      key_file="$temporary/files/$output"
      openssl pkey -in "$key_file" -pubout >"$temporary/key.pub"
      chmod 600 "$key_file"
      ;;
    *)
      if [ "$file" != chain.crt ]; then
        openssl x509 -in "$temporary/files/$output" -pubkey -noout >"$temporary/$file.pub"
      fi
      chmod 644 "$temporary/files/$output"
      ;;
  esac
done
if [ -n "$key_file" ]; then
  for spec in "$@"; do
    file=${spec%%=*}
    case "$file" in
      certificate.crt|fullchain.crt)
        cmp -s "$temporary/$file.pub" "$temporary/key.pub" || {
          printf 'Certificate and private key do not match\n' >&2
          exit 1
        }
        ;;
    esac
  done
fi
awk 'tolower($1) == "etag:" { sub(/\r$/, "", $2); value = $2 } END { if (value != "") print value }' \
  "$temporary/headers" >"$temporary/etag"

if [ "$deployment" = atomic ]; then
  current=".certvault-$(basename "$etag_dir").current"
  old_snapshot=$(readlink "$destination/$current" 2>/dev/null || true)
  snapshot=$(mktemp -d "$destination/.certvault-version.XXXXXX")
  rmdir "$snapshot"
  mv "$temporary/files" "$snapshot"
  chmod 755 "$snapshot"
  ln -s "$(basename "$snapshot")" "$temporary/current"
  mv -Tf "$temporary/current" "$destination/$current"
  for spec in "$@"; do
    output=${spec#*=}
    ln -s "$current/$output" "$temporary/output"
    mv -Tf "$temporary/output" "$destination/$output"
  done
else
  mkdir "$temporary/backup"
  for spec in "$@"; do
    output=${spec#*=}
    if [ -f "$destination/$output" ]; then cp "$destination/$output" "$temporary/backup/$output"; fi
  done
  rollback=1
  umask 022
  for spec in "$@"; do
    output=${spec#*=}
    cp -f "$temporary/files/$output" "$destination/$output"
  done
  umask 077
fi

# A failed reload keeps the old validator, so the next run retries deployment.
if [ -n "$reload_command" ]; then /bin/sh -c "$reload_command"; fi
rollback=0
# Retain the current and previous successful snapshots, bounding normal disk use.
if [ "$deployment" = atomic ] && [ -n "$old_snapshot" ]; then
  older=$(readlink "$destination/$current.previous" 2>/dev/null || true)
  case "$older" in
    .certvault-version.*)
      if [ "$older" != "$old_snapshot" ] && [ "$older" != "$(basename "$snapshot")" ]; then rm -rf "$destination/$older"; fi
      ;;
  esac
  ln -s "$old_snapshot" "$temporary/previous"
  mv -Tf "$temporary/previous" "$destination/$current.previous"
fi
if [ -s "$temporary/etag" ]; then
  mv -f "$temporary/etag" "$etag_file"
  chmod 600 "$etag_file"
else
  rm -f "$etag_file"
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
