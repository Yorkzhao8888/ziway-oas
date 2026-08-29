/**
 * API Client — 知味生态统一 HTTP 客户端
 *
 * 设计原则：
 *   - 每个 MBS 代理基址创建一个独立 axios 实例
 *   - 自动注入 Authorization + X-User-ID
 *   - 统一错误处理（401 跳登录、403 提示、网络异常兜底）
 *   - P1 换 gRPC 后只需替换本文件内部实现，业务代码不动
 *
 * ZW-ARC-017 更新：
 *   - CBOS 新增 dmbs 代理（Mall 消费 cmbs+dmbs）
 *   - DBOS 新增 hmbs 代理（Shop 消费 dmbs+hmbs+fmbs）
 *   - Xcase 新增 obos 治理 Tab
 *
 * 使用方式：
 *   import { cmbsApi, ambsApi } from '@/utils/api-client'
 *   cmbsApi.get('/customers', { params: { page: 1 } })
 *   ambsApi.post('/auth/login', { username, password })
 */

import axios, { type AxiosInstance, type AxiosError, type InternalAxiosRequestConfig } from 'axios'

// ============================================================
// 1. 从环境变量构建完整 URL
// ============================================================

const BOS_BASE = import.meta.env.VITE_BOS_BASE_URL // http://localhost:8082

function fullUrl(proxyBase: string): string {
  return `${BOS_BASE}${proxyBase}`
}

// ============================================================
// 2. Token 管理（轻量实现，可替换为 zustand/pinia store）
// ============================================================

const TOKEN_KEY = 'zt_token'
const USER_ID_KEY = 'zt_user_id'

export function getToken(): string | null {
  return localStorage.getItem(TOKEN_KEY)
}

export function setToken(token: string): void {
  localStorage.setItem(TOKEN_KEY, token)
}

export function getUserId(): string | null {
  return localStorage.getItem(USER_ID_KEY)
}

export function setUserId(userId: string): void {
  localStorage.setItem(USER_ID_KEY, userId)
}

export function clearAuth(): void {
  localStorage.removeItem(TOKEN_KEY)
  localStorage.removeItem(USER_ID_KEY)
}

// ============================================================
// 3. 创建 axios 实例（带拦截器）
// ============================================================

function createApiClient(proxyBase: string): AxiosInstance {
  const instance = axios.create({
    baseURL: fullUrl(proxyBase),
    timeout: 15000,
    headers: {
      'Content-Type': 'application/json',
    },
  })

  // ---- 请求拦截：注入鉴权头 ----
  instance.interceptors.request.use(
    (config: InternalAxiosRequestConfig) => {
      const token = getToken()
      if (token) {
        config.headers.Authorization = `Bearer ${token}`
      }
      const userId = getUserId()
      if (userId) {
        config.headers['X-User-ID'] = userId
      }
      return config
    },
    (error) => Promise.reject(error),
  )

  // ---- 响应拦截：统一错误处理 ----
  instance.interceptors.response.use(
    (response) => response,
    (error: AxiosError<{ code?: number; message?: string }>) => {
      if (error.response) {
        const { status, data } = error.response
        switch (status) {
          case 401:
            clearAuth()
            window.location.href = '/login?redirect=' + encodeURIComponent(window.location.pathname)
            break
          case 403:
            console.error('[API] 无权限:', data?.message || 'Forbidden')
            break
          case 404:
            console.warn('[API] 资源不存在:', error.config?.url)
            break
          case 409:
            console.warn('[API] 冲突:', data?.message)
            break
          case 422:
            console.warn('[API] 参数错误:', data?.message)
            break
          case 500:
          case 502:
          case 503:
            console.error('[API] 服务端异常:', status, data?.message)
            break
          default:
            console.error('[API] 未知错误:', status, data?.message)
        }
      } else if (error.code === 'ECONNABORTED') {
        console.error('[API] 请求超时:', error.config?.url)
      } else {
        console.error('[API] 网络异常:', error.message)
      }
      return Promise.reject(error)
    },
  )

  return instance
}

// ============================================================
// 4. 导出各 MBS 的 API 实例
// ============================================================

// ---- AMBS（鉴权/用户）所有 APP 都需要 ----
export const ambsApi = createApiClient(import.meta.env.VITE_AMBS_BASE)

// ---- 单 MBS APP ----
export const pmbsApi = createApiClient(import.meta.env.VITE_PMBS_BASE)   // Lab
export const hmbsApi = createApiClient(import.meta.env.VITE_HMBS_BASE)   // Mate
export const embsApi = createApiClient(import.meta.env.VITE_EMBS_BASE)   // Market

// ---- Mall: CBOS → cmbs + dmbs [ZW-ARC-017] ----
export const cmbsApi = createApiClient(import.meta.env.VITE_CMBS_BASE)
export const dmbsMallApi = createApiClient(import.meta.env.VITE_DMBS_BASE)

// ---- Shop: DBOS → dmbs + hmbs + fmbs [ZW-ARC-017] ----
export const dmbsShopApi = createApiClient(import.meta.env.VITE_DMBS_BASE)
export const hmbsShopApi = createApiClient(import.meta.env.VITE_HMBS_BASE)
export const fmbsShopApi = createApiClient(import.meta.env.VITE_FMBS_BASE)

// ---- Xcase: ibos → imbs + fmbs ----
export const imbsApi = createApiClient(import.meta.env.VITE_IMBS_BASE)
export const fmbsIbosApi = createApiClient(import.meta.env.VITE_FMBS_IBOS_BASE)

// ---- Xcase: vbos → gmbs + ombs + vmbs ----
export const vmbsApi = createApiClient(import.meta.env.VITE_VMBS_BASE)
export const ombsVbosApi = createApiClient(import.meta.env.VITE_OMBS_VBOS_BASE)
export const gmbsVbosApi = createApiClient(import.meta.env.VITE_GMBS_VBOS_BASE)

// ---- Xcase: obos → ombs [ZW-ARC-017 新增] ----
export const ombsObosApi = createApiClient(import.meta.env.VITE_OMBS_OBOS_BASE)

// ============================================================
// 5. 登录辅助函数
// ============================================================

/**
 * 登录 → 获取 token → 存入 localStorage
 * 所有 APP 统一走 abos/ambs 的登录接口
 */
export async function login(username: string, password: string) {
  const res = await ambsApi.post('/auth/login', { username, password })
  const { token, user_id } = res.data.data
  setToken(token)
  setUserId(user_id)
  return res.data.data
}

/**
 * 登出
 */
export function logout() {
  clearAuth()
  window.location.href = '/login'
}

/**
 * 检查是否已登录
 */
export function isLoggedIn(): boolean {
  return !!getToken()
}
