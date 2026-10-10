#!/bin/sh
set -eu
test_dir=$(mktemp -d)
trap 'rm -rf "$test_dir"' EXIT HUP INT TERM
mkdir -p "$test_dir/bin" "$test_dir/home" "$test_dir/certificates"
openssl genpkey -algorithm EC -pkeyopt ec_paramgen_curve:P-256 -out "$test_dir/signing.pem" >/dev/null 2>&1
openssl pkey -in "$test_dir/signing.pem" -pubout -out "$test_dir/public.pem" >/dev/null 2>&1
openssl req -x509 -newkey rsa:2048 -nodes -keyout "$test_dir/certificates/private.key" -out "$test_dir/certificates/fullchain.crt" -subj /CN=example.test -days 1 >/dev/null 2>&1
tar -cf "$test_dir/certificates.tar" -C "$test_dir/certificates" fullchain.crt private.key

release() {
  directory=$1
  label=$2
  protocol=${3:-1}
  mkdir -p "$directory/files"
  cp public/client/launcher.sh public/client/update.sh "$directory/files/"
  {
    head -n 3 public/client/sync.sh
    printf 'printf "%%s\\n" "%s" >>"$CLIENT_EXEC_LOG"\n' "$label"
    tail -n +4 public/client/sync.sh
  } >"$directory/files/sync.sh"
  chmod 700 "$directory/files/"*.sh
  sign_release "$directory" "$protocol"
}
sign_release() {
  directory=$1
  protocol=$2
  tar -cf "$directory/client.tar" -C "$directory/files" launcher.sh sync.sh update.sh
  revision=$(openssl dgst -sha256 "$directory/client.tar" | awk '{print $NF}')
  {
    printf 'protocol %s\nsha256 %s\nsize %s\n' "$protocol" "$revision" "$(wc -c <"$directory/client.tar")"
  } >"$directory/payload"
  openssl dgst -sha256 -sign "$test_dir/signing.pem" -out "$directory/signature" "$directory/payload"
  cat "$directory/payload" >"$directory/manifest"
  printf 'signature %s\n' "$(openssl base64 -A -in "$directory/signature")" >>"$directory/manifest"
}
release "$test_dir/one" one
release "$test_dir/two" two
export MOCK_UPDATE_SOURCE="$test_dir/one"
export MOCK_UPDATE_PUBLIC_KEY="$test_dir/public.pem"
export MOCK_CERTIFICATE_BUNDLE="$test_dir/certificates.tar"
export MOCK_CURL_LOG="$test_dir/curl.log"
export CLIENT_EXEC_LOG="$test_dir/executed.log"
export TEST_CRONTAB="$test_dir/crontab"
export CERTVAULT_API_KEY=test-token
export HOME="$test_dir/home"
export PATH="$test_dir/bin:$PATH"
cat >"$test_dir/bin/curl" <<'SCRIPT'
#!/bin/sh
set -eu
output=""
headers=""
etag=""
url=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    -o) output=$2; shift 2 ;;
    -D) headers=$2; shift 2 ;;
    -H) case "$2" in If-None-Match:*) etag=${2#If-None-Match: } ;; esac; shift 2 ;;
    --proto|--proto-redir|--connect-timeout|--max-time|--max-filesize|-w) shift 2 ;;
    -*) shift ;;
    *) url=$1; shift ;;
  esac
