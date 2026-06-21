#!/usr/bin/env bash
set -euo pipefail

RESULTS_DIR="${RESULTS_DIR:-./k6-scripts/results/$(date +%Y%m%d-%H%M%S)}"
BASE_URL_IN_COMPOSE="${BASE_URL_IN_COMPOSE:-http://app:12660}"
APP_CONTAINER="${APP_CONTAINER:-RPA-app}"
SAMPLE_INTERVAL="${SAMPLE_INTERVAL:-1}"
LOAD_EMAIL="${LOAD_EMAIL:-k6-admin@example.com}"
LOAD_PASSWORD="${LOAD_PASSWORD:-password}"
DB_USER="${DB_USER:-${POSTGRES_USER:-rpa_app}}"
DB_NAME="${DB_NAME:-${POSTGRES_DB:-RPA}}"

mkdir -p "${RESULTS_DIR}"
mkdir -p ./k6-scripts/fixtures
chmod 0777 "${RESULTS_DIR}"

if [ ! -f ./k6-scripts/fixtures/app.gz ]; then
  printf 'rpa-market-load-test-fixture\n' | gzip -c > ./k6-scripts/fixtures/app.gz
fi

if [ ! -f ./k6-scripts/fixtures/avatar.png ]; then
  base64 -d > ./k6-scripts/fixtures/avatar.png <<'PNG'
iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+/p9sAAAAASUVORK5CYII=
PNG
fi

write_manifest() {
  {
    printf 'timestamp=%s\n' "$(date -Is)"
    printf 'results_dir=%s\n' "${RESULTS_DIR}"
    printf 'base_url=%s\n' "${BASE_URL_IN_COMPOSE}"
    printf 'feature_jwt_auth=%s\n' "${FEATURE_JWT_AUTH:-false}"
    printf 'feature_casbin_authz=%s\n' "${FEATURE_CASBIN_AUTHZ:-false}"
    printf 'feature_rate_limit_enabled=%s\n' "${FEATURE_RATE_LIMIT_ENABLED:-false}"
    printf 'feature_request_pool_enabled=%s\n' "${FEATURE_REQUEST_POOL_ENABLED:-false}"
    printf 'git_available=%s\n' "$(git rev-parse --is-inside-work-tree 2>/dev/null || true)"
    printf 'docker_compose_version=%s\n' "$(docker compose version --short 2>/dev/null || true)"
  } > "${RESULTS_DIR}/manifest.txt"
}

sample_stats() {
  local out="$1"
  printf 'timestamp,name,cpu_perc,mem_usage,mem_perc,net_io,block_io,pids\n' > "${out}"
  while true; do
    docker stats --no-stream --format "$(date -Is),{{.Name}},{{.CPUPerc}},{{.MemUsage}},{{.MemPerc}},{{.NetIO}},{{.BlockIO}},{{.PIDs}}" >> "${out}" || true
    sleep "${SAMPLE_INTERVAL}"
  done
}

run_k6() {
  local name="$1"
  local script="$2"
  shift 2
  local stats_file="${RESULTS_DIR}/${name}.docker-stats.csv"
  sample_stats "${stats_file}" &
  local sampler_pid=$!
  set +e
  docker compose run --rm "$@" k6 run \
    -o experimental-prometheus-rw \
    --tag "testid=${name}" \
    --summary-export "/scripts/results/$(basename "${RESULTS_DIR}")/${name}.summary.json" \
    "${script}" 2>&1 | tee "${RESULTS_DIR}/${name}.log"
  local status=${PIPESTATUS[0]}
  set -e
  kill "${sampler_pid}" 2>/dev/null || true
  wait "${sampler_pid}" 2>/dev/null || true
  printf '%s\n' "${status}" > "${RESULTS_DIR}/${name}.exitcode"
  return 0
}

run_smoke() {
  local prefix="$1"
  local auth_mode="$2"
  run_k6 "${prefix}_smoke" /scripts/smoke.js \
    -e BASE_URL="${BASE_URL_IN_COMPOSE}" \
    -e EMAIL="${LOAD_EMAIL}" \
    -e PASSWORD="${LOAD_PASSWORD}" \
    -e AUTH_MODE="${auth_mode}" \
    -e SMOKE_VUS="${SMOKE_VUS:-1}" \
    -e SMOKE_ITERATIONS="${SMOKE_ITERATIONS:-1}"
}

run_api() {
  local prefix="$1"
  local auth_mode="$2"
  local api="$3"
  local rate_var="${api^^}_RATE"
  local duration_var="${api^^}_DURATION"
  local vus_var="${api^^}_VUS"
  local max_vus_var="${api^^}_MAX_VUS"

  run_k6 "${prefix}_${api}" "/scripts/apis/${api}.js" \
    -e BASE_URL="${BASE_URL_IN_COMPOSE}" \
    -e EMAIL="${LOAD_EMAIL}" \
    -e PASSWORD="${LOAD_PASSWORD}" \
    -e AUTH_MODE="${auth_mode}" \
    -e RATE="${!rate_var:-${DEFAULT_RATE:-100}}" \
    -e DURATION="${!duration_var:-${DEFAULT_DURATION:-1m}}" \
    -e VUS="${!vus_var:-${DEFAULT_VUS:-100}}" \
    -e MAX_VUS="${!max_vus_var:-${DEFAULT_MAX_VUS:-200}}"
}

run_api_suite() {
  local prefix="$1"
  local auth_mode="$2"
  shift 2
  for api in "$@"; do
    run_api "${prefix}" "${auth_mode}" "${api}"
  done
}

