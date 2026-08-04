#!/bin/sh
set -eu

test_dir=$(mktemp -d)
trap 'rm -rf "$test_dir"' EXIT HUP INT TERM
mkdir -p "$test_dir/bin" "$test_dir/home"

cat >"$test_dir/bin/curl" <<'EOF'
#!/bin/sh
set -eu
output=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    -H|-o) option=$1; value=$2; shift 2; [ "$option" != "-o" ] || output=$value ;;
    *) shift ;;
  esac
done
[ -n "$output" ]
printf '%s\n' 'mock certificate material' >"$output"
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

output="$test_dir/output/private-key.pem"
run_installer() {
  PATH="$test_dir/bin:$PATH" \
    HOME="$test_dir/home" \
    TEST_CRONTAB="$test_dir/crontab" \
    CERTVAULT_API_KEY="test-token" \
    sh public/client/install.sh \
      --server "https://certvault.example" \
      --certificate "homelab" \
      --artifact "private-key.pem" \
      --output "$output" \
      --schedule "17 3 * * *"
}

run_installer
run_installer

[ "$(cat "$output")" = "mock certificate material" ]
[ "$(stat -c '%a' "$output")" = "600" ]
[ "$(stat -c '%a' "$test_dir/home/.config/certvault/homelab-private-key.pem.token")" = "600" ]
[ "$(grep -c '# certvault:homelab-private-key.pem' "$test_dir/crontab")" = "1" ]
