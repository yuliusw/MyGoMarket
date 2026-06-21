<template>
  <div class="group-manager">
    <!-- 左侧：我的团体列表 -->
    <div class="group-list">
      <h3>我的团体列表</h3>
      <ul>
        <li
          v-for="g in groups"
          :key="g.GroupID"
          :class="{ active: currentGroup?.Group?.GroupID === g.GroupID }"
          @click="fetchGroupDetail(g.GroupID)"
        >
          {{ g.GroupName }}
        </li>
      </ul>
    </div>

    <!-- 右侧：团体详情 -->
    <div class="group-detail" v-if="currentGroup && currentGroup.Group">
      <div class="detail-header">
        <h2>
          <input v-model="currentGroup.Group.GroupName" class="edit-title" />
          <button @click="handleUpdateGroup" class="btn-xs btn-blue">修改名称</button>
        </h2>
        <div class="actions">
          <button @click="handleLeaveGroup" class="btn-xs btn-warn">退出该群</button>
          <button @click="handleDissolveGroup" class="btn-xs btn-danger">
            解散该群(仅限Owner)
          </button>
        </div>
      </div>

      <!-- 成员管理 -->
      <div class="members-section">
        <h3>成员管理</h3>

        <!-- 邀请表单 -->
        <div class="invite-form">
          <input v-model="inviteUserId" placeholder="被邀请人用户 ID" />
          <select v-model.number="inviteRoleId">
            <option :value="1">管理员</option>
            <option :value="2">普通成员</option>
          </select>
          <button @click="handleInvite">邀请进群</button>
        </div>

        <!-- 成员表格 -->
        <table class="member-table">
          <thead>
            <tr>
              <th>用户 ID</th>
              <th>角色</th>
              <th>角色描述</th>
              <th>操作</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="m in currentGroup.Members" :key="m.UserID">
              <td>{{ m.UserID }}</td>
              <td>{{ m.Role?.Name || '普通成员' }}</td>
              <td>{{ m.Role?.Description || '-' }}</td>
              <td>
                <button @click="handleKick(m.UserID)" class="btn-text-danger">移除</button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
    <div class="empty-notice" v-else>
      <p>⬅️ 请在左侧选择一个团体查看详情</p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import request from '../utils/request'

// 对应后端单条 Group 的数据结构
interface GroupItem {
  GroupID: string
  GroupName: string
  OwnerID: string
  Type: string
  CreatedAt: string
}

// 对应后端 Member 的数据结构
interface MemberItem {
  GroupID: string
  UserID: string
  RoleID: number
  Role: {
    ID: number
    Name: string
    Description: string
    Permissions: any
  } | null
}

// 对应详情接口返回的完整结构
interface GroupDetail {
  Group: GroupItem
  Members: MemberItem[]
}

const groups = ref<GroupItem[]>([])
const currentGroup = ref<GroupDetail | null>(null)

// 邀请表单状态
const inviteUserId = ref('')
const inviteRoleId = ref(2)

// 1. 初始化获取“我的团体列表” (对应 GET /groups)
const loadGroups = async () => {
  try {
    const res: any = await request.get('/iam/groups')
    groups.value = res || []
  } catch (err: any) {
    alert(err.response?.data?.error || err.error || '获取群组列表失败')
  }
}

// 2. 获取指定的群详情 (对应 GET /groups/:id)
const fetchGroupDetail = async (id: string) => {
  try {
    const res: any = await request.get(`/iam/groups/${id}`)
    currentGroup.value = res
  } catch (err: any) {
    alert(`无法查看群详情: ${err.response?.data?.error || err.error || '权限不足'}`)
  }
}

// 3. 更新群组信息 (对应 PUT /groups/:id)
const handleUpdateGroup = async () => {
  if (!currentGroup.value?.Group) return
  const groupId = currentGroup.value.Group.GroupID
  try {
    await request.put(`/iam/groups/${groupId}`, {
      GroupName: currentGroup.value.Group.GroupName,
    })
    alert('修改群名成功！')
    loadGroups()
  } catch (err: any) {
    alert(err.response?.data?.error || err.error || '修改失败')
  }
}

