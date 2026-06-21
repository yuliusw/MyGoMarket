<template>
  <div class="app-layout">
    <!-- 左侧边栏 -->
    <aside class="sidebar">
      <div class="logo">RPA Market</div>
      <nav class="menu">
        <router-link
          v-for="item in menuList"
          :key="item.path"
          :to="item.path"
          class="menu-item"
          active-class="active"
        >
          {{ item.name }}
        </router-link>
      </nav>
    </aside>

    <!-- 右侧主体内容 -->
    <main class="main-content">
      <router-view v-slot="{ Component }">
        <transition name="fade" mode="out-in">
          <component :is="Component" />
        </transition>
      </router-view>
    </main>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { menuItems } from '../router'

// 响应式菜单数据
const menuList = ref(menuItems)
</script>

<style scoped>
.app-layout {
  display: flex;
  width: 100vw;
  height: 100vh;
  overflow: hidden;
}

.sidebar {
  width: 240px;
  background-color: #1e1e2d;
  color: #fff;
  display: flex;
  flex-direction: column;
  box-shadow: 2px 0 8px rgba(0, 0, 0, 0.15);
}

.logo {
  padding: 20px;
  font-size: 1.2rem;
  font-weight: bold;
  text-align: center;
  border-bottom: 1px solid #2b2b40;
}

.menu {
  display: flex;
  flex-direction: column;
  padding: 10px 0;
}

.menu-item {
  padding: 15px 20px;
  color: #a2a3b7;
  text-decoration: none;
  transition: all 0.3s;
}

.menu-item:hover {
  background-color: #2b2b40;
  color: #fff;
}

.menu-item.active {
  background-color: #009ef7;
  color: #fff;
  font-weight: bold;
}

.main-content {
  flex: 1;
  padding: 20px;
  background-color: #f5f8fa;
  overflow-y: auto;
}

/* 切换动效 */
.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.2s ease;
}
.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}
</style>
