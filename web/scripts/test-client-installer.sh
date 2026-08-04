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

destination="$test_dir/output"
run_installer() {
  PATH="$test_dir/bin:$PATH" \
    HOME="$test_dir/home" \
    TEST_CRONTAB="$test_dir/crontab" \
    MOCK_CURL_LOG="$test_dir/curl.log" \
    CERTVAULT_API_KEY="test-token" \
    sh public/client/install.sh \
      --server "https://certvault.example" \
      --certificate "homelab" \
      --files "fullchain.pem,private-key.pem" \
      --destination "$destination" \
      --schedule "17 3 * * *"
}

run_installer
run_installer

[ "$(cat "$destination/fullchain.pem")" = "mock certificate material" ]
[ "$(cat "$destination/private-key.pem")" = "mock certificate material" ]
[ "$(stat -c '%a' "$destination/fullchain.pem")" = "644" ]
[ "$(stat -c '%a' "$destination/private-key.pem")" = "600" ]
[ "$(stat -c '%a' "$test_dir/home/.config/certvault/homelab-fullchain.pem-private-key.pem.token")" = "600" ]
[ "$(stat -c '%a' "$test_dir/home/.config/certvault/homelab-fullchain.pem-private-key.pem.etags/fullchain.pem")" = "600" ]
[ "$(cat "$test_dir/home/.config/certvault/homelab-fullchain.pem-private-key.pem.etags/fullchain.pem")" = '"mock-fullchain.pem"' ]
[ "$(grep -c ' 200$' "$test_dir/curl.log")" = "2" ]
[ "$(grep -c ' 304$' "$test_dir/curl.log")" = "2" ]
[ "$(grep -c '# certvault:homelab-fullchain.pem-private-key.pem' "$test_dir/crontab")" = "1" ]
