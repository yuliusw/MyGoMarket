<template>
  <section>
    <div class="page-title">
      <div>
        <h1>应用市场</h1>
        <p>浏览已上架 RPA 应用，购买后会自动生成订单、扣款并发放订阅。</p>
      </div>
      <button class="btn" @click="showPublish = !showPublish">
        {{ showPublish ? '关闭发布' : '发布应用' }}
      </button>
    </div>

    <div v-if="showPublish" class="panel publish-panel">
      <div class="field">
        <label>应用名称</label>
        <input v-model="publishForm.name" placeholder="例如：发票批量识别机器人" />
      </div>
      <div class="field">
        <label>分类</label>
        <input v-model="publishForm.category" placeholder="财务 / 运营 / 数据处理" />
      </div>
      <div class="field">
        <label>标签，英文逗号分隔</label>
        <input v-model="publishForm.tags" placeholder="invoice,ocr,rpa" />
      </div>
      <div class="field">
        <label>安装包，仅支持 zip/gz/tgz</label>
        <input type="file" accept=".zip,.gz,.tgz" @change="selectFile" />
      </div>
      <button class="btn" :disabled="publishing" @click="submitPublish">
        {{ publishing ? '发布中...' : '提交发布' }}
      </button>
    </div>

    <div class="panel toolbar">
      <div class="field">
        <label>搜索</label>
        <input v-model="query.keyword" placeholder="应用名称" @keyup.enter="loadApps" />
      </div>
      <div class="field">
        <label>状态</label>
        <select v-model="query.status" @change="loadApps">
          <option value="published">已上架</option>
          <option value="off_shelved">已下架</option>
          <option value="">全部</option>
        </select>
      </div>
      <button class="btn secondary" @click="loadApps">刷新</button>
    </div>

    <p v-if="message" :class="messageType">{{ message }}</p>

    <div class="grid">
      <article v-for="app in apps" :key="app.app_id" class="panel app-card">
        <div class="card-head">
          <div>
            <h2>{{ app.name }}</h2>
            <p>{{ app.category || '未分类' }} · {{ formatDate(app.create_at) }}</p>
          </div>
          <span :class="['badge', app.status]">{{ app.status }}</span>
        </div>

        <div class="tags">
          <span v-for="tag in app.tags || []" :key="tag">{{ tag }}</span>
          <span v-if="!app.tags?.length">暂无标签</span>
        </div>

        <dl>
          <div>
            <dt>价格</dt>
            <dd>{{ priceOf(app) }} COIN</dd>
          </div>
          <div>
            <dt>文件</dt>
            <dd>{{ fileSizeOf(app) }}</dd>
          </div>
          <div>
            <dt>App ID</dt>
            <dd class="mono">{{ app.app_id }}</dd>
          </div>
        </dl>

        <div class="actions">
          <button class="btn" :disabled="app.status !== 'published' || purchasingId === app.app_id" @click="buy(app)">
            {{ purchasingId === app.app_id ? '购买中...' : '购买并发放' }}
          </button>
          <button class="btn secondary" @click="download(app)">下载</button>
          <button class="btn secondary" :disabled="app.status !== 'published'" @click="offShelf(app)">下架</button>
          <button class="btn danger" @click="remove(app)">删除</button>
        </div>
      </article>
    </div>

    <div class="pager">
      <button class="btn secondary" :disabled="query.page <= 1" @click="page(-1)">上一页</button>
      <span>第 {{ query.page }} 页，共 {{ total }} 个</span>
      <button class="btn secondary" :disabled="apps.length < query.page_size" @click="page(1)">下一页</button>
    </div>

    <aside v-if="lastOrder" class="panel result-panel">
      <strong>最近购买结果</strong>
      <p>订单状态：<span class="success">{{ lastOrder.status }}</span></p>
      <p>订单：<code>{{ lastOrder.order_id }}</code></p>
      <p>流水：<code>{{ lastOrder.tx_id }}</code></p>
      <p>订阅：<code>{{ lastOrder.subscription_id }}</code></p>
    </aside>
  </section>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import {
  deleteApp,
  listApps,
  offShelfApp,
  publishApp,
  purchaseApp,
  type MarketApp,
  type Order,
} from '../api'

const apps = ref<MarketApp[]>([])
const total = ref(0)
const showPublish = ref(false)
const publishing = ref(false)
const purchasingId = ref('')
const message = ref('')
const messageType = ref<'success' | 'error'>('success')
const lastOrder = ref<Order | null>(null)

const query = reactive({ page: 1, page_size: 12, keyword: '', status: 'published' })
const publishForm = reactive({ name: '', category: '', tags: '', file: null as File | null })

async function loadApps() {
  try {
    const res = await listApps(query)
    apps.value = res.data || []
    total.value = res.total || 0
  } catch (err) {
    notify(errorMessage(err, '应用列表加载失败'), 'error')
  }
}

function page(step: number) {
  query.page += step
  loadApps()
}

