#!/bin/sh
# certvault-client-protocol: 1
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
status=$(curl --connect-timeout 10 --max-time 120 -fsSL -D "$temporary/headers" -w '%{http_code}' \
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