run_http_api_suite() {
  local prefix="$1"
  local auth_mode="$2"
  run_api_suite "${prefix}" "${auth_mode}" \
    iam_register \
    iam_login \
    iam_profile_get \
    iam_profile_update \
    iam_profile_avatar \
    iam_groups_list \
    iam_group_create \
    iam_group_detail \
    iam_group_update \
    iam_roles_list \
    iam_role_get \
    iam_permissions_list \
    market_apps_list \
    market_apps_search \
    market_rank_daily \
    market_rank_weekly \
    market_rank_total \
    market_app_publish \
    market_app_detail \
    market_app_download \
    market_app_update
}

current_auth_mode() {
  if [ "${FEATURE_JWT_AUTH:-false}" = "true" ]; then
    printf 'login\n'
    return
  fi
  printf 'bypass\n'
}

current_feature_prefix() {
  printf 'jwt_%s_casbin_%s_rl_%s_pool_%s\n' \
    "${FEATURE_JWT_AUTH:-false}" \
    "${FEATURE_CASBIN_AUTHZ:-false}" \
    "${FEATURE_RATE_LIMIT_ENABLED:-false}" \
    "${FEATURE_REQUEST_POOL_ENABLED:-false}"
}

restart_app() {
  local jwt="$1"
  local casbin="$2"
  local rate_limit="$3"
  local request_pool="$4"
  FEATURES_JWT_AUTH="${jwt}" \
    FEATURES_CASBIN_AUTHZ="${casbin}" \
    FEATURES_RATE_LIMIT_ENABLED="${rate_limit}" \
    FEATURES_RATE_LIMIT_RATE="${FEATURES_RATE_LIMIT_RATE:-100000}" \
    FEATURES_RATE_LIMIT_CAPACITY="${FEATURES_RATE_LIMIT_CAPACITY:-100000}" \
    FEATURES_RATE_LIMIT_LIMIT="${FEATURES_RATE_LIMIT_LIMIT:-100000}" \
    FEATURES_REQUEST_POOL_ENABLED="${request_pool}" \
    FEATURES_REQUEST_POOL_CAPACITY="${FEATURES_REQUEST_POOL_CAPACITY:-10000}" \
    docker compose up -d --force-recreate app
  for _ in $(seq 1 30); do
    if docker compose exec -T app /bin/sh -c "wget -qO- http://localhost:12660/api/v1/market/apps?page=1\\&page_size=1 >/dev/null"; then
      return 0
    fi
    sleep 1
  done
  return 1
}

ensure_mq_topic() {
  docker compose exec -T rmqbroker /home/rocketmq/rocketmq-5.3.4/bin/mqadmin updateTopic \
    -n rmqnamesrv:9876 \
    -t Topic_Casbin_Sync \
    -c DefaultCluster > "${RESULTS_DIR}/mq-topic-init.log" 2>&1 || true
}

ensure_load_user() {
  curl -sS -X POST "http://localhost:12660/api/v1/iam/register" \
    -H 'Content-Type: application/json' \
    -d "{\"username\":\"k6-admin\",\"email\":\"${LOAD_EMAIL}\",\"password\":\"${LOAD_PASSWORD}\"}" \
    > "${RESULTS_DIR}/load-user-register.json" || true

  docker exec RPA-pgdb psql -U "${DB_USER}" -d "${DB_NAME}" -v ON_ERROR_STOP=1 -c "
    INSERT INTO group_members (group_id, user_id, role_id)
    SELECT '11111111-1111-1111-1111-111111111111', u.user_id, r.role_id
    FROM users u, roles r
    WHERE u.email = '${LOAD_EMAIL}' AND r.role_name = 'superadmin'
    ON CONFLICT (group_id, user_id) DO UPDATE SET role_id = EXCLUDED.role_id;
  " > "${RESULTS_DIR}/load-user-grant.log" 2>&1
}

write_manifest

FEATURE_PREFIX="${FEATURE_PREFIX:-$(current_feature_prefix)}"
AUTH_MODE_FOR_TEST="${AUTH_MODE_FOR_TEST:-$(current_auth_mode)}"

if [ "${RUN_HTTP_APIS:-true}" = "true" ]; then
  if [ "${FEATURE_CASBIN_AUTHZ:-false}" = "true" ]; then
    ensure_mq_topic
  fi
  restart_app \
    "${FEATURE_JWT_AUTH:-false}" \
    "${FEATURE_CASBIN_AUTHZ:-false}" \
    "${FEATURE_RATE_LIMIT_ENABLED:-false}" \
    "${FEATURE_REQUEST_POOL_ENABLED:-false}"
  ensure_load_user
  run_smoke "${FEATURE_PREFIX}" "${AUTH_MODE_FOR_TEST}"
  run_http_api_suite "${FEATURE_PREFIX}" "${AUTH_MODE_FOR_TEST}"
fi

if [ "${RUN_MQ:-false}" = "true" ]; then
  ensure_mq_topic
  restart_app \
    "${FEATURE_JWT_AUTH:-false}" \
    "${FEATURE_CASBIN_AUTHZ:-false}" \
    "${FEATURE_RATE_LIMIT_ENABLED:-false}" \
    "${FEATURE_REQUEST_POOL_ENABLED:-false}"
  ensure_load_user
  run_smoke "${FEATURE_PREFIX}_mq" "${AUTH_MODE_FOR_TEST}"
  run_api_suite "${FEATURE_PREFIX}_mq" "${AUTH_MODE_FOR_TEST}" \
    mq_role_purge_broadcast \
    mq_group_invalidate_broadcast
fi

printf 'Load test data saved to %s\n' "${RESULTS_DIR}"
