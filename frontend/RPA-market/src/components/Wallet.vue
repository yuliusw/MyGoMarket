<template>
  <section>
    <div class="page-title">
      <div>
        <h1>钱包</h1>
        <p>查看当前 COIN 余额，充值测试购买链路，确认流水快照。</p>
      </div>
      <button class="btn secondary" @click="loadWallet">刷新钱包</button>
    </div>

    <p v-if="message" :class="messageType">{{ message }}</p>

    <div class="wallet-grid">
      <div class="panel balance-card">
        <span>当前余额</span>
        <strong>{{ wallet?.balance || '--' }}</strong>
        <small>{{ wallet?.currency_code || 'COIN' }}</small>
        <p v-if="wallet" class="mono">{{ wallet.wallet_id }}</p>
      </div>

      <div class="panel topup-card">
        <h2>测试充值</h2>
        <div class="field">
          <label>充值金额</label>
          <input v-model="topupAmount" placeholder="100.0000" />
        </div>
        <button class="btn" :disabled="!wallet || loading" @click="topup">
          {{ loading ? '处理中...' : '充值' }}
        </button>
      </div>
    </div>

    <div class="panel tx-panel">
      <div class="section-head">
        <h2>最近流水</h2>
        <button class="btn secondary" :disabled="!wallet" @click="loadTransactions">刷新流水</button>
      </div>

      <div class="table-wrap">
        <table>
          <thead>
            <tr>
              <th>时间</th>
              <th>类型</th>
              <th>金额</th>
              <th>余额快照</th>
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
              <td class="mono">{{ tx.reference_id || '-' }}</td>
              <td class="mono">{{ tx.idempotency_key || '-' }}</td>
            </tr>
            <tr v-if="!transactions.length">
              <td colspan="6" class="empty">暂无流水</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import {
  creditWallet,
  getMyWallet,
  listWalletTransactions,
  type Wallet,
  type WalletTransaction,
} from '../api'

const wallet = ref<Wallet | null>(null)
const transactions = ref<WalletTransaction[]>([])
const topupAmount = ref('100.0000')
const loading = ref(false)
const message = ref('')
const messageType = ref<'success' | 'error'>('success')

async function loadWallet() {
  try {
    wallet.value = await getMyWallet()
    await loadTransactions()
  } catch (err) {
    notify(errorMessage(err, '钱包加载失败，请先登录'), 'error')
  }
}

async function loadTransactions() {
  if (!wallet.value) return
  try {
    const res = await listWalletTransactions(wallet.value.wallet_id)
    transactions.value = res.items || []
  } catch (err) {
    notify(errorMessage(err, '流水加载失败'), 'error')
  }
}

async function topup() {
  if (!wallet.value) return
  loading.value = true
  try {
    const res = await creditWallet(wallet.value.wallet_id, topupAmount.value)
    wallet.value = res.wallet
    notify('充值成功')
    await loadTransactions()
  } catch (err) {
    notify(errorMessage(err, '充值失败'), 'error')
  } finally {
    loading.value = false
  }
}

function formatDate(value: string) {
  if (!value) return '-'
  return new Date(value).toLocaleString()
}

function notify(text: string, type: 'success' | 'error' = 'success') {
  message.value = text
  messageType.value = type
}

function errorMessage(err: unknown, fallback: string) {
  if (typeof err === 'object' && err && 'message' in err) return String((err as { message: unknown }).message)
  return fallback
}

onMounted(loadWallet)
</script>

<style scoped>
.wallet-grid {
  display: grid;
  grid-template-columns: minmax(260px, 360px) 1fr;
  gap: 18px;
  margin-bottom: 18px;
}

.balance-card,
.topup-card,
.tx-panel {
  padding: 22px;
}

.balance-card span,
.balance-card small {
  color: #64748b;
}

.balance-card strong {
  display: block;
  margin: 6px 0;
  color: #0f172a;
  font-size: 52px;
  font-weight: 900;
  letter-spacing: -0.06em;
}

.topup-card {
  display: grid;
  align-content: start;
  gap: 14px;
}

.section-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 16px;
}

h2 {
  margin: 0;
  color: #0f172a;
  font-size: 20px;
  font-weight: 800;
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

.empty {
  color: #64748b;
  text-align: center;
}

@media (max-width: 900px) {
  .wallet-grid {
    grid-template-columns: 1fr;
  }
}
</style>
