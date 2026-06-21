<template>
  <div class="market-manager">
    <div class="app-sidebar">
      <div class="sidebar-header">
        <h3>应用市场管理</h3>
        <button @click="openPublishForm" class="btn-xs btn-blue">+ 发布新应用</button>
      </div>

      <div class="filter-section">
        <input
          v-model="listQuery.keyword"
          @keyup.enter="loadApps"
          placeholder="搜索应用名称..."
          class="filter-input"
        />
        <div class="filter-row">
          <select v-model="listQuery.status" @change="loadApps" class="filter-select">
            <option value="">全部状态</option>
            <option value="published">已上架</option>
            <option value="off_shelved">已下架</option>
          </select>
          <button @click="loadApps" class="btn-xs btn-outline">查询</button>
        </div>
      </div>

      <ul class="app-list">
        <li
          v-for="app in apps"
          :key="app.app_id"
          :class="{ active: currentApp?.app_id === app.app_id && !isPublishing }"
          @click="selectApp(app)"
        >
          <div class="app-item-title">
            <span class="name">{{ app.name }}</span>
            <span :class="['status-badge', app.status]">{{
              app.status === 'published' ? '上架' : '下架'
            }}</span>
          </div>
          <div class="app-item-meta">
            {{ app.category || '未分类' }} · {{ formatDate(app.create_at) }}
          </div>
        </li>
      </ul>
      <div class="pagination">
        <button :disabled="listQuery.page <= 1" @click="changePage(-1)">上一页</button>
        <span>{{ listQuery.page }}</span>
        <button @click="changePage(1)">下一页</button>
      </div>
    </div>

    <div class="app-detail-container">
      <div v-if="isPublishing" class="publish-section">
        <h2>🚀 发布新应用</h2>
        <div class="form-group">
          <label>应用名称 <span class="required">*</span></label>
          <input v-model="publishForm.name" placeholder="输入应用名称" />
        </div>
        <div class="form-group">
          <label>应用分类</label>
          <input v-model="publishForm.category" placeholder="如：效率工具、报表处理" />
        </div>
        <div class="form-group">
          <label>标签 (用逗号分隔)</label>
          <input v-model="publishForm.tagsStr" placeholder="如：Excel, 自动化" />
        </div>
        <div class="form-group">
          <label>安装包文件 (最大 100MB) <span class="required">*</span></label>
          <input type="file" @change="handleFileSelect" accept=".zip,.rar,.exe,.apk" />
        </div>
        <button @click="submitPublish" class="btn-md btn-blue" :disabled="isUploading">
          {{ isUploading ? '上传中...' : '确认发布' }}
        </button>
      </div>

      <div v-else-if="currentApp" class="detail-section">
        <div class="detail-header">
          <h2>{{ currentApp.name }}</h2>
          <div class="actions">
            <button @click="handleDownload" class="btn-xs btn-blue">📥 下载文件</button>
            <button
              v-if="currentApp.status === 'published'"
              @click="handleOffShelf"
              class="btn-xs btn-warn"
            >
              🚫 下架
            </button>
            <button @click="handleDelete" class="btn-xs btn-danger">🗑️ 彻底删除</button>
          </div>
        </div>

        <div class="app-meta-cards">
          <div class="card">
            <strong>App ID:</strong> <br />
            <span class="text-xs">{{ currentApp.app_id }}</span>
          </div>
          <div class="card">
            <strong>文件大小:</strong> <br />
            {{ formatBytes(currentApp.metadata?.file_size) }}
          </div>
          <div class="card">
            <strong>更新时间:</strong> <br />
            {{ formatDate(currentApp.update_at) }}
          </div>
        </div>

        <hr class="divider" />

        <h3>基础信息修改</h3>
        <div class="form-group">
          <label>应用名称</label>
          <input v-model="editForm.name" />
        </div>
        <div class="form-group">
          <label>应用分类</label>
          <input v-model="editForm.category" />
        </div>
        <div class="form-group">
          <label>标签 (用逗号分隔)</label>
          <input v-model="editForm.tagsStr" />
        </div>
        <div class="form-group">
          <label>状态</label>
          <select v-model="editForm.status">
            <option value="published">已上架 (published)</option>
            <option value="off_shelved">已下架 (off_shelved)</option>
          </select>
        </div>
        <button @click="submitUpdate" class="btn-xs btn-blue">保存修改</button>
      </div>

      <div class="empty-notice" v-else>
        <p>⬅️ 请在左侧选择一个应用，或点击“发布新应用”</p>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import request from '../utils/request' // 替换为你的 Axios 实例路径

// --- 数据接口定义 ---
interface AppItem {
  app_id: string
  name: string
  developer_id: string
  category: string
  tags: string[]
  metadata: Record<string, any>
  status: string
  create_at: string
  update_at: string
}

