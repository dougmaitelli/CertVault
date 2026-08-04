#!/bin/sh
set -eu

test_dir=$(mktemp -d)
trap 'rm -rf "$test_dir"' EXIT HUP INT TERM
mkdir -p "$test_dir/bin" "$test_dir/home"

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
artifact=${url##*/}
etag="\"mock-$artifact\""
if [ "$if_none_match" = "$etag" ]; then
  printf 'HTTP/1.1 304 Not Modified\r\nETag: %s\r\n\r\n' "$etag" >"$headers"
  printf '304'
  printf '%s 304\n' "$artifact" >>"$MOCK_CURL_LOG"
  exit 0
fi
printf 'HTTP/1.1 200 OK\r\nETag: %s\r\n\r\n' "$etag" >"$headers"
printf '%s\n' 'mock certificate material' >"$output"
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

file_mode() {
  stat -c '%a' "$1" 2>/dev/null || stat -f '%Lp' "$1"
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
      --files "fullchain.pem,private-key.pem" \
      --destination "$destination" \
      --schedule "$schedule"
}

run_installer
job_script="$test_dir/home/.local/libexec/certvault-homelab-fullchain.pem-private-key.pem"
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

[ "$(cat "$destination/fullchain.pem")" = "mock certificate material" ]
[ "$(cat "$destination/private-key.pem")" = "mock certificate material" ]
[ "$(file_mode "$destination/fullchain.pem")" = "644" ]
[ "$(file_mode "$destination/private-key.pem")" = "600" ]
[ "$(file_mode "$test_dir/home/.config/certvault/homelab-fullchain.pem-private-key.pem.token")" = "600" ]
[ "$(cat "$test_dir/home/.config/certvault/homelab-fullchain.pem-private-key.pem.token")" = "replacement-token" ]
[ "$(file_mode "$test_dir/home/.config/certvault/homelab-fullchain.pem-private-key.pem.etags/fullchain.pem")" = "600" ]
[ "$(cat "$test_dir/home/.config/certvault/homelab-fullchain.pem-private-key.pem.etags/fullchain.pem")" = '"mock-fullchain.pem"' ]
[ "$(grep -c ' 200$' "$test_dir/curl.log")" = "4" ]
[ "$(grep -c ' 304$' "$test_dir/curl.log")" = "2" ]
[ "$(grep -c '# certvault:homelab-fullchain.pem-private-key.pem' "$test_dir/crontab")" = "1" ]
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
