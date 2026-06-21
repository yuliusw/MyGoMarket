<template>
  <div class="profile-manager">
    <!-- 左侧：头像与基本状态 -->
    <div class="profile-sidebar">
      <h3>个人资料</h3>
      <div class="avatar-section">
        <img :src="profile.avatar_url || defaultAvatar" alt="User Avatar" class="avatar-img" />
        <!-- 隐藏的 file input，通过 label 触发 -->
        <label class="btn-xs btn-blue upload-label">
          更换头像
          <input type="file" accept="image/*" @change="handleAvatarUpload" style="display: none" />
        </label>
      </div>
      <div class="user-meta" v-if="profile.user_id">
        <p>
          <strong>ID:</strong> <br />
          <span class="text-muted">{{ profile.user_id }}</span>
        </p>
        <p>
          <strong>注册时间:</strong> <br />
          <span class="text-muted">{{ formatDate(profile.created_at) }}</span>
        </p>
      </div>
    </div>

    <!-- 右侧：资料编辑与安全设置 -->
    <div class="profile-detail" v-if="profile.user_id">
      <!-- 基础信息编辑 -->
      <div class="detail-section">
        <h2>基础信息</h2>
        <div class="form-group">
          <label>用户名</label>
          <input v-model="editForm.username" placeholder="请输入用户名" />
        </div>
        <div class="form-group">
          <label>邮箱</label>
          <input v-model="editForm.email" placeholder="请输入邮箱地址" />
        </div>
        <button @click="handleUpdateProfile" class="btn-xs btn-blue">保存基本信息</button>
      </div>

      <hr class="divider" />

      <!-- 修改密码 -->
      <div class="detail-section">
        <h2 class="text-danger">安全设置</h2>
        <div class="form-group">
          <label>当前密码</label>
          <input type="password" v-model="pwdForm.old_password" placeholder="请输入当前密码" />
        </div>
        <div class="form-group">
          <label>新密码</label>
          <input
            type="password"
            v-model="pwdForm.new_password"
            placeholder="请输入新密码 (至少6位)"
          />
        </div>
        <button @click="handleUpdatePassword" class="btn-xs btn-danger">确认修改密码</button>
      </div>
    </div>

    <div class="empty-notice" v-else>
      <p>加载中...</p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
// 替换为你的实际 request 工具路径
import request from '../utils/request'

const defaultAvatar = 'https://cube.elemecdn.com/3/7c/3ea6beec64369c2642b92c6726f1epng.png'

interface UserProfile {
  user_id: string
  username: string
  email: string
  avatar_url: string
  created_at: string
}

const profile = ref<UserProfile>({
  user_id: '',
  username: '',
  email: '',
  avatar_url: '',
  created_at: '',
})

// 用于双向绑定的表单数据
const editForm = reactive({
  username: '',
  email: '',
})

const pwdForm = reactive({
  old_password: '',
  new_password: '',
})

// 1. 初始化获取用户信息
const loadProfile = async () => {
  try {
    const res: any = await request.get('/iam/profile')
    profile.value = res
    // 同步到编辑表单
    editForm.username = res.username
    editForm.email = res.email
  } catch (err: any) {
    alert(err.response?.data?.error || err.error || '获取用户信息失败')
  }
}

// 2. 更新基础信息
const handleUpdateProfile = async () => {
  if (!editForm.username || !editForm.email) {
    alert('用户名和邮箱不能为空')
    return
  }
  try {
    await request.put('/iam/profile', {
      username: editForm.username,
      email: editForm.email,
    })
    alert('个人信息更新成功！')
    loadProfile() // 重新加载获取最新状态
  } catch (err: any) {
    alert(err.response?.data?.error || err.error || '更新失败')
  }
}

