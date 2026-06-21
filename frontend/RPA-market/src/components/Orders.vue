<template>
  <section>
    <div class="page-title">
      <div>
        <h1>订单</h1>
        <p>检查购买后的订单、钱包流水和订阅发放 ID。</p>
      </div>
      <button class="btn secondary" @click="loadOrders">刷新订单</button>
    </div>

    <p v-if="message" class="error">{{ message }}</p>

    <div class="panel order-panel">
      <div class="table-wrap">
        <table>
          <thead>
            <tr>
              <th>创建时间</th>
              <th>状态</th>
              <th>金额</th>
              <th>应用</th>
              <th>订单</th>
              <th>流水</th>
              <th>订阅</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="order in orders" :key="order.order_id">
              <td>{{ formatDate(order.created_at) }}</td>
              <td><span :class="['status', order.status]">{{ order.status }}</span></td>
              <td>{{ order.amount }} {{ order.currency_code }}</td>
              <td class="mono">{{ order.app_id }}</td>
              <td class="mono">{{ order.order_id }}</td>
              <td class="mono">{{ order.tx_id || '-' }}</td>
              <td class="mono">{{ order.subscription_id || '-' }}</td>
            </tr>
            <tr v-if="!orders.length">
              <td colspan="7" class="empty">暂无订单，去应用市场购买一个应用。</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { listOrders, type Order } from '../api'

const orders = ref<Order[]>([])
const message = ref('')

async function loadOrders() {
  try {
    const res = await listOrders()
    orders.value = res.items || []
    message.value = ''
  } catch (err) {
    message.value = errorMessage(err, '订单加载失败，请先登录')
  }
}

function formatDate(value: string) {
  if (!value) return '-'
  return new Date(value).toLocaleString()
}

function errorMessage(err: unknown, fallback: string) {
  if (typeof err === 'object' && err && 'message' in err) return String((err as { message: unknown }).message)
  return fallback
}

onMounted(loadOrders)
</script>

<style scoped>
.order-panel {
  padding: 22px;
}

.table-wrap {
  overflow-x: auto;
}

table {
  width: 100%;
  border-collapse: collapse;
}

th,
td {
  border-bottom: 1px solid #e2e8f0;
  padding: 12px 10px;
  text-align: left;
  vertical-align: top;
}

th {
  color: #64748b;
  font-size: 12px;
  text-transform: uppercase;
}

.status {
  border-radius: 999px;
  padding: 4px 9px;
  background: #e2e8f0;
  color: #334155;
  font-size: 12px;
  font-weight: 800;
}

.status.paid {
  background: #dcfce7;
  color: #166534;
}

.status.pending {
  background: #fef3c7;
  color: #92400e;
}

.status.cancelled {
  background: #fee2e2;
  color: #991b1b;
}

.mono {
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 12px;
  word-break: break-all;
}

.empty {
  color: #64748b;
  text-align: center;
}
</style>
