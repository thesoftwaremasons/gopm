import React, { useState } from 'react'
import type { ResultSet } from '../types'

interface Props {
  result: ResultSet
  onPageChange?: (offset: number) => void
  offset?: number
  limit?: number
}

export function ResultGrid({ result, onPageChange, offset = 0, limit = 100 }: Props) {
  const [expandedCell, setExpandedCell] = useState<string | null>(null)
  if (!result.columns.length) return <p className="text-gray-400 p-4">No results.</p>

  const canPrev = offset > 0
  const canNext = result.has_more || result.rows.length === limit

  function formatCell(val: unknown): string {
    if (val === null || val === undefined) return 'NULL'
    if (typeof val === 'object') return JSON.stringify(val)
    return String(val)
  }

  return (
    <div className="flex flex-col gap-2">
      <div className="text-xs text-gray-400">
        {result.total > 0 ? `${result.total} total rows` : `${result.rows.length} rows`}
      </div>
      <div className="overflow-auto rounded border border-gray-700">
        <table className="w-full text-sm text-left text-gray-200">
          <thead className="bg-gray-800 text-gray-300 uppercase text-xs">
            <tr>
              {result.columns.map((col: string) => (
                <th key={col} className="px-3 py-2 whitespace-nowrap border-b border-gray-700">{col}</th>
              ))}
            </tr>
          </thead>
          <tbody>
            {result.rows.map((row: unknown[], ri: number) => (
              <tr key={ri} className="border-b border-gray-800 hover:bg-gray-800/50">
                {row.map((val: unknown, ci: number) => {
                  const key = `${ri}-${ci}`
                  const text = formatCell(val)
                  const isLong = text.length > 60
                  const expanded = expandedCell === key
                  return (
                    <td key={ci} className="px-3 py-1.5 align-top" onClick={() => isLong && setExpandedCell(expanded ? null : key)}>
                      {isLong && !expanded
                        ? <span className="cursor-pointer text-blue-400">{text.slice(0, 60)}…</span>
                        : <span className={isLong ? 'text-blue-300 whitespace-pre-wrap break-all' : ''}>{text}</span>
                      }
                    </td>
                  )
                })}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {onPageChange && (
        <div className="flex gap-2 items-center">
          <button disabled={!canPrev} onClick={() => onPageChange(Math.max(0, offset - limit))}
            className="px-3 py-1 rounded bg-gray-700 text-sm disabled:opacity-40 hover:bg-gray-600">← Prev</button>
          <span className="text-xs text-gray-400">offset {offset}</span>
          <button disabled={!canNext} onClick={() => onPageChange(offset + limit)}
            className="px-3 py-1 rounded bg-gray-700 text-sm disabled:opacity-40 hover:bg-gray-600">Next →</button>
        </div>
      )}
    </div>
  )
}
