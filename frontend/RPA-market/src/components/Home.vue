<template>
  <div class="home-container">
    <h2>欢迎来到 RPA Market 权限管理系统</h2>
    <hr />

    <div class="create-box">
      <h3>创建新团体</h3>
      <div class="input-group">
        <input v-model="newGroupName" type="text" placeholder="请输入团体名称" />
        <button @click="handleCreateGroup">立即创建</button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import request from '../utils/request'

const newGroupName = ref('')

const handleCreateGroup = async () => {
  if (!newGroupName.value.trim()) return alert('群组名不能为空')
  try {
    // 对应后端 RegisterGroup 接口
    await request.post('/iam/groups', { name: newGroupName.value })
    alert('团体创建成功！可以前往[Group]组件查看')
    newGroupName.value = ''
  } catch (err: any) {
    alert(err.error || '创建失败，请先登录')
  }
}
</script>

<style scoped>
.home-container {
  background: white;
  padding: 30px;
  border-radius: 8px;
}
.create-box {
  margin-top: 20px;
  padding: 20px;
  background: #f8f9fa;
  border-radius: 6px;
  max-width: 500px;
}
.input-group {
  display: flex;
  gap: 10px;
  margin-top: 10px;
}
.input-group input {
  flex: 1;
  padding: 10px;
  border: 1px solid #ddd;
  border-radius: 4px;
}
.input-group button {
  padding: 10px 20px;
  background: #50cd89;
  color: white;
  border: none;
  border-radius: 4px;
  cursor: pointer;
}
</style>
