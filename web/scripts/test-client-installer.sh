#!/bin/sh
set -eu

test_dir=$(mktemp -d)
trap 'rm -rf "$test_dir"' EXIT HUP INT TERM
mkdir -p "$test_dir/bin" "$test_dir/home" "$test_dir/material"
openssl req -x509 -newkey rsa:2048 -nodes -keyout "$test_dir/material/private.key" -out "$test_dir/material/fullchain.crt" -subj /CN=example.com -days 1 >/dev/null 2>&1
cp "$test_dir/material/fullchain.crt" "$test_dir/material/certificate.crt"
cp "$test_dir/material/fullchain.crt" "$test_dir/material/chain.crt"
tar -cf "$test_dir/bundle.tar" -C "$test_dir/material" certificate.crt chain.crt fullchain.crt private.key
export MOCK_BUNDLE="$test_dir/bundle.tar"

cat >"$test_dir/bin/curl" <<'EOF'
#!/bin/sh
set -eu
output=""
headers=""
status_format=""
if_none_match=""
url=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    -H)
      value=$2
      shift 2
      case "$value" in If-None-Match:*) if_none_match=${value#If-None-Match: } ;; esac
      ;;
    -D) headers=$2; shift 2 ;;
    -w) status_format=$2; shift 2 ;;
    -o) output=$2; shift 2 ;;
    -*) shift ;;
    *) url=$1; shift ;;
  esac
done
[ -n "$output" ] && [ -n "$headers" ] && [ -n "$status_format" ] && [ -n "$url" ]
if [ "${MOCK_SLOW-}" = 1 ]; then
  mkdir "$MOCK_CURL_LOG.active" || exit 23
  trap 'rmdir "$MOCK_CURL_LOG.active"' EXIT
  sleep 0.2
fi
artifact=bundle
case "$url" in */bundle.tar?files=*) ;; *) exit 2 ;; esac
etag="\"mock-${MOCK_VERSION:-bundle}\""
if [ "$if_none_match" = "$etag" ]; then
  printf 'HTTP/1.1 304 Not Modified\r\nETag: %s\r\n\r\n' "$etag" >"$headers"
  printf '304'
  printf '%s 304\n' "$artifact" >>"$MOCK_CURL_LOG"
  exit 0
fi
printf 'HTTP/1.1 200 OK\r\nETag: %s\r\n\r\n' "$etag" >"$headers"
cp "$MOCK_BUNDLE" "$output"
printf '200'
printf '%s 200\n' "$artifact" >>"$MOCK_CURL_LOG"
EOF
chmod 700 "$test_dir/bin/curl"

cat >"$test_dir/bin/crontab" <<'EOF'
#!/bin/sh
set -eu
case "${1-}" in
  -l) [ -f "$TEST_CRONTAB" ] && cat "$TEST_CRONTAB" || exit 1 ;;
  -) cat >"$TEST_CRONTAB" ;;
  *) exit 2 ;;
esac
EOF
chmod 700 "$test_dir/bin/crontab"

cat >"$test_dir/bin/chmod" <<'EOF'
#!/bin/sh
set -eu
for argument in "$@"; do
  case "$argument" in
    "${MOCK_CHMOD_UNSUPPORTED_DIR-}"/.certvault.tmp.*/.permission-test)
      [ -n "${MOCK_CHMOD_UNSUPPORTED_DIR-}" ] && exit 1
      ;;
  esac
done
exec /bin/chmod "$@"
EOF
chmod 700 "$test_dir/bin/chmod"

file_mode() {
  stat -Lc '%a' "$1" 2>/dev/null || stat -f '%Lp' "$1"
}

destination="$test_dir/output-one"
schedule="17 3 * * *"
server="https://certvault.example"
token="test-token"
run_installer() {
  PATH="$test_dir/bin:$PATH" \
    HOME="$test_dir/home" \
    TEST_CRONTAB="$test_dir/crontab" \
    MOCK_CURL_LOG="$test_dir/curl.log" \
    CERTVAULT_API_KEY="$token" \
    sh public/client/install.sh \
      --server "$server" \
      --certificate "homelab" \
      --file "fullchain.crt" \
      --file "private.key" \
      --destination "$destination" \
      --schedule "$schedule"
}

run_installer
job_script="$test_dir/home/.local/libexec/certvault-homelab-fullchain.crt-private.key"
PATH="$test_dir/bin:$PATH" \
  MOCK_CURL_LOG="$test_dir/curl.log" \
  "$job_script"

