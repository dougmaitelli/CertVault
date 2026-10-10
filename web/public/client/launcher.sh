#!/bin/sh
# certvault-client-protocol: 1
set -eu
client_dir=$1
update_config=$2
shift 2
server=$2
if ! "$client_dir/current/update.sh" "$client_dir" "$update_config" "$server"; then
  printf 'CertVault client update failed; continuing with the installed client.\n' >&2
fi
# Updates take an exclusive lock before switching and pruning versions.
# Concurrent sync jobs share this lock and retain their normal destination locks.
exec 7>"$client_dir/run.lock"
flock -s 7
exec "$client_dir/current/sync.sh" "$@"
