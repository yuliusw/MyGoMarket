#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SCRIPT_DIR="${ROOT_DIR}/script"
COMPOSE_FILE="${SCRIPT_DIR}/docker-compose.yaml"
COMPOSE_ENV_ARGS=()

if [ -f "${SCRIPT_DIR}/.env" ]; then
  set -a
  # shellcheck disable=SC1091
  source "${SCRIPT_DIR}/.env"
  set +a
  COMPOSE_ENV_ARGS=(--env-file "${SCRIPT_DIR}/.env")
fi

APP_URL="${APP_URL:-http://localhost:12660/api/v1/market/apps?page=1&page_size=1}"
MINIO_ROOT_USER="${MINIO_ROOT_USER:-rpa_minio_access_key}"
MINIO_ROOT_PASSWORD="${MINIO_ROOT_PASSWORD:-change-me-minio-secret-key}"

cd "${ROOT_DIR}"

printf 'Building Go binary...\n'
go build -o "${SCRIPT_DIR}/main" .

printf 'Building Docker image rpa-app:v1.0...\n'
docker build -t rpa-app:v1.0 "${SCRIPT_DIR}"

printf 'Validating compose configuration...\n'
docker compose "${COMPOSE_ENV_ARGS[@]}" -f "${COMPOSE_FILE}" config >/tmp/rpa-market-compose-config.yaml

printf 'Removing old compose containers and volumes...\n'
docker compose "${COMPOSE_ENV_ARGS[@]}" -f "${COMPOSE_FILE}" down -v --remove-orphans

printf 'Starting fresh compose stack...\n'
docker compose "${COMPOSE_ENV_ARGS[@]}" -f "${COMPOSE_FILE}" up -d

printf 'Waiting for PostgreSQL and Redis health checks...\n'
for _ in $(seq 1 60); do
  db_status="$(docker inspect -f '{{.State.Health.Status}}' RPA-pgdb 2>/dev/null || true)"
  redis_status="$(docker inspect -f '{{.State.Health.Status}}' RPA-redis 2>/dev/null || true)"
  if [ "${db_status}" = "healthy" ] && [ "${redis_status}" = "healthy" ]; then
    break
  fi
  sleep 2
done

printf 'Ensuring app is running after dependencies are healthy...\n'
docker compose "${COMPOSE_ENV_ARGS[@]}" -f "${COMPOSE_FILE}" up -d app

printf 'Verifying database RBAC seed data...\n'
docker exec RPA-pgdb sh -c 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -v ON_ERROR_STOP=1 -P pager=off -c "SELECT '\''users'\'' AS table_name, count(*) FROM users UNION ALL SELECT '\''groups'\'', count(*) FROM groups UNION ALL SELECT '\''roles'\'', count(*) FROM roles UNION ALL SELECT '\''permissions'\'', count(*) FROM permissions UNION ALL SELECT '\''role_permissions'\'', count(*) FROM role_permissions ORDER BY table_name;" -c "SELECT r.role_name, count(p.permission_code) AS permission_count FROM roles r LEFT JOIN role_permissions rp ON rp.role_id=r.role_id LEFT JOIN permissions p ON p.permission_id=rp.permission_id GROUP BY r.role_name ORDER BY r.role_name;"'

printf 'Checking Redis...\n'
docker exec RPA-redis redis-cli ping

printf 'Checking MinIO buckets...\n'
docker run --rm --network script_default --entrypoint /bin/sh minio/mc -c "mc alias set myminio http://minio:9000 '${MINIO_ROOT_USER}' '${MINIO_ROOT_PASSWORD}' >/dev/null && mc ls myminio"

printf 'Checking RocketMQ cluster...\n'
docker exec rmqbroker /home/rocketmq/rocketmq-5.3.4/bin/mqadmin clusterList -n rmqnamesrv:9876

printf 'Checking app HTTP endpoint...\n'
curl -fsS "${APP_URL}"
printf '\nDeployment reset completed.\n'
