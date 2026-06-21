<template>
  <section>
    <div class="page-title">
      <div>
        <h1>管理查询</h1>
        <p>全局查看虚拟账户、钱包流水、订单和审计变更日志。</p>
      </div>
      <button class="btn secondary" :disabled="loading" @click="loadCurrent">
        {{ loading ? '加载中...' : '刷新' }}
      </button>
    </div>

    <p v-if="message" class="error">{{ message }}</p>

    <div class="panel admin-card">
      <div class="tabs">
        <button
          v-for="tab in tabs"
          :key="tab.key"
          :class="['tab', { active: activeTab === tab.key }]"
          @click="switchTab(tab.key)"
        >
          {{ tab.name }}
        </button>
      </div>

      <form class="filters" @submit.prevent="applyFilters">
        <div v-for="field in activeFields" :key="field.key" class="field">
          <label>{{ field.label }}</label>
          <select v-if="field.options" v-model="filters[field.key]">
            <option value="">全部</option>
            <option v-for="option in field.options" :key="option" :value="option">{{ option }}</option>
          </select>
          <input v-else v-model="filters[field.key]" :placeholder="field.placeholder" />
        </div>
        <div class="actions">
          <button class="btn" :disabled="loading" type="submit">查询</button>
          <button class="btn secondary" type="button" @click="resetFilters">重置</button>
        </div>
      </form>
    </div>

    <div class="panel result-card">
      <div class="result-head">
        <div>
          <h2>{{ activeName }}</h2>
          <p class="muted">共 {{ total }} 条，当前第 {{ page }} 页</p>
        </div>
        <div class="pager">
          <button class="btn secondary" :disabled="page <= 1 || loading" @click="goPage(page - 1)">上一页</button>
          <button class="btn secondary" :disabled="!hasNext || loading" @click="goPage(page + 1)">下一页</button>
        </div>
      </div>

      <div class="table-wrap">
        <table v-if="activeTab === 'virtualOrders'">
          <thead>
            <tr>
              <th>更新时间</th>
              <th>账户</th>
              <th>所有者</th>
              <th>类型</th>
              <th>余额</th>
              <th>状态</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="wallet in wallets" :key="wallet.wallet_id">
              <td>{{ formatDate(wallet.updated_at) }}</td>
              <td class="mono">{{ wallet.wallet_id }}</td>
              <td class="mono">{{ wallet.owner_id }}</td>
              <td>{{ wallet.owner_type }}</td>
              <td>{{ wallet.balance }} {{ wallet.currency_code }}</td>
              <td><span :class="['status', wallet.status]">{{ wallet.status }}</span></td>
            </tr>
            <tr v-if="!wallets.length"><td colspan="6" class="empty">暂无数据</td></tr>
          </tbody>
        </table>

        <table v-else-if="activeTab === 'transactions'">
          <thead>
            <tr>
              <th>时间</th>
              <th>类型</th>
              <th>金额</th>
              <th>余额快照</th>
              <th>钱包</th>
              <th>关联单据</th>
              <th>幂等键</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="tx in transactions" :key="tx.tx_id">
              <td>{{ formatDate(tx.created_at) }}</td>
              <td>{{ tx.tx_type }}</td>
              <td :class="tx.amount.startsWith('-') ? 'error' : 'success'">{{ tx.amount }}</td>
              <td>{{ tx.balance_after }}</td>
              <td class="mono">{{ tx.wallet_id }}</td>
              <td class="mono">{{ tx.reference_id || '-' }}</td>
              <td class="mono">{{ tx.idempotency_key || '-' }}</td>
            </tr>
            <tr v-if="!transactions.length"><td colspan="7" class="empty">暂无数据</td></tr>
          </tbody>
        </table>

        <table v-else-if="activeTab === 'orders'">
          <thead>
            <tr>
              <th>创建时间</th>
              <th>状态</th>
              <th>金额</th>
              <th>用户</th>
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
              <td class="mono">{{ order.user_id }}</td>
              <td class="mono">{{ order.app_id }}</td>
              <td class="mono">{{ order.order_id }}</td>
              <td class="mono">{{ order.tx_id || '-' }}</td>
              <td class="mono">{{ order.subscription_id || '-' }}</td>
            </tr>
            <tr v-if="!orders.length"><td colspan="8" class="empty">暂无数据</td></tr>
          </tbody>
        </table>

        <table v-else>
          <thead>
            <tr>
              <th>时间</th>
              <th>事件</th>
              <th>操作者</th>
              <th>资源</th>
              <th>Trace</th>
              <th>错误</th>
              <th>元数据</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="log in changeLogs" :key="log.event_id">
              <td>{{ formatDate(log.created_at) }}</td>
              <td>{{ log.event_type }}</td>
              <td class="mono">{{ log.actor_id || '-' }}</td>
              <td class="mono">{{ log.resource || '-' }}</td>
              <td class="mono">{{ log.trace_id || '-' }}</td>
              <td class="error">{{ log.error || '-' }}</td>
              <td class="mono metadata">{{ log.metadata || '{}' }}</td>
            </tr>
            <tr v-if="!changeLogs.length"><td colspan="7" class="empty">暂无数据</td></tr>
          </tbody>
        </table>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import {
  listAdminChangeLogs,
  listAdminOrders,
  listAdminVirtualOrders,
  listAdminWalletTransactions,
  type ChangeLog,
  type Order,
  type Wallet,
  type WalletTransaction,
} from '../api'

