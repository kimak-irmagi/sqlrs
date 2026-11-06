#!/bin/sh
# Pinned private-layer session initialization. Requirements: private-capture
# ownership section 3.1 and storage schema section 4. serve never runs SQL;
# a child requires an existing exact ready marker and an exclusive invocation.
set -eu
umask 077
die() { exit 90; }
token() {
  case "$1" in ''|*[!a-zA-Z0-9_.-]*|[!a-zA-Z0-9]*) return 1 ;; esac
  [ "${#1}" -le 128 ]
}
digest() {
  case "$1" in sha256:*) ;; *) return 1 ;; esac
  hex=${1#sha256:}
  [ "${#hex}" -eq 64 ] || return 1
  case "$hex" in *[!0-9a-f]*) return 1 ;; esac
}
trusted_parent() {
  [ ! -L "$1" ] && [ -d "$1" ] || return 1
  owner=$(stat -c '%u' "$1") || return 1
  mode=$(stat -c '%a' "$1") || return 1
  [ "$owner" = 0 ] || return 1
  # stat prints octal permission digits, including possible special bits.
  case "$mode" in *[!0-7]*|'') return 1 ;; esac
  [ "$((0$mode & 0022))" -eq 0 ]
}
private_root() {
  trusted_parent / || return 1
  [ -e /sqlrs ] || [ -L /sqlrs ] || mkdir -m 755 /sqlrs || return 1
  trusted_parent /sqlrs || return 1
  [ -e /sqlrs/capture ] || [ -L /sqlrs/capture ] || mkdir -m 700 /sqlrs/capture || return 1
  [ ! -L /sqlrs/capture ] && [ -d /sqlrs/capture ] || return 1
  [ "$(stat -c '%u:%a' /sqlrs/capture)" = '0:700' ]
}
# Closed JSON decoding stays inside the pinned helper; it does not start a JVM,
# eval caller text or invoke a second Docker command. All emitted bindings are
# validated ASCII tokens. Decoded-key comparison rejects escaped duplicates.
json_bindings() {
  awk -v schema="$1" '
  function ws() { while (substr(input,p,1) ~ /^[ \t\r\n]$/) p++ }
  function quoted( c,e,h,n,i,k) {
    if (substr(input,p++,1)!="\"") { bad=1; return "" }
    result=""
    while (p<=length(input)) {
      c=substr(input,p++,1)
      if (c=="\"") return result
      if (c ~ /^[\001-\037]$/) { bad=1; return "" }
      if (c=="\\") {
        e=substr(input,p++,1)
        if (e=="u") {
          h=substr(input,p,4); p+=4
          if (length(h)!=4 || h ~ /[^0-9a-fA-F]/) { bad=1; return "" }
          n=0
          for(i=1;i<=4;i++) { k=index("0123456789abcdef",tolower(substr(h,i,1)))-1; n=n*16+k }
          # Engine identities are ASCII. Non-ASCII/surrogate/control escapes
          # cannot belong to any approved key or binding in either schema.
          if(n<32 || n>126) { bad=1; return "" }
          c=sprintf("%c",n)
        } else if(e=="\"" || e=="\\" || e=="/") c=e
        else { bad=1; return "" }
      }
      result=result c
    }
    bad=1; return ""
  }
  function token(v) { return length(v)>0 && length(v)<=128 && v ~ /^[a-zA-Z0-9][a-zA-Z0-9_.-]*$/ }
  function hex(v) { return length(v)==64 && v !~ /[^0-9a-f]/ }
  function digest(v) { return substr(v,1,7)=="sha256:" && hex(substr(v,8)) }
  function positive_int64(v) { return v!="0" && (length(v)<19 || (length(v)==19 && ("x" v)<"x9223372036854775808")) }
  { input=input $0 "\n" }
  END {
    if(schema=="prune") {
      names="protocol retirement_ref session_id worker_generation runner_id prior_invocation_ref prior_manifest_digest next_invocation_ref helper_digest"
      nums="worker_generation"; bools=""
    } else if(schema=="step") {
      names="protocol job_id task_id session_id invocation_ref attempt_no worker_generation runner_id execution_digest helper_digest child_exit_code stdout_eof stderr_eof stdout_bytes stdout_sha256 stderr_bytes stderr_sha256 prior_retirement_ref"
      nums="attempt_no worker_generation child_exit_code stdout_bytes stderr_bytes"; bools="stdout_eof stderr_eof"
    } else exit 1
    count=split(names,a," "); for(i=1;i<=count;i++) allowed[a[i]]=1
    n=split(nums,a," "); for(i=1;i<=n;i++) number[a[i]]=1
    n=split(bools,a," "); for(i=1;i<=n;i++) boolean[a[i]]=1
    p=1; ws(); if(substr(input,p++,1)!="{") exit 1
    ws(); seen_count=0
    while(p<=length(input)) {
      k=quoted(); if(bad || !allowed[k] || seen[k]++) exit 1
      seen_count++; ws(); if(substr(input,p++,1)!=":") exit 1; ws()
      if(substr(input,p,1)=="\"") { v=quoted(); typ[k]="string" }
      else if(substr(input,p,4)=="null") { v="null"; p+=4; typ[k]="null" }
      else if(substr(input,p,4)=="true") { v="true"; p+=4; typ[k]="bool" }
      else if(substr(input,p,5)=="false") { v="false"; p+=5; typ[k]="bool" }
      else if(match(substr(input,p),/^(0|[1-9][0-9]*)/)) { v=substr(input,p,RLENGTH); p+=RLENGTH; typ[k]="number" }
      else exit 1
      if(bad) exit 1
      val[k]=v; ws(); c=substr(input,p++,1)
      if(c=="}") break
      if(c!=",") exit 1
      ws()
    }
    ws(); if(p<=length(input) || seen_count!=count) exit 1
    for(k in allowed) {
      if(number[k]) { if(typ[k]!="number") exit 1 }
      else if(boolean[k]) { if(typ[k]!="bool" || val[k]!="true") exit 1 }
      else if(k=="prior_retirement_ref" && typ[k]=="null") continue
      else if(typ[k]!="string" || val[k] !~ /^[a-zA-Z0-9][a-zA-Z0-9_.:-]*$/ || length(val[k])>128) exit 1
    }
    if(!token(val["session_id"]) || !hex(val["runner_id"]) || !digest(val["helper_digest"])) exit 1
    if(!positive_int64(val["worker_generation"])) exit 1
    if(schema=="prune") {
      if(val["protocol"]!="lb-capture-prune-v1" || !token(val["retirement_ref"]) || !token(val["prior_invocation_ref"]) || !token(val["next_invocation_ref"]) || !digest(val["prior_manifest_digest"])) exit 1
      print val["retirement_ref"],val["session_id"],val["worker_generation"],val["runner_id"],val["prior_invocation_ref"],val["prior_manifest_digest"],val["next_invocation_ref"],val["helper_digest"]
    } else {
      if(val["protocol"]!="lb-step-capture-v2" || !token(val["job_id"]) || !token(val["task_id"]) || !token(val["invocation_ref"]) || !digest(val["execution_digest"]) || !hex(val["stdout_sha256"]) || !hex(val["stderr_sha256"])) exit 1
      if(typ["prior_retirement_ref"]!="null" && !token(val["prior_retirement_ref"])) exit 1
      if(!positive_int64(val["attempt_no"]) || length(val["child_exit_code"])>3 || val["child_exit_code"]+0>255) exit 1
      if(length(val["stdout_bytes"])>8 || val["stdout_bytes"]+0>67108864 || length(val["stderr_bytes"])>8 || val["stderr_bytes"]+0>67108864) exit 1
      print val["job_id"],val["session_id"],val["invocation_ref"],val["worker_generation"],val["runner_id"],val["helper_digest"],val["child_exit_code"]
    }
  }'
}
[ "$(id -u)" = 0 ] || die
[ "$#" -ge 1 ] || die
token "${SQLRS_SESSION_ID:-}" || die
case "${SQLRS_WORKER_GENERATION:-}" in ''|*[!0-9]*|0*) die ;; esac
# Restrict to the engine's signed 64-bit integer domain without overflow or
# floating-point conversion. Leading zeroes would make non-canonical JSON.
[ "${#SQLRS_WORKER_GENERATION}" -le 19 ] || die
if [ "${#SQLRS_WORKER_GENERATION}" -eq 19 ]; then
  [ "$SQLRS_WORKER_GENERATION" \< 9223372036854775808 ] || die
