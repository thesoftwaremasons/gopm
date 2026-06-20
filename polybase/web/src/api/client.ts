import type {
  StoredConnection,
  SchemaInfo,
  ResultSet,
  CreateConnectionRequest,
  JoinDefinition,
} from '../types'

const BASE = '/api/v1'

async function request<T>(path: string, options?: RequestInit): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    headers: { 'Content-Type': 'application/json' },
    ...options,
  })
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }))
    throw new Error(err.error || res.statusText)
  }
  if (res.status === 204) return undefined as T
  return res.json()
}

// Connections
export const listConnections = (): Promise<StoredConnection[]> =>
  request('/connections')

export const getConnection = (id: string): Promise<StoredConnection> =>
  request(`/connections/${id}`)

export const createConnection = (data: CreateConnectionRequest): Promise<StoredConnection> =>
  request('/connections', { method: 'POST', body: JSON.stringify(data) })

export const updateConnection = (
  id: string,
  data: Partial<CreateConnectionRequest>
): Promise<StoredConnection> =>
  request(`/connections/${id}`, { method: 'PUT', body: JSON.stringify(data) })

export const deleteConnection = (id: string): Promise<void> =>
  request(`/connections/${id}`, { method: 'DELETE' })

export const testConnection = (id: string): Promise<{ ok: boolean; error?: string }> =>
  request(`/connections/${id}/test`, { method: 'POST' })

// Schema
export const listSchemas = (id: string): Promise<SchemaInfo[]> =>
  request(`/connections/${id}/schemas`)

// Data
export const browseTable = (
  id: string,
  schema: string,
  table: string,
  offset = 0,
  limit = 100
): Promise<ResultSet> =>
  request(
    `/connections/${id}/browse?schema=${encodeURIComponent(schema)}&table=${encodeURIComponent(
      table
    )}&offset=${offset}&limit=${limit}`
  )

export const runQuery = (
  id: string,
  query: string,
  rowLimit = 1000
): Promise<ResultSet> =>
  request(`/connections/${id}/query`, {
    method: 'POST',
    body: JSON.stringify({ query, row_limit: rowLimit }),
  })

// Joins
export const listJoins = (): Promise<JoinDefinition[]> =>
  request('/joins')

export const getJoin = (id: string): Promise<JoinDefinition> =>
  request(`/joins/${id}`)

export const createJoin = (
  data: Omit<JoinDefinition, 'id' | 'created_at' | 'updated_at'>
): Promise<JoinDefinition> =>
  request('/joins', { method: 'POST', body: JSON.stringify(data) })

export const updateJoin = (
  id: string,
  data: Omit<JoinDefinition, 'id' | 'created_at' | 'updated_at'>
): Promise<JoinDefinition> =>
  request(`/joins/${id}`, { method: 'PUT', body: JSON.stringify(data) })

export const deleteJoin = (id: string): Promise<void> =>
  request(`/joins/${id}`, { method: 'DELETE' })

export const executeJoin = (id: string): Promise<ResultSet> =>
  request(`/joins/${id}/execute`, { method: 'POST' })
