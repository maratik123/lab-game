#!/usr/bin/env bash
# Stops the contention target's load loop while one of its test binaries is
# provisioning a container, on purpose, and reports whether that stop left
# anything of the run behind.
#
# The target stops its load loop when its foreground race run returns, so this
# probe controls that instant. It puts a stand-in for the go command first on
# PATH; the stand-in passes every invocation through to the real toolchain
# except the race run, which it replaces with a wait on the container runtime's
# event stream. The wait returns the moment an existence-probe container is
# created whose owning test process descends from the target's own shell — the
# window in which a stopped binary has created a container but has not yet
# registered the cleanup that removes it, and its session's reaper is not yet
# up.
#
# Everything read afterwards is scoped to that container's session label, so a
# container another checkout provisions on the same host is never counted. A
# running reaper of the session is not a leftover: it removes itself on exit.
# A reaper that never started is.
#
# Exit 0 = the stop landed inside the window and nothing of the session is left.
# Exit 1 = a container or a volume of the session is left.
# Exit 2 = inconclusive: a tool is missing, no window was observed, the stop
#          landed after the container had already started, or no volume of the
#          session was recorded to check.

set -uo pipefail

# How long the stand-in waits for a provisioning window before giving up.
window_ceiling_s=300
# How long after the target returns a leftover may still be removed by its
# session's reaper before it counts: longer than the reaper's default
# ten-second reconnection timeout, with room for the removal itself.
settle_ceiling_s=60
# How often the session's volumes are recorded, from the moment the window
# opens until the target returns. One late look is not enough: once the stop
# no longer kills them, the tests in flight stop and remove their own
# containers within seconds, and a look taken while they do records less than
# the session had.
record_interval_s=0.2
probe_prefix=lab-game-exists-probe-

usage() {
  cat <<'USAGE'
Usage:
  probe-contention-stop.sh     run make test-contention with its load loop
                               stopped inside a container provisioning, and
                               report what that stop left behind
USAGE
}

inconclusive() { printf 'probe: INCONCLUSIVE — %s\n' "$*"; exit 2; }

# descends_from <pid> <ancestor> succeeds when the parent chain of pid reaches
# ancestor while both are alive.
descends_from() {
  local pid=$1 ancestor=$2
  while [ -n "$pid" ] && [ "$pid" -gt 1 ]; do
    [ "$pid" = "$ancestor" ] && return 0
    pid=$(ps -o ppid= -p "$pid" 2>/dev/null | tr -d ' ')
  done
  return 1
}

# shim is the go stand-in. The race run waits for the window and returns;
# every other invocation becomes the real go.
shim() {
  case " $* " in
    *" -race "*) ;;
    *) exec "$PROBE_REAL_GO" "$@" ;;
  esac
  local anchor=$PPID events_pid t name id sid pid
  exec 3< <(exec timeout "$window_ceiling_s" podman events --since 2m \
    --filter type=container --filter event=create \
    --filter label=org.testcontainers=true \
    --format '{{.TimeNano}} {{.Name}} {{.ID}} {{index .Attributes "org.testcontainers.sessionId"}}')
  events_pid=$!
  while read -r -u 3 t name id sid; do
    case "$name" in
      "$probe_prefix"*) pid=${name#"$probe_prefix"} ;;
      *) continue ;;
    esac
    case "$pid" in '' | *[!0-9]*) continue ;; esac
    descends_from "$pid" "$anchor" || continue
    printf '%s %s %s %s\n' "$(date +%s%N)" "$t" "$id" "$sid" >"$PROBE_HIT.partial"
    mv "$PROBE_HIT.partial" "$PROBE_HIT"
    echo "probe: window observed on $name; returning in place of the race run"
    break
  done
  kill "$events_pid" 2>/dev/null
  exec 3<&-
  exit 0
}

# session_containers <sid> prints one line per container of the session:
# id, name, state, and whether it is the session's reaper.
session_containers() {
  podman ps -a --filter "label=org.testcontainers.sessionId=$1" \
    --format '{{.ID}} {{.Names}} {{.State}} {{index .Labels "org.testcontainers.reaper"}}'
}

# session_volumes <sid> prints the volumes mounted by the session's containers
# other than its reaper, one per line.
session_volumes() {
  local cid name state reaper
  while read -r cid name state reaper; do
    [ "$reaper" = true ] && continue
    podman inspect --format '{{range .Mounts}}{{if eq .Type "volume"}}{{.Name}}{{"\n"}}{{end}}{{end}}' "$cid" </dev/null 2>/dev/null
  done < <(session_containers "$1") | sed '/^$/d'
}