type TabKey = 'virtualOrders' | 'transactions' | 'orders' | 'changeLogs'

interface FilterField {
  key: string
  label: string
  placeholder?: string
  options?: string[]
}

const tabs: Array<{ key: TabKey; name: string }> = [
  { key: 'virtualOrders', name: '虚拟账户' },
  { key: 'transactions', name: '流水' },
  { key: 'orders', name: '订单' },
  { key: 'changeLogs', name: '变更日志' },
]

const filterFields: Record<TabKey, FilterField[]> = {
  virtualOrders: [
    { key: 'owner_id', label: '所有者 ID', placeholder: '用户或团队 UUID' },
    { key: 'owner_type', label: '所有者类型', options: ['user', 'group'] },
    { key: 'currency_code', label: '币种', placeholder: 'COIN' },
    { key: 'status', label: '状态', options: ['active', 'frozen', 'closed'] },
  ],
  transactions: [
    { key: 'wallet_id', label: '钱包 ID', placeholder: '钱包 UUID' },
    { key: 'reference_id', label: '关联单据', placeholder: '订单/业务 UUID' },
    { key: 'tx_type', label: '流水类型', options: ['recharge', 'consume', 'refund', 'transfer_in', 'transfer_out'] },
    { key: 'from', label: '开始时间', placeholder: '2026-06-21 或 RFC3339' },
    { key: 'to', label: '结束时间', placeholder: '2026-06-22 或 RFC3339' },
  ],
  orders: [
    { key: 'user_id', label: '用户 ID', placeholder: '用户 UUID' },
    { key: 'app_id', label: '应用 ID', placeholder: '应用 UUID' },
    { key: 'wallet_id', label: '钱包 ID', placeholder: '钱包 UUID' },
    { key: 'status', label: '状态', options: ['pending', 'paid', 'cancelled'] },
    { key: 'currency_code', label: '币种', placeholder: 'COIN' },
    { key: 'from', label: '开始时间', placeholder: '2026-06-21 或 RFC3339' },
    { key: 'to', label: '结束时间', placeholder: '2026-06-22 或 RFC3339' },
  ],
  changeLogs: [
    { key: 'event_type', label: '事件类型', placeholder: 'role_permissions_updated' },
    { key: 'actor_id', label: '操作者', placeholder: '用户 UUID' },
    { key: 'resource', label: '资源', placeholder: '资源 ID' },
    { key: 'trace_id', label: 'Trace ID', placeholder: '请求追踪 ID' },
    { key: 'from', label: '开始时间', placeholder: '2026-06-21 或 RFC3339' },
    { key: 'to', label: '结束时间', placeholder: '2026-06-22 或 RFC3339' },
  ],
}