done
printf '%s\n' "$url" >>"$MOCK_CURL_LOG"
case "$url" in
  */client/update/key.pem) cp "$MOCK_UPDATE_PUBLIC_KEY" "$output" ;;
  */client/update/manifest)
    [ "${MOCK_UPDATE_FAILURE-}" != offline ] || exit 28
    cp "$MOCK_UPDATE_SOURCE/manifest" "$output"
    if [ "${MOCK_UPDATE_FAILURE-}" = signature ]; then sed -i 's/^size .*/size 1/' "$output"; fi
    ;;
  */client/update/*.tar)
    [ "${MOCK_UPDATE_DELAY-}" != 1 ] || sleep 0.2
    case "${MOCK_UPDATE_FAILURE-}" in
      interrupt) kill -TERM "$PPID"; exit 28 ;;
      partial) head -c 64 "$MOCK_UPDATE_SOURCE/client.tar" >"$output" ;;
      digest) cp "$MOCK_UPDATE_SOURCE/client.tar" "$output"; printf x | dd of="$output" bs=1 seek=200 conv=notrunc 2>/dev/null ;;
      *) cp "$MOCK_UPDATE_SOURCE/client.tar" "$output" ;;
    esac
    ;;
  */bundle.tar?files=*)
    if [ -n "$etag" ]; then
      printf 'HTTP/1.1 304 Not Modified\r\nETag: "certificates"\r\n\r\n' >"$headers"
      printf 304
    else
      cp "$MOCK_CERTIFICATE_BUNDLE" "$output"
      printf 'HTTP/1.1 200 OK\r\nETag: "certificates"\r\n\r\n' >"$headers"
      printf 200
    fi
    ;;
  *) exit 2 ;;
esac
SCRIPT
chmod 700 "$test_dir/bin/curl"
cat >"$test_dir/bin/crontab" <<'SCRIPT'
#!/bin/sh
set -eu
case "${1-}" in
  -l) [ -f "$TEST_CRONTAB" ] && cat "$TEST_CRONTAB" || exit 1 ;;
  -) cat >"$TEST_CRONTAB" ;;
  *) exit 2 ;;