fi
digest "${SQLRS_RUNNER_CONFIG_DIGEST:-}" || die
digest "${SQLRS_HELPER_DIGEST:-}" || die
self_hash=$(sha256sum "$0") || die
[ "sha256:${self_hash%% *}" = "$SQLRS_HELPER_DIGEST" ] || die
ready=/sqlrs/capture/ready.complete
expected_ready=$(printf '{"protocol":"lb-capture-ready-v1","session_id":"%s","worker_generation":%s,"runner_config_digest":"%s","helper_digest":"%s","capture_protocol_revision":"lb-step-capture-v2"}' \
  "$SQLRS_SESSION_ID" "$SQLRS_WORKER_GENERATION" "$SQLRS_RUNNER_CONFIG_DIGEST" "$SQLRS_HELPER_DIGEST") || die
if [ "$1" = serve ]; then
[ "$#" -eq 1 ] || die
private_root || die
# A prior/partial initialization is not silently repaired or overwritten.
# Lost-response reconciliation reads the existing exact marker through Docker.
for entry in /sqlrs/capture/.[!.]* /sqlrs/capture/..?* /sqlrs/capture/*; do
  [ ! -e "$entry" ] && [ ! -L "$entry" ] || die
done
partial=/sqlrs/capture/ready.partial
(set -C; : > "$partial") 2>/dev/null || die
printf '%s\n' "$expected_ready" > "$partial" || die
[ "$(wc -c < "$partial")" -le 8192 ] || die
sync -f "$partial" || die
# The exclusive partial file fences competing initializers. Publish with one
# rename (not a link/unlink pair): readers must never see a ready file with a
# transient second hard link that would fail the invocation ownership check.
mv -T "$partial" "$ready" || die
sync -f /sqlrs/capture || die
while :; do sleep 3600; done
fi

# Invocation does not create/repair readiness or the capture root. Its caller
# must first have admitted native-bound readiness and this exact slot in SQLite.
trusted_parent / && trusted_parent /sqlrs || die
[ ! -L /sqlrs/capture ] && [ -d /sqlrs/capture ] || die
[ "$(stat -c '%u:%a' /sqlrs/capture)" = '0:700' ] || die
[ ! -L "$ready" ] && [ -f "$ready" ] || die
[ "$(stat -c '%u:%a:%h' "$ready")" = '0:600:1' ] || die
[ "$(wc -c < "$ready")" -le 8192 ] || die
printf '%s\n' "$expected_ready" | cmp -s - "$ready" || die
for value in "${SQLRS_JOB_ID:-}" "${SQLRS_TASK_ID:-}" "${SQLRS_INVOCATION_REF:-}"; do token "$value" || die; done
[ "${SQLRS_CAPTURE_DIR:-}" = "/sqlrs/capture/$SQLRS_INVOCATION_REF" ] || die
case "${SQLRS_ATTEMPT_NO:-}" in ''|*[!0-9]*|0*) die ;; esac
[ "${#SQLRS_ATTEMPT_NO}" -le 19 ] || die
if [ "${#SQLRS_ATTEMPT_NO}" -eq 19 ]; then [ "$SQLRS_ATTEMPT_NO" \< 9223372036854775808 ] || die; fi
case "${SQLRS_RUNNER_ID:-}" in ''|*[!0-9a-f]*) die ;; esac
[ "${#SQLRS_RUNNER_ID}" -eq 64 ] || die
digest "${SQLRS_EXECUTION_DIGEST:-}" || die
case "${SQLRS_CAPTURE_MAX_BYTES:-}" in ''|*[!0-9]*|0*) die ;; esac
[ "${#SQLRS_CAPTURE_MAX_BYTES}" -le 8 ] || die
[ "$SQLRS_CAPTURE_MAX_BYTES" -le 67108864 ] || die
prior_json=null
prior_capture=
prior_present=0
if [ -n "${SQLRS_CAPTURE_PRUNE_AUTHORIZATION:-}" ]; then
  [ "${#SQLRS_CAPTURE_PRUNE_AUTHORIZATION}" -le 4096 ] || die
  # Byte count also bounds non-ASCII/multibyte input before the JSON parser.
  [ "$(printf '%s' "$SQLRS_CAPTURE_PRUNE_AUTHORIZATION" | wc -c)" -le 4096 ] || die
  bindings=$(printf '%s' "$SQLRS_CAPTURE_PRUNE_AUTHORIZATION" | json_bindings prune) || die
  IFS=' ' read -r retirement_ref prior_session prior_generation prior_runner prior_invocation prior_digest next_invocation prior_helper <<EOF
$bindings
EOF
  [ "$prior_session" = "$SQLRS_SESSION_ID" ] && [ "$prior_generation" = "$SQLRS_WORKER_GENERATION" ] || die
  [ "$prior_runner" = "$SQLRS_RUNNER_ID" ] && [ "$prior_helper" = "$SQLRS_HELPER_DIGEST" ] || die
  [ "$next_invocation" = "$SQLRS_INVOCATION_REF" ] && [ "$prior_invocation" != "$next_invocation" ] || die
  prior_capture=/sqlrs/capture/$prior_invocation
  if [ "$1" = probe-prune ]; then
    [ "$#" -eq 1 ] || die
    [ ! -e "$prior_capture" ] && [ ! -L "$prior_capture" ] || exit 91
    printf 'sqlrs-capture-prune-absent-v1\n'
    exit 0
  fi
  if [ -e "$prior_capture" ] || [ -L "$prior_capture" ]; then
  prior_present=1
  [ ! -L "$prior_capture" ] && [ -d "$prior_capture" ] || die
  [ "$(stat -c '%u:%a' "$prior_capture")" = '0:700' ] || die
  for file in manifest.complete stdout.complete stderr.complete; do
    [ ! -L "$prior_capture/$file" ] && [ -f "$prior_capture/$file" ] || die
    [ "$(stat -c '%u:%a:%h' "$prior_capture/$file")" = '0:600:1' ] || die
  done
  for entry in "$prior_capture"/.[!.]* "$prior_capture"/..?* "$prior_capture"/*; do
    case "$entry" in "$prior_capture/manifest.complete"|"$prior_capture/stdout.complete"|"$prior_capture/stderr.complete") continue ;; esac
    [ ! -e "$entry" ] && [ ! -L "$entry" ] || die
  done
  [ "$(wc -c < "$prior_capture/manifest.complete")" -le 8192 ] || die
  manifest_hash=$(sha256sum "$prior_capture/manifest.complete") || die
  [ "sha256:${manifest_hash%% *}" = "$prior_digest" ] || die
  bindings=$(json_bindings step < "$prior_capture/manifest.complete") || die
  IFS=' ' read -r old_job old_session old_invocation old_generation old_runner old_helper old_exit <<EOF
$bindings
EOF
  [ "$old_job" = "$SQLRS_JOB_ID" ] && [ "$old_session" = "$SQLRS_SESSION_ID" ] && [ "$old_invocation" = "$prior_invocation" ] || die
  [ "$old_generation" = "$SQLRS_WORKER_GENERATION" ] && [ "$old_runner" = "$SQLRS_RUNNER_ID" ] || die
  [ "$old_helper" = "$SQLRS_HELPER_DIGEST" ] && [ "$old_exit" = 0 ] || die
  fi
  # Prior absence may already be confirmed by the exact saved retirement. It
  # grants no current non-start/replay proof; the exclusive current-directory
  # fence below remains mandatory, and the queue controls first-start admission.
  prior_json=\"$retirement_ref\"
else
  [ "$1" != probe-prune ] || die
fi
for entry in /sqlrs/capture/.[!.]* /sqlrs/capture/..?* /sqlrs/capture/*; do
  [ "$entry" = "$ready" ] && continue
  [ -n "$prior_capture" ] && [ "$entry" = "$prior_capture" ] && continue
  [ ! -e "$entry" ] && [ ! -L "$entry" ] || die
done
capture=$SQLRS_CAPTURE_DIR
# Reserve the new identity before deleting a single old byte. A crash at any
# later point leaves this directory as a replay fence, even without a manifest.
mkdir -m 700 "$capture" || die
if [ "$prior_present" -eq 1 ]; then
  rm "$prior_capture/manifest.complete" "$prior_capture/stdout.complete" "$prior_capture/stderr.complete" || die
  rmdir "$prior_capture" || die
  sync -f /sqlrs/capture || die
elif [ -n "$prior_capture" ]; then
  [ ! -e "$prior_capture" ] && [ ! -L "$prior_capture" ] || die
fi
stdout_fifo=$capture/stdout.pipe
stderr_fifo=$capture/stderr.pipe
stdout_partial=$capture/stdout.partial
stderr_partial=$capture/stderr.partial
manifest_partial=$capture/manifest.partial
capture_blocks=$(( (SQLRS_CAPTURE_MAX_BYTES + 511) / 512 ))
mkfifo "$stdout_fifo" "$stderr_fifo" || die
# Count exact bytes while writing, not only after EOF. ulimit units differ
# between shells; it is a backstop, not the logical private-storage bound.
# Keep one FIFO descriptor open through the extra-byte check. The extra byte
# is counted in a pipe, never retained in a partial file or diagnostic output.
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
sync -f "$stdout_partial" "$stderr_partial" || die
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
sync -f "$capture/stdout.complete" "$capture/stderr.complete" || die
printf '{"protocol":"lb-step-capture-v2","job_id":"%s","task_id":"%s","session_id":"%s","invocation_ref":"%s","attempt_no":%s,"worker_generation":%s,"runner_id":"%s","execution_digest":"%s","helper_digest":"%s","child_exit_code":%s,"stdout_eof":true,"stderr_eof":true,"stdout_bytes":%s,"stdout_sha256":"%s","stderr_bytes":%s,"stderr_sha256":"%s","prior_retirement_ref":%s}\n' \
  "$SQLRS_JOB_ID" "$SQLRS_TASK_ID" "$SQLRS_SESSION_ID" "$SQLRS_INVOCATION_REF" \
  "$SQLRS_ATTEMPT_NO" "$SQLRS_WORKER_GENERATION" "$SQLRS_RUNNER_ID" "$SQLRS_EXECUTION_DIGEST" "$SQLRS_HELPER_DIGEST" \
  "$child_exit" "$stdout_bytes" "$stdout_hash" "$stderr_bytes" "$stderr_hash" "$prior_json" > "$manifest_partial" || die
[ "$(wc -c < "$manifest_partial")" -le 8192 ] || die
sync -f "$manifest_partial" || die
mv "$manifest_partial" "$capture/manifest.complete" || die
sync -f "$capture" || die
exit "$child_exit"