const activeTab = ref<TabKey>('virtualOrders')
const filters = reactive<Record<string, string>>({})
const page = ref(1)
const pageSize = ref(20)
const total = ref(0)
const loading = ref(false)
const message = ref('')
const wallets = ref<Wallet[]>([])
const transactions = ref<WalletTransaction[]>([])
const orders = ref<Order[]>([])
const changeLogs = ref<ChangeLog[]>([])

const activeFields = computed(() => filterFields[activeTab.value])
const activeName = computed(() => tabs.find((tab) => tab.key === activeTab.value)?.name || '')
const hasNext = computed(() => page.value * pageSize.value < total.value)

async function loadCurrent() {
  loading.value = true
  message.value = ''
  try {
    const params = queryParams()
    if (activeTab.value === 'virtualOrders') {
      const res = await listAdminVirtualOrders(params)
      wallets.value = res.items || []
      total.value = res.total || 0
    } else if (activeTab.value === 'transactions') {
      const res = await listAdminWalletTransactions(params)
      transactions.value = res.items || []
      total.value = res.total || 0
    } else if (activeTab.value === 'orders') {
      const res = await listAdminOrders(params)
      orders.value = res.items || []
      total.value = res.total || 0
    } else {
      const res = await listAdminChangeLogs(params)
      changeLogs.value = res.items || []
      total.value = res.total || 0
    }
  } catch (err) {
    message.value = errorMessage(err, '管理数据加载失败，请确认账号具备 role:manage 权限')
  } finally {
    loading.value = false
  }
}

function queryParams() {
  const params: Record<string, string | number> = { page: page.value, page_size: pageSize.value }
  for (const field of activeFields.value) {
    const value = filters[field.key]?.trim()
    if (value) params[field.key] = value
  }
  return params
}

function switchTab(tab: TabKey) {
  activeTab.value = tab
  clearFilters()
  page.value = 1
  void loadCurrent()
}

function applyFilters() {
  page.value = 1
  void loadCurrent()
}

function resetFilters() {
  clearFilters()
  page.value = 1
  void loadCurrent()
}

function clearFilters() {
  for (const key of Object.keys(filters)) delete filters[key]
}

function goPage(nextPage: number) {
  page.value = nextPage
  void loadCurrent()
}

function formatDate(value: string) {
  if (!value) return '-'
  return new Date(value).toLocaleString()
}

function errorMessage(err: unknown, fallback: string) {
  if (typeof err === 'object' && err && 'message' in err) return String((err as { message: unknown }).message)
  return fallback
}

onMounted(loadCurrent)
</script>

<style scoped>
.admin-card,
.result-card {
  padding: 22px;
}

.admin-card {
  margin-bottom: 18px;
}

.tabs {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  margin-bottom: 18px;
}

.tab {
  border: 1px solid #cbd5e1;
  border-radius: 999px;
  padding: 9px 14px;
  background: #fff;
  color: #334155;
  font-weight: 800;
}

.tab.active {
  border-color: #0f172a;
  background: #0f172a;
  color: #fff;
}

.filters {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(210px, 1fr));
  gap: 14px;
  align-items: end;
}

.actions,
.pager {
  display: flex;
  gap: 10px;
}

.result-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 18px;
}

.result-head h2 {
  margin: 0;
  color: #0f172a;
  font-size: 20px;
  font-weight: 900;
}

.result-head p {
  margin: 4px 0 0;
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

.mono {
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 12px;
  word-break: break-all;
}

.metadata {
  max-width: 360px;
}

.status {
  border-radius: 999px;
  padding: 4px 9px;
  background: #e2e8f0;
  color: #334155;
  font-size: 12px;
  font-weight: 800;
}

.status.active,
.status.paid {
  background: #dcfce7;
  color: #166534;
}

.status.pending {
  background: #fef3c7;
  color: #92400e;
}

.status.cancelled,
.status.frozen,
.status.closed {
  background: #fee2e2;
  color: #991b1b;
}

.empty {
  color: #64748b;
  text-align: center;
}

@media (max-width: 760px) {
  .result-head {
    align-items: stretch;
    flex-direction: column;
  }

  .pager .btn,
  .actions .btn {
    flex: 1;
  }
}
</style>
