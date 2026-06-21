import request from './utils/request'

export interface MarketApp {
  app_id: string
  name: string
  developer_id: string
  category: string
  tags: string[]
  metadata: Record<string, unknown>
  status: string
  create_at: string
  update_at: string
}

export interface Wallet {
  wallet_id: string
  owner_id: string
  owner_type: string
  balance: string
  currency_code: string
  status: string
  updated_at: string
}

export interface WalletTransaction {
  tx_id: string
  wallet_id: string
  tx_type: string
  amount: string
  balance_after: string
  reference_id: string
  idempotency_key: string
  description: string
  created_at: string
}

export interface Order {
  order_id: string
  user_id: string
  app_id: string
  wallet_id: string
  amount: string
  currency_code: string
  status: string
  tx_id: string
  subscription_id: string
  idempotency_key: string
  description: string
  created_at: string
  paid_at: string
  updated_at: string
}

export interface ChangeLog {
  event_id: string
  event_type: string
  trace_id: string
  actor_id: string
  resource: string
  metadata: string
  error: string
  created_at: string
}

export interface PageResult<T> {
  items: T[]
  total: number
  page: number
  page_size: number
}

export async function listApps(params: Record<string, unknown>) {
  return request.get('/market/apps', { params }) as Promise<{
    data: MarketApp[]
    total: number
    page: number
    page_size: number
  }>
}

export async function publishApp(formData: FormData) {
  return request.post('/market/apps', formData, {
    headers: { 'Content-Type': 'multipart/form-data' },
  }) as Promise<{ app_id: string; message: string; idempotent?: boolean }>
}

export async function updateApp(appID: string, payload: Record<string, unknown>) {
  return request.put(`/market/apps/${appID}`, payload) as Promise<{ data: MarketApp; message: string }>
}

export async function offShelfApp(appID: string) {
  return request.put(`/market/apps/${appID}/offshelf`) as Promise<{ message: string }>
}

export async function deleteApp(appID: string) {
  return request.delete(`/market/apps/${appID}`) as Promise<{ message: string }>
}

export async function getMyWallet(currencyCode = 'COIN') {
  return request.get('/wallets/me', { params: { currency_code: currencyCode } }) as Promise<Wallet>
}

export async function creditWallet(walletID: string, amount: string) {
  return request.post(`/wallets/${walletID}/credit`, {
    amount,
    description: 'frontend topup',
    idempotency_key: uniqueKey('topup'),
  }) as Promise<{ wallet: Wallet; transaction: WalletTransaction }>
}

export async function listWalletTransactions(walletID: string, page = 1, pageSize = 20) {
  return request.get(`/wallets/${walletID}/transactions`, {
    params: { page, page_size: pageSize },
  }) as Promise<{ items: WalletTransaction[]; page: number; page_size: number }>
}

export async function purchaseApp(appID: string, amount: string) {
  return request.post('/orders/purchase', {
    app_id: appID,
    amount,
    currency_code: 'COIN',
    description: 'frontend purchase',
    idempotency_key: uniqueKey(`purchase-${appID}`),
  }) as Promise<Order>
}

export async function listOrders(page = 1, pageSize = 20) {
  return request.get('/orders', { params: { page, page_size: pageSize } }) as Promise<{
    items: Order[]
    page: number
    page_size: number
  }>
}

export async function listAdminVirtualOrders(params: Record<string, unknown>) {
  return request.get('/admin/virtual-orders', { params }) as Promise<PageResult<Wallet>>
}

export async function listAdminWalletTransactions(params: Record<string, unknown>) {
  return request.get('/admin/wallet-transactions', { params }) as Promise<PageResult<WalletTransaction>>
}

export async function listAdminOrders(params: Record<string, unknown>) {
  return request.get('/admin/orders', { params }) as Promise<PageResult<Order>>
}

export async function listAdminChangeLogs(params: Record<string, unknown>) {
  return request.get('/admin/change-logs', { params }) as Promise<PageResult<ChangeLog>>
}

function uniqueKey(prefix: string) {
  if (globalThis.crypto?.randomUUID) {
    return `${prefix}-${globalThis.crypto.randomUUID()}`
  }
  return `${prefix}-${Date.now()}-${Math.random().toString(16).slice(2)}`
}
