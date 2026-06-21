<template>
  <div class="auth-container">
    <div class="auth-card">
      <h2>{{ isLogin ? '用户登录' : '新用户注册' }}</h2>

      <form @submit.prevent="handleSubmit">
        <div v-if="!isLogin" class="form-item">
          <label>用户名</label>
          <input v-model="form.username" type="text" placeholder="最少3个字符" required />
        </div>

        <div class="form-item">
          <label>邮箱</label>
          <input v-model="form.email" type="email" placeholder="输入邮箱" required />
        </div>

        <div class="form-item">
          <label>密码</label>
          <input v-model="form.password" type="password" placeholder="最少6位密码" required />
        </div>

        <button type="submit" class="btn-submit">{{ isLogin ? '登录' : '注册' }}</button>
      </form>

      <p class="toggle-mode" @click="isLogin = !isLogin">
        {{ isLogin ? '还没有账号？前往注册' : '已有账号？返回登录' }}
      </p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive } from 'vue'
import { useRouter } from 'vue-router'
import request from '../utils/request'

const router = useRouter()
const isLogin = ref(true)

const form = reactive({
  username: '',
  email: '',
  password: '',
})

const handleSubmit = async () => {
  try {
    if (isLogin.value) {
      // 对应后端 Login 接口
      const res: any = await request.post('/iam/login', {
        email: form.email,
        password: form.password,
      })
      localStorage.setItem('auth_token', res.token)
      alert('登录成功！')
      router.push('/home')
    } else {
      // 对应后端 Register 接口
      await request.post('/iam/register', form)
      alert('注册成功，请登录！')
      isLogin.value = true
    }
  } catch (err: any) {
    alert(err.error || '操作失败')
  }
}
</script>

<style scoped>
.auth-container {
  display: flex;
  justify-content: center;
  align-items: center;
  height: 70vh;
}
.auth-card {
  background: #fff;
  padding: 30px;
  border-radius: 8px;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.1);
  width: 360px;
}
.form-item {
  margin-bottom: 15px;
  display: flex;
  flex-direction: column;
}
.form-item label {
  margin-bottom: 5px;
  font-weight: bold;
}
.form-item input {
  padding: 10px;
  border: 1px solid #ddd;
  border-radius: 4px;
}
.btn-submit {
  width: 100%;
  padding: 10px;
  background: #009ef7;
  color: white;
  border: none;
  border-radius: 4px;
  cursor: pointer;
}
.toggle-mode {
  text-align: center;
  color: #009ef7;
  margin-top: 15px;
  cursor: pointer;
  font-size: 0.9rem;
}
</style>
