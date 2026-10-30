#!/usr/bin/env bash
# Orca's setup and archive hooks: lets several worktrees run the app at once.
#
#   scripts/worktree.sh setup     on a new worktree: the dev shell, a .env with this
#                                 worktree's ports and database, the frontend
#                                 dependencies, and the database migrated and seeded
#   scripts/worktree.sh archive   when Orca archives it: drops the database, frees the ports
#
# Every worktree shares the Postgres in compose.yaml and gets its own database in
# it. Ports go by slot: slot N serves the API on 8080+10N and the frontend on
# 5173+10N (the main checkout is slot 0). Slots are claimed in the Git common
# dir, which every worktree of the repo shares. Running either command again
# changes nothing.
set -euo pipefail

root="$(git rev-parse --show-toplevel)"
cd "$root"

if [ "$(git rev-parse --path-format=absolute --git-dir)" = "$(git rev-parse --path-format=absolute --git-common-dir)" ]; then
  echo "worktree.sh: this is the main checkout, which keeps its own .env" >&2
  exit 1
fi

marker="# Written by scripts/worktree.sh"
claims="$(git rev-parse --path-format=absolute --git-common-dir)/lastro-worktrees"
slug="$(basename "$root" | tr '[:upper:]' '[:lower:]' | tr -c 'a-z0-9\n' '_' | cut -c1-40)"
db="lastro_$slug"

listening() {
  (exec 3<>"/dev/tcp/127.0.0.1/$1") 2>/dev/null || (exec 3<>"/dev/tcp/::1/$1") 2>/dev/null
}

# claim prints this worktree's slot, taking the first free one if it has none.
claim() {
  exec 9>"$claims.lock"
  flock 9
  touch "$claims"
  # Worktrees removed without the archive hook leave their claim behind.
  local kept slot path
  kept="$(while read -r slot path; do [ -d "$path" ] && echo "$slot $path"; done <"$claims")"
  printf '%s\n' "$kept" | sed '/^$/d' >"$claims"
  slot="$(awk -v p="$root" '$2 == p { print $1 }' "$claims")"
  if [ -z "$slot" ]; then
    for n in $(seq 1 50); do
      grep -q "^$n " "$claims" && continue
      listening $((8080 + 10 * n)) && continue
      listening $((5173 + 10 * n)) && continue
      slot="$n"
      echo "$slot $root" >>"$claims"
      break
    done
  fi
  flock -u 9
  [ -n "$slot" ] || { echo "worktree.sh: no free slot" >&2; exit 1; }
  echo "$slot"
}

ours() { [ -f .env ] && grep -qF "$marker" .env; }

docker_up() {
  docker info >/dev/null 2>&1 || { echo "Docker is not running: skipping the database. Run scripts/worktree.sh setup again once it is."; return 1; }
}

psql_admin() { docker compose exec -T postgres psql -U lastro -d postgres -v ON_ERROR_STOP=1 -tAc "$1"; }

setup() {
  # The tools come from flake.nix, and Orca's shells do not load direnv on their own.
  if [ -z "${IN_NIX_SHELL:-}" ]; then
    command -v direnv >/dev/null && direnv allow
    exec nix develop --command "$root/scripts/worktree.sh" setup
  fi

  if [ -f .env ] && ! ours; then
    echo "Keeping the .env that is already here; skipping the database."
  elif [ ! -f .env ]; then
    local slot api web
    slot="$(claim)"
    api=$((8080 + 10 * slot))
    web=$((5173 + 10 * slot))
    {
      echo "$marker (slot $slot): API on :$api, frontend on :$web, database $db."
      echo "# No Pluggy credentials, so this worktree never syncs real data."
      echo
      sed -e "s|^LASTRO_HTTP_ADDR=.*|LASTRO_HTTP_ADDR=:$api|" \
        -e "s|^LASTRO_WEB_PORT=.*|LASTRO_WEB_PORT=$web|" \
        -e "s|^LASTRO_PUBLIC_URL=.*|LASTRO_PUBLIC_URL=http://localhost:$api|" \
        -e "s|^\(LASTRO_DATABASE_URL=.*/\)lastro?|\1$db?|" \
        .env.example
    } >.env
    echo "Wrote .env: API on :$api, frontend on :$web, database $db"
  fi

  go mod download
  [ -d web/node_modules ] || npm --prefix web ci

  if ours && docker_up; then
    task db
    if [ -z "$(psql_admin "select 1 from pg_database where datname = '$db'")" ]; then
      docker compose exec -T postgres createdb -U lastro "$db"
    fi
    task migrate
    task seed
  fi

  echo
  task urls
  echo "Start both with: task up"
}

archive() {
  if ours && docker_up && docker compose ps --status running --services | grep -qx postgres; then
    psql_admin "drop database if exists \"$db\" with (force)" >/dev/null
    echo "Dropped database $db"
  fi
  if [ -f "$claims" ]; then
    exec 9>"$claims.lock"
    flock 9
    awk -v p="$root" '$2 != p' "$claims" >"$claims.tmp" && mv "$claims.tmp" "$claims"
    flock -u 9
  fi
}

case "${1:-}" in
setup) setup ;;
archive) archive ;;
*)
  echo "usage: scripts/worktree.sh setup|archive" >&2
  exit 2
  ;;
esac
