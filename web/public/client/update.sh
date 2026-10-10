#!/bin/sh
# certvault-client-protocol: 1
set -eu
umask 077
client_dir=$1
update_config=$2
server=${3%/}
case "$server" in https://*) ;; *) printf 'Client updates require HTTPS\n' >&2; exit 1 ;; esac
exec 8>"$client_dir/update.lock"
flock -x 8
now=$(date +%s)
next_check=$(cat "$update_config/next-check" 2>/dev/null || printf '0')
case "$next_check" in ''|*[!0-9]*) next_check=0 ;; esac
if [ "$now" -lt "$next_check" ]; then exit 0; fi
# Persist a one-hour failure backoff before network work, including interruptions.
check_file=$(mktemp "$update_config/.next-check.XXXXXX")
printf '%s\n' "$((now+3600))" >"$check_file"
mv -f "$check_file" "$update_config/next-check"
temporary=$(mktemp -d "$client_dir/.update.XXXXXX")
candidate=""
cleanup() {
  [ -z "$candidate" ] || rm -rf "$candidate"
  rm -rf "$temporary"
}
trap cleanup EXIT
trap 'exit 1' HUP INT TERM
curl --proto '=https' --proto-redir '=https' --connect-timeout 5 --max-time 20 --max-filesize 4096 -fsSL \
  "$server/client/update/manifest" -o "$temporary/manifest"
[ "$(wc -l <"$temporary/manifest")" -eq 4 ]
[ "$(sed -n '1p' "$temporary/manifest")" = 'protocol 1' ]
revision=$(sed -n '2s/^sha256 //p' "$temporary/manifest")
size=$(sed -n '3s/^size //p' "$temporary/manifest")
signature=$(sed -n '4s/^signature //p' "$temporary/manifest")
case "$revision" in ''|*[!0-9a-f]*) exit 1 ;; esac
[ "${#revision}" -eq 64 ]
case "$size" in ''|*[!0-9]*) exit 1 ;; esac
[ "${#size}" -le 7 ] && [ "$size" -gt 0 ] && [ "$size" -le 2097152 ]
[ -n "$signature" ]
head -n 3 "$temporary/manifest" >"$temporary/payload"
printf '%s' "$signature" | openssl base64 -d -A >"$temporary/signature"
openssl dgst -sha256 -verify "$update_config/public.pem" -signature "$temporary/signature" "$temporary/payload" >/dev/null
current_revision=$(cat "$client_dir/current/revision" 2>/dev/null || true)
if [ "$current_revision" != "$revision" ]; then
  curl --proto '=https' --proto-redir '=https' --connect-timeout 5 --max-time 30 --max-filesize "$size" -fsSL \
    "$server/client/update/$revision.tar" -o "$temporary/client.tar"
  [ "$(wc -c <"$temporary/client.tar")" -eq "$size" ]
  [ "$(openssl dgst -sha256 "$temporary/client.tar" | awk '{print $NF}')" = "$revision" ]
  [ "$(tar -tf "$temporary/client.tar")" = "$(printf 'launcher.sh\nsync.sh\nupdate.sh')" ]
  # Only regular entries may be extracted; reject links and unexpected members.
  tar -tvf "$temporary/client.tar" | awk 'substr($1,1,1)!="-" {bad=1} END {exit bad || NR!=3}'
  candidate=$(mktemp -d "$client_dir/version-$revision.XXXXXX")
  for file in launcher.sh sync.sh update.sh; do
    tar -xOf "$temporary/client.tar" "$file" >"$candidate/$file"
    grep -Fx '# certvault-client-protocol: 1' "$candidate/$file" >/dev/null
    sh -n "$candidate/$file"
    chmod 700 "$candidate/$file"
  done
  printf '%s\n' "$revision" >"$candidate/revision"
  exec 7>"$client_dir/run.lock"
  # A busy sync should delay an update, not indefinitely hold up another job.
  flock -x -w 5 7
  previous=$(readlink "$client_dir/current")
  case "$previous" in version-* ) ;; *) exit 1 ;; esac
  case "$previous" in */*) exit 1 ;; esac
  ln -s "${candidate##*/}" "$temporary/current"
  ln -s "$previous" "$temporary/previous"
  mv -Tf "$temporary/current" "$client_dir/current"
  candidate=""
  mv -Tf "$temporary/previous" "$client_dir/previous"
  current=$(readlink "$client_dir/current")
  for version in "$client_dir"/version-*; do
    [ -d "$version" ] || continue
    case "${version##*/}" in "$current"|"$previous") continue ;; esac
    rm -rf "$version"
  done
  printf 'Updated CertVault client to %s\n' "$revision"
fi
check_file=$(mktemp "$update_config/.next-check.XXXXXX")
printf '%s\n' "$((now+86400))" >"$check_file"
mv -f "$check_file" "$update_config/next-check"