main() {
  command -v podman >/dev/null 2>&1 || inconclusive "podman is not on PATH"
  command -v timeout >/dev/null 2>&1 || inconclusive "timeout is not on PATH"
  local root script dir hit real_go
  root=$(git rev-parse --show-toplevel) || inconclusive "not inside the repository"
  script=$(realpath "${BASH_SOURCE[0]}")
  real_go=$(command -v go) || inconclusive "go is not on PATH"
  cd "$root" || exit 2

  dir=$root/tmp/contention-stop-probe
  hit=$dir/hit
  rm -rf -- "$dir"
  mkdir -p "$dir/bin"
  # shellcheck disable=SC2016  # the positional parameters are the generated stand-in's own
  printf '#!/usr/bin/env bash\nexec bash %q __shim "$@"\n' "$script" >"$dir/bin/go"
  chmod +x "$dir/bin/go"

  export PROBE_REAL_GO=$real_go PROBE_HIT=$hit PATH="$dir/bin:$PATH"

  local make_pid make_status=0
  make --no-print-directory test-contention >"$dir/make.log" 2>&1 &
  make_pid=$!
  while [ ! -f "$hit" ] && kill -0 "$make_pid" 2>/dev/null; do
    sleep 0.05
  done

  local stop_ns='' create_ns='' id='' sid='' volumes=() v
  local -A seen=()
  if [ -f "$hit" ]; then
    read -r stop_ns create_ns id sid <"$hit"
    while :; do
      while read -r v; do
        [ -n "${seen[$v]:-}" ] && continue
        seen[$v]=1
        volumes+=("$v")
      done < <(session_volumes "$sid")
      kill -0 "$make_pid" 2>/dev/null || break
      sleep "$record_interval_s"
    done
  fi
  wait "$make_pid" || make_status=$?
  echo "probe: make test-contention exited $make_status; its output is in $dir/make.log"

  [ -n "$sid" ] || inconclusive "no existence-probe container of this run was created within ${window_ceiling_s}s"
  echo "probe: session $sid"
  echo "probe: stop requested $(((stop_ns - create_ns) / 1000000)) ms after container $id was created"

  local start_ns
  start_ns=$(podman events --stream=false --since 30m --until 1s \
    --filter "container=$id" --filter event=start --format '{{.TimeNano}}' | head -n 1)
  if [ -n "$start_ns" ] && [ "$start_ns" -lt "$stop_ns" ]; then
    inconclusive "the stop was requested $(((stop_ns - start_ns) / 1000000)) ms after the container started, outside the window"
  fi

  local deadline=$((SECONDS + settle_ceiling_s)) left=() vleft=() cid name state reaper
  while :; do
    left=()
    vleft=()
    while read -r cid name state reaper; do
      [ -n "$cid" ] || continue
      [ "$reaper" = true ] && [ "$state" = running ] && continue
      left+=("$name $cid $state")
    done < <(session_containers "$sid")
    for v in "${volumes[@]}"; do
      podman volume exists "$v" && vleft+=("$v")
    done
    if [ "${#left[@]}" -eq 0 ] && [ "${#vleft[@]}" -eq 0 ]; then break; fi
    if [ "$SECONDS" -ge "$deadline" ]; then break; fi
    sleep 2
  done

  if [ "${#left[@]}" -gt 0 ] || [ "${#vleft[@]}" -gt 0 ]; then
    printf 'probe: RED — %d container(s) and %d volume(s) of session %s left %ss after the target returned:\n' \
      "${#left[@]}" "${#vleft[@]}" "$sid" "$settle_ceiling_s"
    printf '  container %s\n' "${left[@]}"
    [ "${#vleft[@]}" -eq 0 ] || printf '  volume %s\n' "${vleft[@]}"
    exit 1
  fi
  [ "${#volumes[@]}" -gt 0 ] || inconclusive "no container of session $sid is left, but no volume of it was recorded to check"
  printf 'probe: GREEN — nothing of session %s is left; %d recorded volume(s) are gone\n' "$sid" "${#volumes[@]}"
  exit 0
}

case "${1:-}" in
  -h|--help) usage; exit 0 ;;
  __shim) shift; shim "$@" ;;
esac

main "$@"