old_destination=$destination
old_server=$server
destination="$test_dir/output-two"
schedule="29 4 * * 1"
server="https://replacement.example"
token="replacement-token"
run_installer

cmp "$destination/fullchain.crt" "$test_dir/material/fullchain.crt"
cmp "$destination/private.key" "$test_dir/material/private.key"
[ "$(file_mode "$destination/fullchain.crt")" = "644" ]
[ "$(file_mode "$destination/private.key")" = "600" ]
[ "$(file_mode "$test_dir/home/.config/certvault/homelab-fullchain.crt-private.key.token")" = "600" ]
[ "$(cat "$test_dir/home/.config/certvault/homelab-fullchain.crt-private.key.token")" = "replacement-token" ]
[ "$(file_mode "$test_dir/home/.config/certvault/homelab-fullchain.crt-private.key.etags/bundle")" = "600" ]
[ "$(cat "$test_dir/home/.config/certvault/homelab-fullchain.crt-private.key.etags/bundle")" = '"mock-bundle"' ]
[ "$(grep -c ' 200$' "$test_dir/curl.log")" = "2" ]
[ "$(grep -c ' 304$' "$test_dir/curl.log")" = "1" ]
[ "$(grep -c '# certvault:homelab-fullchain.crt-private.key' "$test_dir/crontab")" = "1" ]
grep -Fq "29 4 * * 1" "$test_dir/crontab"
grep -Fq "$destination" "$job_script"
grep -Fq "$server" "$job_script"
if grep -Fq "$old_destination" "$job_script"; then
  printf 'Old destination was not replaced in job script\n' >&2
  exit 1
fi
if grep -Fq "$old_server" "$job_script"; then
  printf 'Old server was not replaced in job script\n' >&2
  exit 1
fi

mapped_destination="$test_dir/proxmox"
reload_log="$test_dir/reloads.log"
PATH="$test_dir/bin:$PATH" \
  HOME="$test_dir/home" \
  TEST_CRONTAB="$test_dir/crontab" \
  MOCK_CURL_LOG="$test_dir/curl.log" \
  MOCK_CHMOD_UNSUPPORTED_DIR="$mapped_destination" \
  CERTVAULT_API_KEY="$token" \
  sh public/client/install.sh \
    --server "$server" \
    --certificate "proxmox" \
    --file "fullchain.crt=pveproxy-ssl.pem" \
    --file "private.key=pveproxy-ssl.key" \
    --destination "$mapped_destination" \
    --reload-command "printf 'reloaded\\n' >> '$reload_log'" \
    --schedule "$schedule"

mapped_job_script="$test_dir/home/.local/libexec/certvault-proxmox-fullchain.crt-private.key"
cmp "$mapped_destination/pveproxy-ssl.pem" "$test_dir/material/fullchain.crt"
cmp "$mapped_destination/pveproxy-ssl.key" "$test_dir/material/private.key"
[ "$(file_mode "$mapped_destination/pveproxy-ssl.pem")" = "644" ]
[ "$(file_mode "$mapped_destination/pveproxy-ssl.key")" = "600" ]
[ "$(wc -l <"$reload_log" | tr -d ' ')" = "1" ]

PATH="$test_dir/bin:$PATH" \
  MOCK_CURL_LOG="$test_dir/curl.log" \
  MOCK_CHMOD_UNSUPPORTED_DIR="$mapped_destination" \
  "$mapped_job_script"
[ "$(wc -l <"$reload_log" | tr -d ' ')" = "1" ]

# Both overlapping runs must serialize; the second sees the first run's ETag.
previous_reloads=$(wc -l <"$reload_log")
for run in 1 2; do
  PATH="$test_dir/bin:$PATH" MOCK_CURL_LOG="$test_dir/curl.log" MOCK_CHMOD_UNSUPPORTED_DIR="$mapped_destination" MOCK_VERSION=renewed MOCK_SLOW=1 "$mapped_job_script" &
  if [ "$run" = 1 ]; then first_pid=$!; else second_pid=$!; fi
done
wait "$first_pid"
wait "$second_pid"
[ "$(wc -l <"$reload_log")" -eq "$((previous_reloads+1))" ]