// --- 状态变量 ---
const apps = ref<AppItem[]>([])
const currentApp = ref<AppItem | null>(null)
const isPublishing = ref(false)
const isUploading = ref(false)

// 列表查询参数
const listQuery = reactive({
  page: 1,
  page_size: 10,
  keyword: '',
  category: '',
  status: 'published', // 默认看上架的
})

// 发布表单数据
const publishForm = reactive({
  name: '',
  category: '',
  tagsStr: '',
  file: null as File | null,
})

// 编辑表单数据
const editForm = reactive({
  name: '',
  category: '',
  tagsStr: '',
  status: '',
})

// --- API 方法 ---

// 1. 获取列表
const loadApps = async () => {
  try {
    const res: any = await request.get('/market/apps', { params: listQuery })
    apps.value = res.data || []
  } catch (err: any) {
    alert(err.response?.data?.error || '获取列表失败')
  }
}

// 分页切换
const changePage = (step: number) => {
  listQuery.page += step
  loadApps()
}

// 选择应用查看详情
const selectApp = (app: AppItem) => {
  isPublishing.value = false
  currentApp.value = app
  // 填充编辑表单
  editForm.name = app.name
  editForm.category = app.category
  editForm.tagsStr = app.tags ? app.tags.join(',') : ''
  editForm.status = app.status
}

// 切换到发布模式
const openPublishForm = () => {
  currentApp.value = null
  isPublishing.value = true
  publishForm.name = ''
  publishForm.category = ''
  publishForm.tagsStr = ''
  publishForm.file = null
}

// 处理文件选择
const handleFileSelect = (e: Event) => {
  const target = e.target as HTMLInputElement
  const file = target.files?.[0] // 使用可选链安全提取
  if (file) {
    publishForm.file = file
  }
}

// 2. 提交发布 (带有文件的 FormData)
const submitPublish = async () => {
  if (!publishForm.name || !publishForm.file) {
    alert('应用名称和安装包文件是必填项！')
    return
  }

  isUploading.value = true
  const formData = new FormData()
  formData.append('name', publishForm.name)
  formData.append('category', publishForm.category)

  // 处理 tags 数组 (按逗号分隔后追加)
  if (publishForm.tagsStr) {
    const tags = publishForm.tagsStr
      .split(',')
      .map((t) => t.trim())
      .filter((t) => t)
    tags.forEach((tag) => formData.append('tags', tag))
  }

  formData.append('app_file', publishForm.file)

  try {
    await request.post('/market/apps', formData, {
      headers: { 'Content-Type': 'multipart/form-data' },
    })
    alert('发布成功！')
    isPublishing.value = false
    loadApps()
  } catch (err: any) {
    alert(err.response?.data?.error || '发布失败')
  } finally {
    isUploading.value = false
  }
}

// 3. 提交修改 (普通 JSON)
const submitUpdate = async () => {
  if (!currentApp.value) return
  try {
    const tags = editForm.tagsStr
      .split(',')
      .map((t) => t.trim())
      .filter((t) => t)
    const payload = {
      name: editForm.name,
      category: editForm.category,
      tags: tags,
      status: editForm.status,
    }
    await request.put(`/market/apps/${currentApp.value.app_id}`, payload)
    alert('信息修改成功！')
    loadApps() // 刷新列表
  } catch (err: any) {
    alert(err.response?.data?.error || '修改失败')
  }
}

// 4. 下载文件
const handleDownload = () => {
  if (!currentApp.value) return
  // 直接通过浏览器跳转到下载直链接口 (这样可以触发浏览器自带的下载行为，且携带当前域名 Cookie(如果同域) )
  // 注意：如果是前后端分离，推荐拼接 baseURL
  const baseURL = request.defaults.baseURL || ''
  window.open(`${baseURL}/market/apps/${currentApp.value.app_id}/download`, '_blank')
}

// 5. 下架
const handleOffShelf = async () => {
  if (!currentApp.value || !confirm('确定要下架该应用吗？')) return
  try {
    // 我们可以直接复用 Update 接口，或者如果你后端保留了独立接口，可以调独立的
    await request.put(`/market/apps/${currentApp.value.app_id}`, { status: 'off_shelved' })
    alert('已成功下架！')
    loadApps()
    selectApp({ ...currentApp.value, status: 'off_shelved' }) // 更新当前视图
  } catch (err: any) {
    alert('下架失败')
  }
}

// 6. 彻底删除
const handleDelete = async () => {
  if (!currentApp.value || !confirm('警告：这将彻底删除应用记录和物理文件，不可逆！确认删除？'))
    return
  try {
    await request.delete(`/market/apps/${currentApp.value.app_id}`)
    alert('删除成功！')
    currentApp.value = null
    loadApps()
  } catch (err: any) {
    alert(err.response?.data?.error || '删除失败')
  }
}

