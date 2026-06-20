export interface Connection {
  id: string
  name: string
  engine: 'postgres' | 'mongodb'
  host: string
  port: number
  database: string
  username: string
  read_only: boolean
  ssl_mode?: string
  created_at: string
}

export interface ConnectionFormData {
  name: string
  engine: 'postgres' | 'mongodb'
  host: string
  port: number
  database: string
  username: string
  password: string
  read_only: boolean
  ssl_mode: string
  options?: Record<string, string>
}

export interface Column {
  name: string
  data_type: string
  nullable: boolean
}

export interface TableInfo {
  name: string
  schema: string
  type: 'table' | 'view' | 'collection'
  columns: Column[]
}

export interface SchemaInfo {
  name: string
  tables: TableInfo[]
}

export interface ResultSet {
  columns: string[]
  rows: unknown[][]
  total: number
  has_more: boolean
}
