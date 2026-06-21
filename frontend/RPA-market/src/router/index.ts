import { createRouter, createWebHistory, type RouteRecordRaw } from 'vue-router'
import MainLayout from '../layout/MainLayout.vue'

// 1. 动态扫描 src/components 下的所有 .vue 文件
const modules = import.meta.glob('../components/*.vue')

// 2. 转换为路由配置与侧边栏菜单
const dynamicRoutes: RouteRecordRaw[] = []
export const menuItems: { name: string; path: string }[] = []

Object.keys(modules).forEach((key) => {
  // 从文件路径中提取组件名。例如: '../components/Login.vue' -> 'Login'
  const componentName = key.split('/').pop()?.replace('.vue', '')

  if (componentName) {
    const pathName = componentName.toLowerCase()
    const path = pathName === 'home' ? '' : pathName // Home 组件作为默认空路径

    // 将生成的路由推入动态路由子集
// 将生成的路由推入动态路由子集
dynamicRoutes.push({
  path: path,
  name: componentName,
  // 加上 as any 或者显式指定组件返回类型
  component: modules[key] as () => Promise<any>, 
})

    // 收集侧边栏菜单数据
    menuItems.push({
      name: componentName,
      path: `/home/${path}`,
    })
  }
})

// 3. 基础路由配置
const routes: RouteRecordRaw[] = [
  {
    path: '/',
    redirect: '/home',
  },
  {
    path: '/home',
    component: MainLayout,
    children: [...dynamicRoutes],
  },
]

const router = createRouter({
  history: createWebHistory(),
  routes,
})

export default router
