#!/bin/sh
# Pinned one-shot private-layer planner. Requirements: private-capture ownership
# and storage schema section 3.5/4; no host output, invented EOF or child replay.
set -eu
umask 077
die() { exit 90; }
trusted_parent() {
  [ ! -L "$1" ] && [ -d "$1" ] || return 1
  [ "$(stat -c '%u' "$1")" = 0 ] || return 1
  mode=$(stat -c '%a' "$1") || return 1
  case "$mode" in ''|*[!0-7]*) return 1 ;; esac
  [ "$((0$mode & 0022))" -eq 0 ]
}
[ "$(id -u)" = 0 ] || die
[ "$#" -ge 1 ] || die
[ "${SQLRS_CAPTURE_DIR:-}" = /sqlrs/capture ] || die
[ "${SQLRS_JOB_ID:-}" != '' ] || die
[ "${SQLRS_TASK_ID:-}" = plan ] || die
[ "${SQLRS_ATTEMPT_NO:-}" != '' ] || die
[ "${SQLRS_EXECUTION_DIGEST:-}" != '' ] || die
[ "${SQLRS_HELPER_DIGEST:-}" != '' ] || die
[ "${SQLRS_CAPTURE_MAX_BYTES:-}" != '' ] || die
case "$SQLRS_JOB_ID" in *[!a-zA-Z0-9_.-]*|[!a-zA-Z0-9]*) die ;; esac
[ "${#SQLRS_JOB_ID}" -le 128 ] || die
case "$SQLRS_ATTEMPT_NO" in *[!0-9]*|0*|'') die ;; esac
[ "${#SQLRS_ATTEMPT_NO}" -le 19 ] || die
for value in "$SQLRS_EXECUTION_DIGEST" "$SQLRS_HELPER_DIGEST"; do
  case "$value" in sha256:*) ;; *) die ;; esac
  hex=${value#sha256:}
  [ "${#hex}" -eq 64 ] || die
  case "$hex" in *[!0-9a-f]*) die ;; esac
done
case "$SQLRS_CAPTURE_MAX_BYTES" in *[!0-9]*|'') die ;; esac
[ "${#SQLRS_CAPTURE_MAX_BYTES}" -le 8 ] || die
[ "$SQLRS_CAPTURE_MAX_BYTES" -ge 1 ] && [ "$SQLRS_CAPTURE_MAX_BYTES" -le 67108864 ] || die
self_hash=$(sha256sum "$0") || die
self_hash=${self_hash%% *}
[ "sha256:$self_hash" = "$SQLRS_HELPER_DIGEST" ] || die

# Initialization is exclusive and precedes the child. Never repair permissions,
# follow links or reuse an existing root (even one lacking a completion marker).
trusted_parent / || die
[ -e /sqlrs ] || [ -L /sqlrs ] || mkdir -m 755 /sqlrs || die
trusted_parent /sqlrs || die
[ ! -e /sqlrs/capture ] && [ ! -L /sqlrs/capture ] || die
mkdir -m 700 /sqlrs/capture || die
capture=/sqlrs/capture
stdout_fifo=$capture/stdout.pipe
stderr_fifo=$capture/stderr.pipe
stdout_partial=$capture/stdout.partial
stderr_partial=$capture/stderr.partial
manifest_partial=$capture/manifest.partial
mkfifo "$stdout_fifo" "$stderr_fifo" || die
capture_blocks=$(( (SQLRS_CAPTURE_MAX_BYTES + 511) / 512 ))

# Exact-count writes keep partial files within quota. The separate EOF probe
# consumes no retained byte and cannot turn truncation into successful capture.
capture_stream() (
  ulimit -f "$capture_blocks" || exit 1
  exec 3< "$1"
  dd bs=65536 count="$SQLRS_CAPTURE_MAX_BYTES" iflag=count_bytes,fullblock status=none <&3 > "$2" 2>/dev/null || exit 1
  extra=$(dd bs=1 count=1 status=none <&3 2>/dev/null | wc -c) || exit 1
  [ "$extra" -eq 0 ]
)
capture_stream "$stdout_fifo" "$stdout_partial" &
stdout_reader=$!
capture_stream "$stderr_fifo" "$stderr_partial" &
stderr_reader=$!
set +e
setpriv --reuid=1001 --regid=1001 --clear-groups --no-new-privs "$@" > "$stdout_fifo" 2> "$stderr_fifo"
child_exit=$?
wait "$stdout_reader"
stdout_status=$?
wait "$stderr_reader"
stderr_status=$?
set -e
[ "$stdout_status" -eq 0 ] && [ "$stderr_status" -eq 0 ] || die
rm "$stdout_fifo" "$stderr_fifo" || die
# O4: one filesystem flush at the same stream-completion phase boundary.
sync -f "$capture" || die
stdout_hash=$(sha256sum "$stdout_partial") || die
stdout_hash=${stdout_hash%% *}
stderr_hash=$(sha256sum "$stderr_partial") || die
stderr_hash=${stderr_hash%% *}
stdout_bytes=$(wc -c < "$stdout_partial") || die
stderr_bytes=$(wc -c < "$stderr_partial") || die
stdout_bytes=$(printf '%s' "$stdout_bytes" | tr -d ' ')
stderr_bytes=$(printf '%s' "$stderr_bytes" | tr -d ' ')
[ "$stdout_bytes" -le "$SQLRS_CAPTURE_MAX_BYTES" ] && [ "$stderr_bytes" -le "$SQLRS_CAPTURE_MAX_BYTES" ] || die
mv "$stdout_partial" "$capture/stdout.complete" || die
mv "$stderr_partial" "$capture/stderr.complete" || die
sync -f "$capture" || die
printf '{"protocol":"lb-capture-v2","job_id":"%s","task_id":"plan","attempt_no":%s,"execution_digest":"%s","helper_digest":"%s","child_exit_code":%s,"stdout_eof":true,"stderr_eof":true,"stdout_bytes":%s,"stdout_sha256":"%s","stderr_bytes":%s,"stderr_sha256":"%s"}\n' \
  "$SQLRS_JOB_ID" "$SQLRS_ATTEMPT_NO" "$SQLRS_EXECUTION_DIGEST" "$SQLRS_HELPER_DIGEST" \
  "$child_exit" "$stdout_bytes" "$stdout_hash" "$stderr_bytes" "$stderr_hash" > "$manifest_partial" || die
[ "$(wc -c < "$manifest_partial")" -le 8192 ] || die
sync -f "$manifest_partial" || die
mv "$manifest_partial" "$capture/manifest.complete" || die
sync -f "$capture" || die
exit "$child_exit"
