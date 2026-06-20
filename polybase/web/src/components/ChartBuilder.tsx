import { useState, useEffect } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { listConnections, listSchemas, getChart, createChart, updateChart, executeChart, runQuery } from '../api/client'
import type { ResultSet, ChartType } from '../types'
import { ChartView } from './ChartView'
import { ResultGrid } from './ResultGrid'

const CHART_TYPES: { type: ChartType; label: string; icon: string }[] = [
  { type: 'bar',  label: 'Bar',  icon: '▊' },
  { type: 'line', label: 'Line', icon: '📈' },
  { type: 'area', label: 'Area', icon: '◼' },
  { type: 'pie',  label: 'Pie',  icon: '◕' },
]

const AGG_FNS = ['COUNT', 'SUM', 'AVG', 'MAX', 'MIN']
const SAFE_IDENT = /^[a-zA-Z_][a-zA-Z0-9_]*$/

export function ChartBuilder() {
  const { chartId } = useParams<{ chartId: string }>()
  const navigate = useNavigate()
  const qc = useQueryClient()
  const isEditing = !!chartId

  // Form state
  const [name, setName] = useState('')
  const [connectionId, setConnectionId] = useState('')
  const [sql, setSql] = useState('')
  const [xColumn, setXColumn] = useState('')
  const [yColumn, setYColumn] = useState('')
  const [chartType, setChartType] = useState<ChartType>('bar')

  // Builder (no-code) mode state
  const [mode, setMode] = useState<'sql' | 'builder'>('sql')
  const [bSchema, setBSchema] = useState('')
  const [bTable, setBTable] = useState('')
  const [bGroupBy, setBGroupBy] = useState('')
  const [bMetric, setBMetric] = useState('')
  const [bAgg, setBAgg] = useState('COUNT')
  const [bLimit, setBLimit] = useState(50)

  // Result state
  const [result, setResult] = useState<ResultSet | null>(null)
  const [resultView, setResultView] = useState<'chart' | 'table'>('chart')
  const [runError, setRunError] = useState<string | null>(null)
  const [saveError, setSaveError] = useState<string | null>(null)

  const { data: connections = [] } = useQuery({
    queryKey: ['connections'],
    queryFn: listConnections,
  })

  const { data: schemas = [] } = useQuery({
    queryKey: ['schemas', connectionId],
    queryFn: () => listSchemas(connectionId),
    enabled: !!connectionId && mode === 'builder',
  })

  const { data: existingChart } = useQuery({
    queryKey: ['chart', chartId],
    queryFn: () => getChart(chartId!),
    enabled: isEditing,
  })

  useEffect(() => {
    if (existingChart) {
      setName(existingChart.name)
      setConnectionId(existingChart.connection_id)
      setSql(existingChart.sql)
      setXColumn(existingChart.x_column)
      setYColumn(existingChart.y_column)
      setChartType(existingChart.chart_type)
    }
  }, [existingChart])

  // Auto-select columns when result arrives and none are set
  useEffect(() => {
    if (result && result.columns.length >= 2) {
      if (!xColumn) setXColumn(result.columns[0])
      if (!yColumn) setYColumn(result.columns[1])
    }
  }, [result, xColumn, yColumn])

  // Builder mode: sync schema auto-select
  useEffect(() => {
    if (schemas.length === 1 && !bSchema) setBSchema(schemas[0].name)
  }, [schemas, bSchema])

  const selectedSchema = schemas.find((s) => s.name === bSchema)
  const tables = selectedSchema?.tables ?? []

  function buildSQL(): string {
    if (!bTable || !bGroupBy) return ''
    if (!SAFE_IDENT.test(bGroupBy)) return ''
    if (bMetric && bAgg !== 'COUNT' && !SAFE_IDENT.test(bMetric)) return ''
    const metric = bMetric && bAgg !== 'COUNT' ? `${bAgg}("${bMetric}")` : 'COUNT(*)'
    const schemaPrefix = bSchema ? `"${bSchema}".` : ''
    return `SELECT "${bGroupBy}", ${metric} AS value\nFROM ${schemaPrefix}"${bTable}"\nGROUP BY "${bGroupBy}"\nORDER BY value DESC\nLIMIT ${bLimit}`
  }

  const runMutation = useMutation({
    mutationFn: async () => {
      const effectiveSql = mode === 'builder' ? buildSQL() : sql
      if (mode === 'builder') setSql(effectiveSql)
      return runQuery(connectionId, effectiveSql, 1000)
    },
    onSuccess: (rs) => {
      setResult(rs)
      setRunError(null)
      setResultView('chart')
    },
    onError: (err: Error) => {
      setRunError(err.message)
      setResult(null)
    },
  })

  const saveMutation = useMutation({
    mutationFn: async () => {
      const payload = {
        name,
        connection_id: connectionId,
        sql: mode === 'builder' ? buildSQL() : sql,
        x_column: xColumn,
        y_column: yColumn,
        chart_type: chartType,
      }
      return isEditing ? updateChart(chartId, payload) : createChart(payload)
    },
    onSuccess: (saved) => {
      qc.invalidateQueries({ queryKey: ['charts'] })
      setSaveError(null)
      if (!isEditing) navigate(`/charts/${saved.id}`)
    },
    onError: (err: Error) => setSaveError(err.message),
  })

  const rerunMutation = useMutation({
    mutationFn: () => executeChart(chartId!),
    onSuccess: (rs) => {
      setResult(rs)
      setRunError(null)
      setResultView('chart')
    },
    onError: (err: Error) => setRunError(err.message),
  })

  const canRun = !!connectionId && (mode === 'sql' ? !!sql.trim() : !!buildSQL())
  const canSave = !!name && !!connectionId && (mode === 'sql' ? !!sql.trim() : !!buildSQL())

  const effectiveSql = mode === 'builder' ? buildSQL() : sql

  return (
    <div className="flex flex-col h-full overflow-hidden">
      {/* Header */}
      <div className="flex items-center gap-3 px-4 py-3 border-b bg-white shrink-0">
        <span className="text-lg">📊</span>
        <h2 className="font-semibold text-gray-800">{isEditing ? 'Edit Chart' : 'New Chart'}</h2>
      </div>

      <div className="flex-1 overflow-auto">
        <div className="p-4 space-y-4 max-w-4xl">
          {/* Name + Connection row */}
          <div className="flex gap-4">
            <div className="flex-1">
              <label className="block text-xs font-medium text-gray-600 mb-1">Chart Name</label>
              <input
                type="text"
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="e.g. Orders by City"
                className="w-full border rounded px-3 py-1.5 text-sm focus:outline-none focus:ring-2 focus:ring-blue-400"
              />
            </div>
            <div className="flex-1">
              <label className="block text-xs font-medium text-gray-600 mb-1">Connection</label>
              <select
                value={connectionId}
                onChange={(e) => { setConnectionId(e.target.value); setResult(null) }}
                className="w-full border rounded px-3 py-1.5 text-sm focus:outline-none focus:ring-2 focus:ring-blue-400 bg-white"
              >
                <option value="">— select —</option>
                {connections.map((c) => (
                  <option key={c.id} value={c.id}>{c.name} ({c.engine})</option>
                ))}
              </select>
            </div>
          </div>

          {/* Mode toggle */}
          <div className="flex gap-1 rounded-lg bg-gray-100 p-1 w-fit">
            {(['sql', 'builder'] as const).map((m) => (
              <button
                key={m}
                onClick={() => setMode(m)}
                className={`px-3 py-1 text-xs rounded-md font-medium transition-colors ${
                  mode === m ? 'bg-white shadow text-gray-800' : 'text-gray-500 hover:text-gray-700'
                }`}
              >
                {m === 'sql' ? '⌨ SQL' : '🔧 Builder'}
              </button>
            ))}
          </div>

          {/* SQL mode */}
          {mode === 'sql' && (
            <div>
              <label className="block text-xs font-medium text-gray-600 mb-1">
                SQL Query <span className="text-gray-400 font-normal">(should return grouped data)</span>
              </label>
              <textarea
                rows={6}
                value={sql}
                onChange={(e) => setSql(e.target.value)}
                placeholder={'SELECT city, COUNT(*) AS count\nFROM orders\nGROUP BY city\nORDER BY count DESC\nLIMIT 20'}
                className="w-full border rounded px-3 py-2 font-mono text-sm focus:outline-none focus:ring-2 focus:ring-blue-400 resize-y"
              />
            </div>
          )}

          {/* Builder mode */}
          {mode === 'builder' && (
            <div className="space-y-3 border rounded-lg p-4 bg-gray-50">
              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label className="block text-xs font-medium text-gray-600 mb-1">Schema</label>
                  <select
                    value={bSchema}
                    onChange={(e) => { setBSchema(e.target.value); setBTable('') }}
                    className="w-full border rounded px-2 py-1.5 text-sm bg-white focus:outline-none focus:ring-2 focus:ring-blue-400"
                    disabled={!connectionId}
                  >
                    <option value="">— select —</option>
                    {schemas.map((s) => <option key={s.name} value={s.name}>{s.name}</option>)}
                  </select>
                </div>
                <div>
                  <label className="block text-xs font-medium text-gray-600 mb-1">Table</label>
                  <select
                    value={bTable}
                    onChange={(e) => setBTable(e.target.value)}
                    className="w-full border rounded px-2 py-1.5 text-sm bg-white focus:outline-none focus:ring-2 focus:ring-blue-400"
                    disabled={!bSchema}
                  >
                    <option value="">— select —</option>
                    {tables.map((t) => <option key={t.name} value={t.name}>{t.name}</option>)}
                  </select>
                </div>
                <div>
                  <label className="block text-xs font-medium text-gray-600 mb-1">Group By (X column)</label>
                  <input
                    type="text"
                    value={bGroupBy}
                    onChange={(e) => setBGroupBy(e.target.value)}
                    placeholder="e.g. city"
                    className="w-full border rounded px-2 py-1.5 text-sm focus:outline-none focus:ring-2 focus:ring-blue-400"
                  />
                </div>
                <div>
                  <label className="block text-xs font-medium text-gray-600 mb-1">Aggregation</label>
                  <div className="flex gap-2">
                    <select
                      value={bAgg}
                      onChange={(e) => setBAgg(e.target.value)}
                      className="border rounded px-2 py-1.5 text-sm bg-white focus:outline-none focus:ring-2 focus:ring-blue-400"
                    >
                      {AGG_FNS.map((a) => <option key={a}>{a}</option>)}
                    </select>
                    {bAgg !== 'COUNT' && (
                      <input
                        type="text"
                        value={bMetric}
                        onChange={(e) => setBMetric(e.target.value)}
                        placeholder="column"
                        className="flex-1 border rounded px-2 py-1.5 text-sm focus:outline-none focus:ring-2 focus:ring-blue-400"
                      />
                    )}
                  </div>
                </div>
                <div>
                  <label className="block text-xs font-medium text-gray-600 mb-1">Row Limit</label>
                  <input
                    type="number"
                    value={bLimit}
                    min={1} max={1000}
                    onChange={(e) => setBLimit(Number(e.target.value))}
                    className="w-full border rounded px-2 py-1.5 text-sm focus:outline-none focus:ring-2 focus:ring-blue-400"
                  />
                </div>
              </div>
              {effectiveSql && (
                <div>
                  <p className="text-xs font-medium text-gray-500 mb-1">Generated SQL</p>
                  <pre className="text-xs bg-gray-100 rounded p-2 text-gray-700 overflow-auto">{effectiveSql}</pre>
                </div>
              )}
            </div>
          )}

          {/* Chart type + column pickers */}
          <div className="flex flex-wrap gap-4 items-end">
            <div>
              <label className="block text-xs font-medium text-gray-600 mb-1">Chart Type</label>
              <div className="flex gap-1">
                {CHART_TYPES.map(({ type, label, icon }) => (
                  <button
                    key={type}
                    onClick={() => setChartType(type)}
                    className={`px-3 py-1.5 text-xs rounded border font-medium transition-colors ${
                      chartType === type
                        ? 'bg-blue-500 border-blue-500 text-white'
                        : 'bg-white border-gray-300 text-gray-600 hover:border-blue-400'
                    }`}
                  >
                    {icon} {label}
                  </button>
                ))}
              </div>
            </div>
            {result && (
              <>
                <div>
                  <label className="block text-xs font-medium text-gray-600 mb-1">X Axis</label>
                  <select
                    value={xColumn}
                    onChange={(e) => setXColumn(e.target.value)}
                    className="border rounded px-2 py-1.5 text-sm bg-white focus:outline-none focus:ring-2 focus:ring-blue-400"
                  >
                    <option value="">— column —</option>
                    {result.columns.map((c) => <option key={c}>{c}</option>)}
                  </select>
                </div>
                <div>
                  <label className="block text-xs font-medium text-gray-600 mb-1">Y Axis (value)</label>
                  <select
                    value={yColumn}
                    onChange={(e) => setYColumn(e.target.value)}
                    className="border rounded px-2 py-1.5 text-sm bg-white focus:outline-none focus:ring-2 focus:ring-blue-400"
                  >
                    <option value="">— column —</option>
                    {result.columns.map((c) => <option key={c}>{c}</option>)}
                  </select>
                </div>
              </>
            )}
          </div>

          {/* Action bar */}
          <div className="flex items-center gap-2 flex-wrap">
            <button
              onClick={() => runMutation.mutate()}
              disabled={!canRun || runMutation.isPending}
              className="px-4 py-2 bg-emerald-500 hover:bg-emerald-600 text-white text-sm rounded font-medium disabled:opacity-50 transition-colors"
            >
              {runMutation.isPending ? '⏳ Running…' : '▶ Run Query'}
            </button>
            {isEditing && (
              <button
                onClick={() => rerunMutation.mutate()}
                disabled={rerunMutation.isPending}
                className="px-3 py-2 bg-gray-100 hover:bg-gray-200 text-gray-700 text-sm rounded font-medium disabled:opacity-50 transition-colors"
              >
                ↺ Re-run Saved
              </button>
            )}
            <button
              onClick={() => saveMutation.mutate()}
              disabled={!canSave || saveMutation.isPending}
              className="px-4 py-2 bg-blue-500 hover:bg-blue-600 text-white text-sm rounded font-medium disabled:opacity-50 transition-colors"
            >
              {saveMutation.isPending ? 'Saving…' : isEditing ? 'Save Changes' : 'Save Chart'}
            </button>
            {saveError && <span className="text-sm text-red-600">{saveError}</span>}
            {saveMutation.isSuccess && !saveError && <span className="text-sm text-green-600">Saved ✓</span>}
          </div>

          {/* Results */}
          {runError && (
            <div className="p-3 bg-red-50 border border-red-200 rounded text-red-700 text-sm">{runError}</div>
          )}

          {result && (
            <div className="border rounded overflow-hidden">
              {/* Tab bar */}
              <div className="flex items-center gap-0 border-b bg-gray-50 px-3">
                {(['chart', 'table'] as const).map((v) => (
                  <button
                    key={v}
                    onClick={() => setResultView(v)}
                    className={`px-4 py-2 text-xs font-medium border-b-2 transition-colors ${
                      resultView === v
                        ? 'border-blue-500 text-blue-600'
                        : 'border-transparent text-gray-500 hover:text-gray-700'
                    }`}
                  >
                    {v === 'chart' ? '📊 Chart' : '🗃 Table'}
                  </button>
                ))}
                <span className="ml-auto text-xs text-gray-400 pr-2">{result.total} rows</span>
              </div>

              {resultView === 'chart' ? (
                <div className="p-4">
                  <ChartView
                    result={result}
                    xColumn={xColumn}
                    yColumn={yColumn}
                    chartType={chartType}
                    height={320}
                  />
                </div>
              ) : (
                <div style={{ maxHeight: 400 }} className="overflow-auto">
                  <ResultGrid result={result} />
                </div>
              )}
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