# A mismatched key must leave deployed files and the validator untouched.
mkdir "$test_dir/mismatch"
openssl req -x509 -newkey rsa:2048 -nodes -keyout "$test_dir/mismatch/private.key" -out "$test_dir/mismatch/fullchain.crt" -subj /CN=renewed.example.com -days 1 >/dev/null 2>&1
cp "$test_dir/mismatch/fullchain.crt" "$test_dir/renewed.crt"
cp "$test_dir/material/private.key" "$test_dir/mismatch/private.key"
tar -cf "$test_dir/mismatch.tar" -C "$test_dir/mismatch" fullchain.crt private.key
if PATH="$test_dir/bin:$PATH" MOCK_CURL_LOG="$test_dir/curl.log" MOCK_BUNDLE="$test_dir/mismatch.tar" MOCK_VERSION=mismatch "$mapped_job_script"; then
  printf 'Mismatched certificate/key deployed\n' >&2
  exit 1
fi
cmp "$mapped_destination/pveproxy-ssl.pem" "$test_dir/material/fullchain.crt"
[ "$(cat "$test_dir/home/.config/certvault/proxmox-fullchain.crt-private.key.etags/bundle")" = '"mock-renewed"' ]

# A reload failure must not acknowledge the new bundle; the next run retries.
retry_marker="$test_dir/reload-ready"
if PATH="$test_dir/bin:$PATH" HOME="$test_dir/home" TEST_CRONTAB="$test_dir/crontab" MOCK_CURL_LOG="$test_dir/curl.log" CERTVAULT_API_KEY="$token" sh public/client/install.sh \
  --server "$server" --certificate retry --file fullchain.crt --file private.key \
  --destination "$test_dir/retry" --reload-command "test -f '$retry_marker'"; then
  printf 'Reload failure was ignored\n' >&2
  exit 1
fi
[ ! -f "$test_dir/home/.config/certvault/retry-fullchain.crt-private.key.etags/bundle" ]
touch "$retry_marker"
PATH="$test_dir/bin:$PATH" MOCK_CURL_LOG="$test_dir/curl.log" "$test_dir/home/.local/libexec/certvault-retry-fullchain.crt-private.key"
[ -f "$test_dir/home/.config/certvault/retry-fullchain.crt-private.key.etags/bundle" ]
[ -L "$test_dir/retry/fullchain.crt" ]
[ -L "$test_dir/retry/private.key" ]

# Interrupt a fixed-file deployment after the certificate copy; restore both.
cat >"$test_dir/bin/cp" <<'SCRIPT'
#!/bin/sh
set -eu
case "$*" in
  *"/files/pveproxy-ssl.key"*)
    if [ "${MOCK_COPY_FAIL-}" = 1 ]; then exit 24; fi
    ;;
esac
exec /bin/cp "$@"
SCRIPT
chmod 700 "$test_dir/bin/cp"
# This fixture is internally consistent but differs from the deployed pair.
openssl req -x509 -newkey rsa:2048 -nodes -keyout "$test_dir/mismatch/private.key" -out "$test_dir/mismatch/fullchain.crt" -subj /CN=new.example.com -days 1 >/dev/null 2>&1
tar -cf "$test_dir/new.tar" -C "$test_dir/mismatch" fullchain.crt private.key
if PATH="$test_dir/bin:$PATH" MOCK_CURL_LOG="$test_dir/curl.log" MOCK_BUNDLE="$test_dir/new.tar" MOCK_VERSION=new MOCK_COPY_FAIL=1 MOCK_CHMOD_UNSUPPORTED_DIR="$mapped_destination" "$mapped_job_script"; then
  printf 'Copy failure was ignored\n' >&2
  exit 1
fi
cmp "$mapped_destination/pveproxy-ssl.pem" "$test_dir/material/fullchain.crt"
cmp "$mapped_destination/pveproxy-ssl.key" "$test_dir/material/private.key"
[ "$(cat "$test_dir/home/.config/certvault/proxmox-fullchain.crt-private.key.etags/bundle")" = '"mock-renewed"' ]
PATH="$test_dir/bin:$PATH" MOCK_CURL_LOG="$test_dir/curl.log" MOCK_BUNDLE="$test_dir/new.tar" MOCK_VERSION=new MOCK_CHMOD_UNSUPPORTED_DIR="$mapped_destination" "$mapped_job_script"
cmp "$mapped_destination/pveproxy-ssl.pem" "$test_dir/mismatch/fullchain.crt"
cmp "$mapped_destination/pveproxy-ssl.key" "$test_dir/mismatch/private.key"
