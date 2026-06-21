#!/bin/bash

echo "🛑 [1/4] 关闭第四层：停止前端入口与数据采集 (Grafana, Alloy)..."
docker compose stop grafana alloy
echo "⏳ 等待 3 秒..."
sleep 3

echo "🛑 [2/4] 关闭第三层：停止业务应用与日志后端 (App, Loki)..."
docker compose stop app loki
echo "⏳ 等待 3 秒..."
sleep 3

echo "🛑 [3/4] 关闭第二层：停止中间件核心 (Prometheus, rmqdashboard, rmqbroker)..."
docker compose stop prometheus rmqdashboard rmqbroker
echo "⏳ 等待 3 秒..."
sleep 3

echo "🛑 [4/4] 关闭第一层：停止基础存储与底层监控 (db, redis, minio, rmqnamesrv, cadvisor)..."
docker compose stop db redis minio rmqnamesrv cadvisor

echo "✅ 所有服务已安全停止！"