// --- 辅助工具函数 ---
const formatDate = (dateStr: string) => {
  if (!dateStr) return '-'
  const d = new Date(dateStr)
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
}

const formatBytes = (bytes?: number) => {
  if (!bytes) return '未知大小'
  if (bytes < 1024) return bytes + ' B'
  else if (bytes < 1048576) return (bytes / 1024).toFixed(1) + ' KB'
  else return (bytes / 1048576).toFixed(1) + ' MB'
}

onMounted(() => {
  loadApps()
})
</script>

<style scoped>
.market-manager {
  display: flex;
  background: white;
  min-height: 75vh;
  border-radius: 8px;
  overflow: hidden;
  box-shadow: 0 2px 10px rgba(0, 0, 0, 0.05);
}

/* 左侧栏样式 */
.app-sidebar {
  width: 320px;
  border-right: 1px solid #eee;
  display: flex;
  flex-direction: column;
  background: #fcfcfc;
}

.sidebar-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 15px 20px;
  border-bottom: 1px solid #eee;
}

.filter-section {
  padding: 15px 20px;
  border-bottom: 1px solid #eee;
  background: #fff;
}
.filter-input {
  width: 100%;
  padding: 8px;
  margin-bottom: 10px;
  border: 1px solid #ddd;
  border-radius: 4px;
}
.filter-row {
  display: flex;
  gap: 10px;
}
.filter-select {
  flex: 1;
  padding: 6px;
  border: 1px solid #ddd;
  border-radius: 4px;
}

.app-list {
  list-style: none;
  padding: 0;
  margin: 0;
  flex: 1;
  overflow-y: auto;
}
.app-list li {
  padding: 15px 20px;
  border-bottom: 1px solid #f1f1f1;
  cursor: pointer;
  transition: background 0.2s;
}
.app-list li:hover {
  background: #f5f8fa;
}
.app-list li.active {
  background: #eef6ff;
  border-left: 4px solid #009ef7;
}

.app-item-title {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 5px;
}
.app-item-title .name {
  font-weight: bold;
  color: #333;
}
.app-item-meta {
  font-size: 0.8rem;
  color: #888;
}

.status-badge {
  font-size: 0.7rem;
  padding: 2px 6px;
  border-radius: 12px;
}
.status-badge.published {
  background: #e8fff3;
  color: #50cd89;
}
.status-badge.off_shelved {
  background: #fff5f8;
  color: #f1416c;
}

.pagination {
  display: flex;
  justify-content: space-between;
  padding: 10px 20px;
  border-top: 1px solid #eee;
  background: #fff;
}

/* 右侧内容区样式 */
.app-detail-container {
  flex: 1;
  padding: 30px;
  overflow-y: auto;
}

.publish-section h2,
.detail-header h2 {
  margin-top: 0;
  margin-bottom: 20px;
  color: #181c32;
}

.detail-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  border-bottom: 1px dashed #eee;
  padding-bottom: 15px;
  margin-bottom: 20px;
}
.actions {
  display: flex;
  gap: 10px;
}

.app-meta-cards {
  display: flex;
  gap: 15px;
  margin-bottom: 25px;
}
.card {
  flex: 1;
  background: #f8f9fa;
  padding: 15px;
  border-radius: 6px;
  font-size: 0.9rem;
  color: #333;
}
.text-xs {
  font-size: 0.75rem;
  word-break: break-all;
  color: #777;
}

.divider {
  border: 0;
  border-top: 2px solid #f1f1f1;
  margin: 30px 0;
}

.form-group {
  margin-bottom: 15px;
  max-width: 500px;
}
.form-group label {
  display: block;
  margin-bottom: 6px;
  font-weight: bold;
  font-size: 0.9rem;
}
.form-group input,
.form-group select {
  width: 100%;
  padding: 10px;
  border: 1px solid #ddd;
  border-radius: 4px;
}
.required {
  color: #f1416c;
}

/* 按钮样式 */
.btn-xs {
  padding: 6px 12px;
  font-size: 0.85rem;
  border: none;
  border-radius: 4px;
  cursor: pointer;
  color: white;
}
.btn-md {
  padding: 10px 20px;
  font-size: 1rem;
  border: none;
  border-radius: 4px;
  cursor: pointer;
  color: white;
}
.btn-blue {
  background: #009ef7;
}
.btn-blue:disabled {
  background: #9cd4f5;
  cursor: not-allowed;
}
.btn-outline {
  background: white;
  border: 1px solid #ddd;
  color: #333;
}
.btn-warn {
  background: #ffc107;
  color: #333;
}
.btn-danger {
  background: #f1416c;
}

.empty-notice {
  height: 100%;
  display: flex;
  justify-content: center;
  align-items: center;
  color: #999;
}
</style>