// 4. 退出团体 (对应 POST /groups/:id/leave)
const handleLeaveGroup = async () => {
  if (!currentGroup.value?.Group || !confirm('确认退出该团体？')) return
  const groupId = currentGroup.value.Group.GroupID
  try {
    await request.post(`/iam/groups/${groupId}/leave`)
    currentGroup.value = null
    loadGroups()
  } catch (err: any) {
    alert(err.response?.data?.error || err.error || '退出失败')
  }
}

// 5. 解散团体 (对应 DELETE /groups/:id)
const handleDissolveGroup = async () => {
  if (!currentGroup.value?.Group || !confirm('警告：此操作不可逆！确认解散？')) return
  const groupId = currentGroup.value.Group.GroupID
  try {
    await request.delete(`/iam/groups/${groupId}`)
    currentGroup.value = null
    loadGroups()
  } catch (err: any) {
    alert(err.response?.data?.error || err.error || '解散失败，可能你不是 Owner')
  }
}

// 6. 邀请成员进群 (对应 POST /groups/:id/members)
const handleInvite = async () => {
  if (!currentGroup.value?.Group || !inviteUserId.value) return
  const groupId = currentGroup.value.Group.GroupID
  try {
    await request.post(`/iam/groups/${groupId}/members`, {
      user_id: inviteUserId.value, // 如果后端接收大写，请根据实际情况改为 UserID 和 RoleID
      role_id: inviteRoleId.value,
    })
    alert('成功邀请成员！')
    fetchGroupDetail(groupId) // 刷新当前详情
    inviteUserId.value = ''
  } catch (err: any) {
    alert(err.response?.data?.error || err.error || '邀请失败，权限不足')
  }
}

// 7. 踢除群成员 (对应 DELETE /groups/:id/members/:user_id)
const handleKick = async (targetUserId: string) => {
  if (!currentGroup.value?.Group || !confirm('确认将该成员移出团体？')) return
  const groupId = currentGroup.value.Group.GroupID
  try {
    await request.delete(`/iam/groups/${groupId}/members/${targetUserId}`)
    alert('移除成功')
    fetchGroupDetail(groupId)
  } catch (err: any) {
    alert(err.response?.data?.error || err.error || '移除失败，权限不足')
  }
}

onMounted(() => {
  loadGroups()
})
</script>

<style scoped>
.group-manager {
  display: flex;
  background: white;
  min-height: 70vh;
  border-radius: 8px;
  overflow: hidden;
}
.group-list {
  width: 250px;
  border-right: 1px solid #eee;
  padding: 20px;
}
.group-list ul {
  list-style: none;
  padding: 0;
}
.group-list li {
  padding: 12px;
  margin-bottom: 8px;
  border-radius: 4px;
  cursor: pointer;
  background: #f8f9fa;
}
.group-list li.active,
.group-list li:hover {
  background: #009ef7;
  color: white;
}

.group-detail {
  flex: 1;
  padding: 20px;
}
.detail-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  border-bottom: 2px solid #f1f1f1;
  padding-bottom: 15px;
}
.edit-title {
  font-size: 1.5rem;
  font-weight: bold;
  border: none;
  border-bottom: 1px dashed #ccc;
  width: 200px;
}
.actions {
  display: flex;
  gap: 10px;
}

.members-section {
  margin-top: 20px;
}
.invite-form {
  display: flex;
  gap: 10px;
  margin-bottom: 20px;
  background: #f8f9fa;
  padding: 15px;
  border-radius: 4px;
}
.invite-form input,
.invite-form select {
  padding: 8px;
  border: 1px solid #ddd;
}
.invite-form button {
  background: #009ef7;
  color: white;
  border: none;
  padding: 8px 15px;
  border-radius: 4px;
  cursor: pointer;
}

.member-table {
  width: 100%;
  border-collapse: collapse;
  text-align: left;
}
.member-table th,
.member-table td {
  padding: 12px;
  border-bottom: 1px solid #eee;
}

.btn-xs {
  padding: 6px 12px;
  border: none;
  border-radius: 4px;
  cursor: pointer;
  color: white;
  font-size: 0.85rem;
}
.btn-blue {
  background: #009ef7;
}
.btn-warn {
  background: #ffc107;
  color: #333;
}
.btn-danger {
  background: #f1416c;
}
.btn-text-danger {
  color: #f1416c;
  border: none;
  background: none;
  cursor: pointer;
}
.empty-notice {
  flex: 1;
  display: flex;
  justify-content: center;
  align-items: center;
  color: #999;
}
</style>
