import { createRouter, createWebHistory, type RouteRecordRaw } from 'vue-router'
import MainLayout from '../layout/MainLayout.vue'

export const menuItems = [
  { name: '应用市场', path: '/market' },
  { name: '钱包', path: '/wallet' },
  { name: '订单', path: '/orders' },
]

const routes: RouteRecordRaw[] = [
  {
    path: '/',
    redirect: '/market',
  },
  {
    path: '/login',
    name: 'Login',
    component: () => import('../components/Login.vue'),
  },
  {
    path: '/',
    component: MainLayout,
    children: [
      { path: 'market', name: 'Market', component: () => import('../components/AppMarket.vue') },
      { path: 'wallet', name: 'Wallet', component: () => import('../components/Wallet.vue') },
      { path: 'orders', name: 'Orders', component: () => import('../components/Orders.vue') },
    ],
  },
]

const router = createRouter({
  history: createWebHistory(),
  routes,
})

export default router
