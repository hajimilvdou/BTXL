/* ============================================================
 * 用户数据状态管理 — Zustand Store
 *
 * 职责:
 * 1. 聚合用户侧常用数据: 资料、额度、统计、凭证、兑换模板、推荐统计
 * 2. 统一做前端字段适配, 将后端 snake_case 转为页面更易读的 camelCase
 * 3. 为页面提供稳定的 fetch / loading 契约, 降低页面层耦合
 * ============================================================ */

import { create } from 'zustand'
import type { User } from '../api/auth'
import {
  getProfile as apiGetProfile,
  getQuota as apiGetQuota,
  getStats as apiGetStats,
  getCredentials as apiGetCredentials,
  getRedemptionTemplates as apiGetRedemptionTemplates,
  getReferralInfo as apiGetReferralInfo,
  type QuotaInfo,
  type UserStats,
  type CredentialItem,
  type ModelQuota,
  type RedemptionTemplate as ApiRedemptionTemplate,
  type ReferralInfo as ApiReferralInfo,
} from '../api/user'

/* ----------------------------------------------------------
 * 对外导出类型
 * ---------------------------------------------------------- */

export interface QuotaItem {
  modelPattern: string
  quotaType: 'count' | 'token' | 'both'
  usedRequests: number
  usedTokens: number
  maxRequests: number
  maxTokens: number
  bonusRequests: number
  bonusTokens: number
  period: 'daily' | 'weekly' | 'monthly' | 'total'
  periodEnd: string
}

export interface UserCredential {
	id: string
	provider: string
	health: 'healthy' | 'unhealthy' | 'unknown'
	createdAt: string
	lastChecked: string | null
}

export type Credential = UserCredential

export interface RedemptionTemplate {
  id: string
  name: string
  description: string
  bonusRequests: number
  bonusTokens: number
  modelPattern: string
  available: boolean
}

export interface ReferralInfo {
  totalInvitees: number
  totalBonusReqs: number
  totalBonusTokens: number
}

/* ----------------------------------------------------------
 * 内部映射函数
 * ---------------------------------------------------------- */

function mapQuotaItem(model: ModelQuota, quota: QuotaInfo): QuotaItem {
  return {
    modelPattern: model.model,
    quotaType: 'count',
    usedRequests: model.used,
    usedTokens: 0,
    maxRequests: model.total,
    maxTokens: 0,
    bonusRequests: 0,
    bonusTokens: 0,
    period: quota.reset_at ? 'monthly' : 'total',
    periodEnd: quota.reset_at ?? '',
  }
}

function mapTemplate(template: ApiRedemptionTemplate): RedemptionTemplate {
  return {
    id: String(template.id),
    name: template.name,
    description: template.description,
    bonusRequests: template.bonus_requests,
    bonusTokens: template.bonus_tokens,
    modelPattern: template.model_pattern,
    available: template.available,
  }
}

function mapReferralInfo(info: ApiReferralInfo): ReferralInfo {
  return {
    totalInvitees: info.total_invitees,
    totalBonusReqs: info.total_bonus_reqs,
    totalBonusTokens: info.total_bonus_tokens,
  }
}

function mapCredential(item: CredentialItem): UserCredential {
	return {
		id: item.id,
		provider: item.provider,
		health: item.health,
		createdAt: item.created_at,
		lastChecked: item.last_checked,
	}
}

function mapStats(stats: UserStats): UserStats {
	return {
		total_requests: stats.total_requests ?? 0,
		today_requests: stats.today_requests ?? 0,
		week_requests: stats.week_requests ?? 0,
		month_requests: stats.month_requests ?? 0,
		recent_usage: stats.recent_usage ?? [],
	}
}

/* ----------------------------------------------------------
 * Store 类型
 * ---------------------------------------------------------- */

interface UserState {
  profile: User | null
  quota: QuotaInfo | null
  quotas: QuotaItem[]
  stats: UserStats | null
	credentials: UserCredential[]
  templates: RedemptionTemplate[]
  referral: ReferralInfo | null

  profileLoading: boolean
  quotaLoading: boolean
  statsLoading: boolean
  credentialsLoading: boolean
  templatesLoading: boolean
  referralLoading: boolean

  error: string | null

  fetchProfile: () => Promise<void>
  fetchQuota: () => Promise<void>
  fetchQuotas: () => Promise<void>
  fetchStats: () => Promise<void>
  fetchCredentials: () => Promise<void>
  fetchTemplates: () => Promise<void>
  fetchReferral: () => Promise<void>
  reset: () => void
}

const initialState = {
  profile: null,
  quota: null,
  quotas: [] as QuotaItem[],
  stats: null,
	credentials: [] as UserCredential[],
  templates: [] as RedemptionTemplate[],
  referral: null as ReferralInfo | null,
  profileLoading: false,
  quotaLoading: false,
  statsLoading: false,
  credentialsLoading: false,
  templatesLoading: false,
  referralLoading: false,
  error: null,
}

/* ----------------------------------------------------------
 * Store 实现
 * ---------------------------------------------------------- */

export const useUserStore = create<UserState>()((set) => ({
  ...initialState,

  fetchProfile: async () => {
    set({ profileLoading: true, error: null })
    try {
      const res = await apiGetProfile()
      set({ profile: res.data, profileLoading: false })
    } catch (err) {
      const message = err instanceof Error ? err.message : '获取个人信息失败'
      set({ profileLoading: false, error: message })
    }
  },

  fetchQuota: async () => {
    set({ quotaLoading: true, error: null })
    try {
      const res = await apiGetQuota()
      set({
        quota: res.data,
        quotas: (res.data.models ?? []).map((model) => mapQuotaItem(model, res.data)),
        quotaLoading: false,
      })
    } catch (err) {
      const message = err instanceof Error ? err.message : '获取额度信息失败'
      set({ quotaLoading: false, error: message })
    }
  },

  fetchQuotas: async () => {
    await useUserStore.getState().fetchQuota()
  },

	  fetchStats: async () => {
	    set({ statsLoading: true, error: null })
	    try {
	      const res = await apiGetStats()
	      set({ stats: mapStats(res.data), statsLoading: false })
	    } catch (err) {
      const message = err instanceof Error ? err.message : '获取使用统计失败'
      set({ statsLoading: false, error: message })
    }
  },

	  fetchCredentials: async () => {
	    set({ credentialsLoading: true, error: null })
	    try {
	      const res = await apiGetCredentials()
	      set({
	        credentials: (res.data.credentials ?? []).map(mapCredential),
	        credentialsLoading: false,
	      })
	    } catch (err) {
      const message = err instanceof Error ? err.message : '获取凭证列表失败'
      set({ credentialsLoading: false, error: message })
    }
  },

  fetchTemplates: async () => {
    set({ templatesLoading: true, error: null })
    try {
      const res = await apiGetRedemptionTemplates()
      set({
        templates: (res.data.templates ?? []).map(mapTemplate),
        templatesLoading: false,
      })
    } catch (err) {
      const message = err instanceof Error ? err.message : '获取兑换模板失败'
      set({ templatesLoading: false, error: message })
    }
  },

  fetchReferral: async () => {
    set({ referralLoading: true, error: null })
    try {
      const res = await apiGetReferralInfo()
      set({ referral: mapReferralInfo(res.data.referral), referralLoading: false })
    } catch {
      set({ referral: null, referralLoading: false })
    }
  },

  reset: () => set(initialState),
}))
