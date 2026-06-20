import type { Connection, ConnectionFormData, ResultSet, SchemaInfo } from '../types'

const BASE = '/api/v1'

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    headers: { 'Content-Type': 'application/json', ...init?.headers },
    ...init,
  })
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }))
    throw new Error(err.error ?? res.statusText)
  }
  if (res.status === 204) return undefined as T
  return res.json()
}

export const api = {
  listConnections(): Promise<Connection[]> {
    return request('/connections')
  },
  getConnection(id: string): Promise<Connection> {
    return request(`/connections/${id}`)
  },
  createConnection(data: ConnectionFormData): Promise<Connection> {
    return request('/connections', { method: 'POST', body: JSON.stringify(data) })
  },
  updateConnection(id: string, data: ConnectionFormData): Promise<Connection> {
    return request(`/connections/${id}`, { method: 'PUT', body: JSON.stringify(data) })
  },
  deleteConnection(id: string): Promise<void> {
    return request(`/connections/${id}`, { method: 'DELETE' })
  },
  testConnection(id: string): Promise<{ status: string }> {
    return request(`/connections/${id}/test`, { method: 'POST' })
  },
  listSchemas(id: string): Promise<SchemaInfo[]> {
    return request(`/connections/${id}/schemas`)
  },
  browse(id: string, schema: string, table: string, offset = 0, limit = 100): Promise<ResultSet> {
    const q = new URLSearchParams({ schema, table, offset: String(offset), limit: String(limit) })
    return request(`/connections/${id}/browse?${q}`)
  },
  query(id: string, query: string, rowLimit = 1000): Promise<ResultSet> {
    return request(`/connections/${id}/query`, {
      method: 'POST',
      body: JSON.stringify({ query, row_limit: rowLimit }),
    })
  },
}