function selectFile(event: Event) {
  const input = event.target as HTMLInputElement
  publishForm.file = input.files?.[0] || null
}

async function submitPublish() {
  if (!publishForm.name || !publishForm.file) {
    notify('应用名称和安装包必填', 'error')
    return
  }
  publishing.value = true
  const form = new FormData()
  form.append('name', publishForm.name)
  form.append('category', publishForm.category)
  publishForm.tags
    .split(',')
    .map((tag) => tag.trim())
    .filter(Boolean)
    .forEach((tag) => form.append('tags', tag))
  form.append('app_file', publishForm.file)
  form.append('idempotency_key', `publish-${Date.now()}`)

  try {
    await publishApp(form)
    notify('发布成功')
    showPublish.value = false
    publishForm.name = ''
    publishForm.category = ''
    publishForm.tags = ''
    publishForm.file = null
    await loadApps()
  } catch (err) {
    notify(errorMessage(err, '发布失败'), 'error')
  } finally {
    publishing.value = false
  }
}

async function buy(app: MarketApp) {
  purchasingId.value = app.app_id
  try {
    const order = await purchaseApp(app.app_id, priceOf(app))
    lastOrder.value = order
    notify(`购买成功，订阅 ${order.subscription_id}`)
  } catch (err) {
    notify(errorMessage(err, '购买失败，请检查登录状态和钱包余额'), 'error')
  } finally {
    purchasingId.value = ''
  }
}

async function offShelf(app: MarketApp) {
  if (!confirm(`确认下架 ${app.name}？`)) return
  try {
    await offShelfApp(app.app_id)
    notify('已下架')
    await loadApps()
  } catch (err) {
    notify(errorMessage(err, '下架失败'), 'error')
  }
}

async function remove(app: MarketApp) {
  if (!confirm(`确认删除 ${app.name}？此操作不可逆。`)) return
  try {
    await deleteApp(app.app_id)
    notify('已删除')
    await loadApps()
  } catch (err) {
    notify(errorMessage(err, '删除失败'), 'error')
  }
}

function download(app: MarketApp) {
  window.open(`/api/v1/market/apps/${app.app_id}/download`, '_blank')
}

function priceOf(app: MarketApp) {
  const value = app.metadata?.price
  if (typeof value === 'number') return value.toFixed(4)
  if (typeof value === 'string' && value.trim()) {
    const parsed = Number(value)
    if (Number.isFinite(parsed)) return parsed.toFixed(4)
  }
  return '10.0000'
}

function fileSizeOf(app: MarketApp) {
  const value = app.metadata?.file_size
  if (typeof value !== 'number') return '未知'
  if (value < 1024) return `${value} B`
  if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} KB`
  return `${(value / 1024 / 1024).toFixed(1)} MB`
}

function formatDate(value: string) {
  if (!value) return '-'
  return new Date(value).toLocaleDateString()
}

function notify(text: string, type: 'success' | 'error' = 'success') {
  message.value = text
  messageType.value = type
}

function errorMessage(err: unknown, fallback: string) {
  if (typeof err === 'object' && err && 'message' in err) return String((err as { message: unknown }).message)
  return fallback
}

onMounted(loadApps)
</script>

<style scoped>
.publish-panel,
.toolbar {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr)) auto;
  gap: 14px;
  align-items: end;
  padding: 18px;
  margin-bottom: 18px;
}

.toolbar {
  grid-template-columns: minmax(220px, 1fr) 180px auto;
}

.grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
  gap: 18px;
}

.app-card {
  padding: 20px;
}

.card-head {
  display: flex;
  justify-content: space-between;
  gap: 14px;
}

h2 {
  margin: 0;
  color: #0f172a;
  font-size: 20px;
  font-weight: 800;
}

.card-head p {
  color: #64748b;
}

.badge {
  height: 26px;
  border-radius: 999px;
  padding: 3px 10px;
  background: #e2e8f0;
  color: #334155;
  font-size: 12px;
  font-weight: 800;
}

.badge.published {
  background: #dcfce7;
  color: #166534;
}

.badge.off_shelved {
  background: #fee2e2;
  color: #991b1b;
}

.tags {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin: 18px 0;
}

.tags span {
  border-radius: 999px;
  background: #eef2ff;
  color: #3730a3;
  padding: 4px 9px;
  font-size: 12px;
  font-weight: 700;
}

dl {
  display: grid;
  gap: 10px;
}

dl div {
  display: flex;
  justify-content: space-between;
  gap: 12px;
}

dt {
  color: #64748b;
}

dd {
  margin: 0;
  color: #0f172a;
  font-weight: 700;
  text-align: right;
}

.mono,
code {
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 12px;
  word-break: break-all;
}

.actions {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  margin-top: 20px;
}

.pager {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 14px;
  margin: 22px 0;
}

.result-panel {
  padding: 18px;
  margin-top: 18px;
}

@media (max-width: 900px) {
  .publish-panel,
  .toolbar {
    grid-template-columns: 1fr;
  }
}
</style>
