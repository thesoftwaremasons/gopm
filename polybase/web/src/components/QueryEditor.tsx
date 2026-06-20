import { useState, useEffect } from 'react'
import { useMutation } from '@tanstack/react-query'
import { useParams } from 'react-router-dom'
import { runQuery } from '../api/client'
import { ResultGrid } from './ResultGrid'
import { ChartView } from './ChartView'
import type { ResultSet, ChartType } from '../types'

const CHART_TYPES: { type: ChartType; label: string }[] = [
  { type: 'bar',  label: '▊ Bar'  },
  { type: 'line', label: '📈 Line' },
  { type: 'area', label: '◼ Area' },
  { type: 'pie',  label: '◕ Pie'  },
]

export function QueryEditor() {
  const { id } = useParams<{ id: string }>()
  const [query, setQuery] = useState('')
  const [result, setResult] = useState<ResultSet | null>(null)
  const [resultView, setResultView] = useState<'table' | 'chart'>('table')

  // Chart config (in-memory, not saved)
  const [chartX, setChartX] = useState('')
  const [chartY, setChartY] = useState('')
  const [chartType, setChartType] = useState<ChartType>('bar')

  // Auto-pick sensible column defaults when a new result arrives
  useEffect(() => {
    if (result && result.columns.length >= 2) {
      setChartX(result.columns[0])
      setChartY(result.columns[1])
    }
  }, [result])

  const mutation = useMutation({
    mutationFn: ({ q, limit }: { q: string; limit: number }) => runQuery(id!, q, limit),
    onSuccess: (data) => {
      setResult(data)
      setResultView('table')
    },
  })

  const handleRun = () => {
    if (!query.trim() || !id) return
    mutation.mutate({ q: query, limit: 1000 })
  }

  const handleKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) {
      e.preventDefault()
      handleRun()
    }
  }

  return (
    <div className="flex flex-col h-full">
      {/* Header */}
      <div className="flex items-center justify-between px-4 py-3 border-b bg-white shrink-0">
        <h2 className="font-semibold text-gray-800">Query Editor</h2>
        <button
          onClick={handleRun}
          disabled={mutation.isPending || !query.trim()}
          className="px-4 py-1.5 bg-blue-500 text-white text-sm rounded hover:bg-blue-600 disabled:opacity-50 transition-colors"
        >
          {mutation.isPending ? 'Running…' : 'Run (Ctrl+Enter)'}
        </button>
      </div>

      {/* Editor area */}
      <div className="flex flex-col shrink-0" style={{ height: '36%' }}>
        <textarea
          className="flex-1 w-full p-4 font-mono text-sm resize-none border-b focus:outline-none focus:ring-2 focus:ring-inset focus:ring-blue-200"
          placeholder={
            id
              ? 'Enter your query here…\nPostgreSQL: SELECT city, COUNT(*) FROM orders GROUP BY city\nMongoDB: collection_name\n{"field": "value"}'
              : 'Select a connection first'
          }
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          onKeyDown={handleKeyDown}
          disabled={!id}
        />
      </div>

      {/* Results area */}
      <div className="flex-1 overflow-hidden flex flex-col">
        {mutation.error && (
          <div className="px-4 py-3 text-red-600 bg-red-50 border-b text-sm shrink-0">
            {(mutation.error as Error).message}
          </div>
        )}

        {result && !mutation.error ? (
          <>
            {/* Tab bar */}
            <div className="flex items-center border-b bg-gray-50 px-3 shrink-0">
              {(['table', 'chart'] as const).map((v) => (
                <button
                  key={v}
                  onClick={() => setResultView(v)}
                  className={`px-4 py-2 text-xs font-medium border-b-2 transition-colors ${
                    resultView === v
                      ? 'border-blue-500 text-blue-600'
                      : 'border-transparent text-gray-500 hover:text-gray-700'
                  }`}
                >
                  {v === 'table' ? '🗃 Table' : '📊 Chart'}
                </button>
              ))}
              <span className="ml-auto text-xs text-gray-400 pr-2">{result.total} rows</span>
            </div>

            {resultView === 'table' ? (
              <div className="flex-1 overflow-hidden">
                <ResultGrid result={result} />
              </div>
            ) : (
              <div className="flex-1 overflow-auto p-4 space-y-3">
                {/* Chart controls */}
                <div className="flex flex-wrap gap-3 items-end">
                  <div>
                    <label className="block text-xs font-medium text-gray-600 mb-1">Chart Type</label>
                    <div className="flex gap-1">
                      {CHART_TYPES.map(({ type, label }) => (
                        <button
                          key={type}
                          onClick={() => setChartType(type)}
                          className={`px-2.5 py-1 text-xs rounded border font-medium transition-colors ${
                            chartType === type
                              ? 'bg-blue-500 border-blue-500 text-white'
                              : 'bg-white border-gray-300 text-gray-600 hover:border-blue-400'
                          }`}
                        >
                          {label}
                        </button>
                      ))}
                    </div>
                  </div>
                  <div>
                    <label className="block text-xs font-medium text-gray-600 mb-1">X Axis</label>
                    <select
                      value={chartX}
                      onChange={(e) => setChartX(e.target.value)}
                      className="border rounded px-2 py-1 text-sm bg-white focus:outline-none focus:ring-2 focus:ring-blue-400"
                    >
                      {result.columns.map((c) => <option key={c}>{c}</option>)}
                    </select>
                  </div>
                  <div>
                    <label className="block text-xs font-medium text-gray-600 mb-1">Y Axis</label>
                    <select
                      value={chartY}
                      onChange={(e) => setChartY(e.target.value)}
                      className="border rounded px-2 py-1 text-sm bg-white focus:outline-none focus:ring-2 focus:ring-blue-400"
                    >
                      {result.columns.map((c) => <option key={c}>{c}</option>)}
                    </select>
                  </div>
                </div>
                <ChartView
                  result={result}
                  xColumn={chartX}
                  yColumn={chartY}
                  chartType={chartType}
                  height={280}
                />
              </div>
            )}
          </>
        ) : !mutation.isPending && !mutation.error ? (
          <div className="flex items-center justify-center h-full text-gray-400">
            <div className="text-center">
              <div className="text-3xl mb-2">⚡</div>
              <div className="text-sm">Results will appear here</div>
              <div className="text-xs mt-1 text-gray-300">Press Ctrl+Enter to run</div>
            </div>
          </div>
        ) : null}
      </div>
    </div>
  )
}
