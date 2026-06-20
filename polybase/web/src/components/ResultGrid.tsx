import type { ResultSet } from '../types'

interface Props {
  result: ResultSet
  onPageChange?: (offset: number) => void
  offset?: number
  limit?: number
}

export function ResultGrid({ result, onPageChange, offset = 0, limit = 100 }: Props) {
  const { columns, rows, total, has_more } = result

  if (columns.length === 0) {
    return (
      <div className="p-8 text-center text-gray-500">
        No data returned
      </div>
    )
  }

  return (
    <div className="flex flex-col h-full">
      {/* Stats bar */}
      <div className="flex items-center justify-between px-4 py-2 bg-gray-50 border-b text-sm text-gray-600">
        <span>
          Showing {offset + 1}–{Math.min(offset + rows.length, total)} of {total} rows
        </span>
        {onPageChange && (
          <div className="flex items-center gap-2">
            <button
              disabled={offset === 0}
              onClick={() => onPageChange(Math.max(0, offset - limit))}
              className="px-2 py-1 rounded border text-xs disabled:opacity-40 hover:bg-gray-100"
            >
              ← Prev
            </button>
            <button
              disabled={!has_more}
              onClick={() => onPageChange(offset + limit)}
              className="px-2 py-1 rounded border text-xs disabled:opacity-40 hover:bg-gray-100"
            >
              Next →
            </button>
          </div>
        )}
      </div>

      {/* Table */}
      <div className="overflow-auto flex-1">
        <table className="w-full text-sm border-collapse">
          <thead className="sticky top-0 bg-gray-100">
            <tr>
              <th className="px-2 py-2 text-left text-xs font-medium text-gray-500 border-b w-10 text-center">
                #
              </th>
              {columns.map((col) => (
                <th
                  key={col}
                  className="px-3 py-2 text-left text-xs font-medium text-gray-600 border-b whitespace-nowrap"
                >
                  {col}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {rows.map((row, i) => (
              <tr
                key={i}
                className={`border-b hover:bg-blue-50 transition-colors ${
                  i % 2 === 0 ? 'bg-white' : 'bg-gray-50'
                }`}
              >
                <td className="px-2 py-1.5 text-xs text-gray-400 text-center">
                  {offset + i + 1}
                </td>
                {row.map((cell, j) => (
                  <td
                    key={j}
                    className="px-3 py-1.5 text-gray-800 whitespace-nowrap max-w-xs overflow-hidden text-ellipsis"
                    title={cell === null ? 'NULL' : String(cell)}
                  >
                    {cell === null ? (
                      <span className="text-gray-400 italic text-xs">NULL</span>
                    ) : typeof cell === 'object' ? (
                      <span className="text-blue-600 font-mono text-xs">
                        {JSON.stringify(cell)}
                      </span>
                    ) : (
                      String(cell)
                    )}
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}