// 3. 上传头像
const handleAvatarUpload = async (e: Event) => {
  const target = e.target as HTMLInputElement
  if (!target.files || target.files.length === 0) return

  const file = target.files[0] as File
  const formData = new FormData()
  formData.append('avatar', file) // 这里的 key 必须和后端 c.FormFile("avatar") 一致

  try {
    const res: any = await request.post('/iam/profile/avatar', formData, {
      headers: {
        'Content-Type': 'multipart/form-data',
      },
    })
    alert('头像上传成功！')
    // 直接使用后端返回的 Minio 预览链接更新视图，或重新 loadProfile()
    profile.value.avatar_url = res.avatar_url
    target.value = '' // 清空 input 状态
  } catch (err: any) {
    alert(err.response?.data?.error || err.error || '头像上传失败')
  }
}

// 4. 修改密码
const handleUpdatePassword = async () => {
  if (!pwdForm.old_password || pwdForm.new_password.length < 6) {
    alert('请正确填写当前密码，且新密码至少为6位')
    return
  }
  if (!confirm('修改密码后需要重新登录，确认修改吗？')) return

  try {
    await request.put('/iam/profile/password', {
      old_password: pwdForm.old_password,
      new_password: pwdForm.new_password,
    })
    alert('密码修改成功，请重新登录！')
    // 清空表单
    pwdForm.old_password = ''
    pwdForm.new_password = ''
    // 跳转到登录页 (请根据你的路由调整)
    // router.push('/login')
    window.location.href = '/login'
  } catch (err: any) {
    alert(err.response?.data?.error || err.error || '密码修改失败')
  }
}

// 辅助函数：格式化时间
const formatDate = (dateStr: string) => {
  if (!dateStr) return '-'
  return new Date(dateStr).toLocaleDateString()
}

onMounted(() => {
  loadProfile()
})
</script>

<style scoped>
.profile-manager {
  display: flex;
  background: white;
  min-height: 70vh;
  border-radius: 8px;
  overflow: hidden;
  box-shadow: 0 2px 10px rgba(0, 0, 0, 0.05);
}

/* 左侧边栏 */
.profile-sidebar {
  width: 250px;
  border-right: 1px solid #eee;
  padding: 20px;
  display: flex;
  flex-direction: column;
  align-items: center;
  background: #f8f9fa;
}

.avatar-section {
  display: flex;
  flex-direction: column;
  align-items: center;
  margin-top: 20px;
  margin-bottom: 30px;
}

.avatar-img {
  width: 120px;
  height: 120px;
  border-radius: 50%;
  object-fit: cover;
  border: 4px solid #fff;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.1);
  margin-bottom: 15px;
}

.upload-label {
  display: inline-block;
  cursor: pointer;
  text-align: center;
}

.user-meta {
  width: 100%;
  font-size: 0.9rem;
  line-height: 1.5;
}

.text-muted {
  color: #6c757d;
  font-size: 0.85rem;
  word-break: break-all;
}

/* 右侧详情 */
.profile-detail {
  flex: 1;
  padding: 30px;
}

.detail-section {
  margin-bottom: 30px;
}

.detail-section h2 {
  font-size: 1.25rem;
  margin-bottom: 20px;
  padding-bottom: 10px;
  border-bottom: 1px dashed #eee;
}

.text-danger {
  color: #f1416c;
}

.form-group {
  margin-bottom: 15px;
  max-width: 400px;
}

.form-group label {
  display: block;
  margin-bottom: 5px;
  font-weight: bold;
  color: #333;
  font-size: 0.9rem;
}

.form-group input {
  width: 100%;
  padding: 10px;
  border: 1px solid #ddd;
  border-radius: 4px;
  font-size: 1rem;
}

.form-group input:focus {
  outline: none;
  border-color: #009ef7;
}

.divider {
  border: 0;
  border-top: 2px solid #f1f1f1;
  margin: 30px 0;
}

/* 按钮通用样式 (继承你之前的风格) */
.btn-xs {
  padding: 8px 16px;
  border: none;
  border-radius: 4px;
  cursor: pointer;
  color: white;
  font-size: 0.9rem;
  transition: opacity 0.2s;
}

.btn-xs:hover {
  opacity: 0.8;
}

.btn-blue {
  background: #009ef7;
}

.btn-danger {
  background: #f1416c;
}

.empty-notice {
  flex: 1;
  display: flex;
  justify-content: center;
  align-items: center;
  color: #999;
}
</style>
