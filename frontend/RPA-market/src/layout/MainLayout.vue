<template>
  <div class="shell">
    <aside class="sidebar">
      <div class="brand">
        <span class="brand-mark">R</span>
        <div>
          <strong>RPA Market</strong>
          <small>应用购买工作台</small>
        </div>
      </div>
      <nav class="menu">
        <router-link
          v-for="item in menuList"
          :key="item.path"
          :to="item.path"
          class="menu-item"
          active-class="is-active"
        >
          {{ item.name }}
        </router-link>
      </nav>
      <router-link class="login-link" to="/login">登录 / 注册</router-link>
    </aside>

    <main class="content">
      <router-view v-slot="{ Component }">
        <transition name="page" mode="out-in">
          <component :is="Component" />
        </transition>
      </router-view>
    </main>
  </div>
</template>

<script setup lang="ts">
import { menuItems } from '../router'

const menuList = menuItems
</script>

<style scoped>
.shell {
  display: flex;
  min-height: 100vh;
  background:
    radial-gradient(circle at top left, rgba(59, 130, 246, 0.16), transparent 36rem),
    linear-gradient(135deg, #f8fafc 0%, #eef2ff 100%);
}

.sidebar {
  position: sticky;
  top: 0;
  width: 280px;
  height: 100vh;
  padding: 24px;
  background: #0f172a;
  color: #e2e8f0;
  display: flex;
  flex-direction: column;
  gap: 28px;
}

.brand {
  display: flex;
  align-items: center;
  gap: 14px;
}

.brand-mark {
  display: grid;
  width: 44px;
  height: 44px;
  place-items: center;
  border-radius: 16px;
  background: linear-gradient(135deg, #38bdf8, #6366f1);
  color: #fff;
  font-size: 24px;
  font-weight: 800;
}

.brand strong {
  display: block;
  font-size: 18px;
  letter-spacing: -0.02em;
}

.brand small {
  color: #94a3b8;
}

.menu {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.menu-item {
  padding: 12px 14px;
  border-radius: 14px;
  color: #cbd5e1;
  text-decoration: none;
  transition: 0.2s ease;
}

.menu-item:hover {
  background: rgba(148, 163, 184, 0.12);
  color: #ffffff;
}

.menu-item.is-active {
  background: #ffffff;
  color: #0f172a;
  font-weight: 700;
}

.login-link {
  margin-top: auto;
  padding: 12px 14px;
  border: 1px solid rgba(226, 232, 240, 0.18);
  border-radius: 14px;
  color: #e2e8f0;
  text-decoration: none;
}

.login-link:hover {
  border-color: rgba(226, 232, 240, 0.45);
  color: #fff;
}

.content {
  flex: 1;
  min-width: 0;
  padding: 32px;
}

.page-enter-active,
.page-leave-active {
  transition: opacity 0.2s ease;
}
.page-enter-from,
.page-leave-to {
  opacity: 0;
}

@media (max-width: 760px) {
  .shell {
    flex-direction: column;
  }

  .sidebar {
    position: relative;
    width: 100%;
    height: auto;
  }

  .menu {
    flex-direction: row;
    overflow-x: auto;
  }

  .login-link {
    margin-top: 0;
  }

  .content {
    padding: 18px;
  }
}
</style>
