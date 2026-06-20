export interface ConnectionConfig {
  id: string
  name: string
  engine: string
  host: string
  port: number
  database: string
  username: string
  password?: string
  read_only: boolean
  ssl_mode?: string
  options?: Record<string, string>
}

export interface StoredConnection {
  id: string
  name: string
  engine: string
  config: ConnectionConfig
  created_at: string
  updated_at: string
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
  columns?: Column[]
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

export interface CreateConnectionRequest {
  name: string
  engine: string
  config: Partial<ConnectionConfig>
}

export type ChartType = 'bar' | 'line' | 'area' | 'pie'

export interface ChartDefinition {
  id: string
  name: string
  connection_id: string
  sql: string
  x_column: string
  y_column: string
  chart_type: ChartType
  created_at?: string
  updated_at?: string
}

export interface JoinDefinition {
  id: string
  name: string
  source_a: string
  source_b: string
  key_field_a: string
  key_field_b: string
  table_a: string
  table_b: string
  schema_a: string
  schema_b: string
  created_at?: string
  updated_at?: string
}