esac
SCRIPT
chmod 700 "$test_dir/bin/crontab"
install_job() {
  sh public/client/install.sh --server https://certvault.example --certificate "$1" \
    --file fullchain.crt --file private.key --destination "$test_dir/$1" --auto-update
}
install_job first
install_job second
first="$HOME/.local/libexec/certvault-first-fullchain.crt-private.key"
second="$HOME/.local/libexec/certvault-second-fullchain.crt-private.key"
origin_id=$(printf '%s' https://certvault.example | openssl dgst -sha256 | awk '{print $NF}')
client_dir="$HOME/.local/libexec/certvault-clients/$origin_id"
update_config="$HOME/.config/certvault/client-updates/$origin_id"
[ "$(grep -c '# certvault:' "$TEST_CRONTAB")" -eq 2 ]
[ "$(tail -n 1 "$CLIENT_EXEC_LOG")" = one ]
[ "$(grep -c '/client/update/.*\.tar' "$MOCK_CURL_LOG")" -eq 1 ]
[ "$(stat -c %a "$update_config/public.pem")" = 600 ]
[ "$(stat -c %a "$client_dir/current/sync.sh")" = 700 ]
# An unchanged daily revision performs no download and no client reload.
before=$(grep -c '/client/update/.*\.tar' "$MOCK_CURL_LOG")
printf '0\n' >"$update_config/next-check"
"$first"
[ "$(grep -c '/client/update/.*\.tar' "$MOCK_CURL_LOG")" -eq "$before" ]
# Concurrent jobs share one successful update check and use the new client now.
export MOCK_UPDATE_SOURCE="$test_dir/two"
export MOCK_UPDATE_DELAY=1
printf '0\n' >"$update_config/next-check"
"$first" &
first_pid=$!
"$second" &
second_pid=$!
wait "$first_pid"
wait "$second_pid"
unset MOCK_UPDATE_DELAY
[ "$(tail -n 2 "$CLIENT_EXEC_LOG" | grep -c '^two$')" -eq 2 ]
[ "$(grep -c '/client/update/.*\.tar' "$MOCK_CURL_LOG")" -eq "$((before+1))" ]
[ -L "$client_dir/previous" ]
[ "$(find "$client_dir" -maxdepth 1 -type d -name 'version-*' | wc -l)" -eq 2 ]
# Corrupt, partial, unsigned, and unavailable updates preserve the working client.
release "$test_dir/three" three
export MOCK_UPDATE_SOURCE="$test_dir/three"
for failure in offline signature partial digest interrupt; do
  export MOCK_UPDATE_FAILURE="$failure"
  printf '0\n' >"$update_config/next-check"
  previous=$(readlink "$client_dir/current")
  "$first"
  [ "$(readlink "$client_dir/current")" = "$previous" ]
  [ "$(tail -n 1 "$CLIENT_EXEC_LOG")" = two ]
  # Failure backoff is shared and does not cause every certificate job to retry.
  requests=$(wc -l <"$MOCK_CURL_LOG")
  "$second"
  [ "$(wc -l <"$MOCK_CURL_LOG")" -eq "$((requests+1))" ]
done
unset MOCK_UPDATE_FAILURE
release "$test_dir/unsupported" incompatible 2
export MOCK_UPDATE_SOURCE="$test_dir/unsupported"
printf '0\n' >"$update_config/next-check"
"$first"
[ "$(tail -n 1 "$CLIENT_EXEC_LOG")" = two ]
release "$test_dir/syntax" malformed
printf '\nif\n' >>"$test_dir/syntax/files/sync.sh"
sign_release "$test_dir/syntax" 1
export MOCK_UPDATE_SOURCE="$test_dir/syntax"
printf '0\n' >"$update_config/next-check"
"$first"
[ "$(tail -n 1 "$CLIENT_EXEC_LOG")" = two ]
# Even a signed archive cannot replace a script with a symbolic link.
release "$test_dir/link" linked
rm "$test_dir/link/files/launcher.sh"
ln -s update.sh "$test_dir/link/files/launcher.sh"
sign_release "$test_dir/link" 1
export MOCK_UPDATE_SOURCE="$test_dir/link"
printf '0\n' >"$update_config/next-check"
"$first"
[ "$(tail -n 1 "$CLIENT_EXEC_LOG")" = two ]
# A running sync holds the shared run lock; updating times out safely.
export MOCK_UPDATE_SOURCE="$test_dir/three"
printf '0\n' >"$update_config/next-check"
exec 6>"$client_dir/run.lock"
flock -s 6
"$first"
[ "$(tail -n 1 "$CLIENT_EXEC_LOG")" = two ]
exec 6>&-
# The next permitted check recovers and retains just current and previous.
printf '0\n' >"$update_config/next-check"
"$first"
[ "$(tail -n 1 "$CLIENT_EXEC_LOG")" = three ]
[ "$(find "$client_dir" -maxdepth 1 -type d -name 'version-*' | wc -l)" -eq 2 ]
[ "$(find "$client_dir" -maxdepth 1 -type d -name '.update.*' | wc -l)" -eq 0 ]
# Replacing the server key never replaces the installed pin automatically.
cp "$update_config/public.pem" "$test_dir/pin-before"
openssl genpkey -algorithm EC -pkeyopt ec_paramgen_curve:P-256 -out "$test_dir/signing.pem" >/dev/null 2>&1
openssl pkey -in "$test_dir/signing.pem" -pubout -out "$test_dir/public.pem" >/dev/null 2>&1
release "$test_dir/rotated" rotated
export MOCK_UPDATE_SOURCE="$test_dir/rotated"
install_job second
cmp "$test_dir/pin-before" "$update_config/public.pem"
[ "$(tail -n 1 "$CLIENT_EXEC_LOG")" = three ]
# Explicit opt-out uses the embedded sync and preserves the same cron entry.
sh public/client/install.sh --server https://certvault.example --certificate second \
  --file fullchain.crt --file private.key --destination "$test_dir/second"
[ "$(grep -c '# certvault:' "$TEST_CRONTAB")" -eq 2 ]
if grep -q 'launcher.sh' "$second"; then printf 'Opt-out retained auto-update\n' >&2; exit 1; fi
if sh public/client/install.sh --server http://certvault.example --certificate rejected \
  --file fullchain.crt --destination "$test_dir/rejected" --auto-update; then
  printf 'Auto-update accepted an insecure origin\n' >&2
  exit 1
fi
printf 'Client updater tests passed\n'
