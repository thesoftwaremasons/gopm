import { useState, useEffect } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { listConnections, listSchemas, getJoin, createJoin, updateJoin, executeJoin } from '../api/client'
import type { ResultSet, SchemaInfo } from '../types'
import { ResultGrid } from './ResultGrid'

interface SideFormProps {
  label: string
  connectionId: string
  setConnectionId: (v: string) => void
  schema: string
  setSchema: (v: string) => void
  table: string
  setTable: (v: string) => void
  keyField: string
  setKeyField: (v: string) => void
  connections: { id: string; name: string; engine: string }[]
  schemas: SchemaInfo[]
  schemasLoading: boolean
}

function SideForm({
  label,
  connectionId, setConnectionId,
  schema, setSchema,
  table, setTable,
  keyField, setKeyField,
  connections, schemas, schemasLoading,
}: SideFormProps) {
  const selectedSchema = schemas.find((s) => s.name === schema)
  const tables = selectedSchema?.tables ?? []

  const handleConnectionChange = (v: string) => {
    setConnectionId(v)
    setSchema('')
    setTable('')
    setKeyField('')
  }
  const handleSchemaChange = (v: string) => {
    setSchema(v)
    setTable('')
  }

  useEffect(() => {
    if (schemas.length === 1 && schema === '') {
      setSchema(schemas[0].name)
    }
  }, [schemas, schema, setSchema])

  return (
    <div className="flex-1 border rounded-lg p-4 bg-gray-50 space-y-3">
      <h3 className="font-semibold text-gray-700 text-sm">{label}</h3>

      <div>
        <label className="block text-xs font-medium text-gray-600 mb-1">Connection</label>
        <select
          value={connectionId}
          onChange={(e) => handleConnectionChange(e.target.value)}
          className="w-full border rounded px-2 py-1.5 text-sm focus:outline-none focus:ring-2 focus:ring-blue-400 bg-white"
        >
          <option value="">— select —</option>
          {connections.map((c) => (
            <option key={c.id} value={c.id}>
              {c.name} ({c.engine})
            </option>
          ))}
        </select>
      </div>

      {connectionId && (
        <div>
          <label className="block text-xs font-medium text-gray-600 mb-1">Schema / Database</label>
          {schemasLoading ? (
            <div className="text-xs text-gray-400 py-1">Loading…</div>
          ) : (
            <select
              value={schema}
              onChange={(e) => handleSchemaChange(e.target.value)}
              className="w-full border rounded px-2 py-1.5 text-sm focus:outline-none focus:ring-2 focus:ring-blue-400 bg-white"
            >
              <option value="">— select —</option>
              {schemas.map((s) => (
                <option key={s.name} value={s.name}>{s.name}</option>
              ))}
            </select>
          )}
        </div>
      )}

      {schema && (
        <div>
          <label className="block text-xs font-medium text-gray-600 mb-1">Table / Collection</label>
          <select
            value={table}
            onChange={(e) => setTable(e.target.value)}
            className="w-full border rounded px-2 py-1.5 text-sm focus:outline-none focus:ring-2 focus:ring-blue-400 bg-white"
          >
            <option value="">— select —</option>
            {tables.map((t) => (
              <option key={t.name} value={t.name}>{t.name}</option>
            ))}
          </select>
        </div>
      )}

      {table && (
        <div>
          <label className="block text-xs font-medium text-gray-600 mb-1">Join Key Field</label>
          <input
            type="text"
            value={keyField}
            onChange={(e) => setKeyField(e.target.value)}
            placeholder="e.g. user_id"
            className="w-full border rounded px-2 py-1.5 text-sm focus:outline-none focus:ring-2 focus:ring-blue-400"
          />
          <p className="text-xs text-gray-400 mt-1">Column name used to match rows between sources</p>
        </div>
      )}
    </div>
  )
}

export function JoinBuilder() {
  const { joinId } = useParams<{ joinId: string }>()
  const navigate = useNavigate()
  const qc = useQueryClient()
  const isEditing = !!joinId

  const [name, setName] = useState('')
  const [sourceA, setSourceA] = useState('')
  const [schemaA, setSchemaA] = useState('')
  const [tableA, setTableA] = useState('')
  const [keyFieldA, setKeyFieldA] = useState('')
  const [sourceB, setSourceB] = useState('')
  const [schemaB, setSchemaB] = useState('')
  const [tableB, setTableB] = useState('')
  const [keyFieldB, setKeyFieldB] = useState('')

  const [result, setResult] = useState<ResultSet | null>(null)
  const [execError, setExecError] = useState<string | null>(null)
  const [saveError, setSaveError] = useState<string | null>(null)

  const { data: connections = [] } = useQuery({
    queryKey: ['connections'],
    queryFn: listConnections,
  })

  const { data: schemasA = [], isLoading: schemasALoading } = useQuery({
    queryKey: ['schemas', sourceA],
    queryFn: () => listSchemas(sourceA),
    enabled: !!sourceA,
  })

  const { data: schemasB = [], isLoading: schemasBLoading } = useQuery({
    queryKey: ['schemas', sourceB],
    queryFn: () => listSchemas(sourceB),
    enabled: !!sourceB,
  })

  const { data: existingJoin } = useQuery({
    queryKey: ['join', joinId],
    queryFn: () => getJoin(joinId!),
    enabled: isEditing,
  })

  useEffect(() => {
    if (existingJoin) {
      setName(existingJoin.name)
      setSourceA(existingJoin.source_a)
      setSchemaA(existingJoin.schema_a)
      setTableA(existingJoin.table_a)
      setKeyFieldA(existingJoin.key_field_a)
      setSourceB(existingJoin.source_b)
      setSchemaB(existingJoin.schema_b)
      setTableB(existingJoin.table_b)
      setKeyFieldB(existingJoin.key_field_b)
    }
  }, [existingJoin])

  const canSave = name && sourceA && tableA && keyFieldA && sourceB && tableB && keyFieldB
  const canExecute = isEditing

  const saveMutation = useMutation({
    mutationFn: async () => {
      const payload = {
        name,
        source_a: sourceA, schema_a: schemaA, table_a: tableA, key_field_a: keyFieldA,
        source_b: sourceB, schema_b: schemaB, table_b: tableB, key_field_b: keyFieldB,
      }
      if (isEditing) {
        return updateJoin(joinId, payload)
      }
      return createJoin(payload)
    },
    onSuccess: (saved) => {
      qc.invalidateQueries({ queryKey: ['joins'] })
      setSaveError(null)
      if (!isEditing) {
        navigate(`/joins/${saved.id}`)
      }
    },
    onError: (err: Error) => setSaveError(err.message),
  })

  const executeMutation = useMutation({
    mutationFn: () => executeJoin(joinId!),
    onSuccess: (rs) => {
      setResult(rs)
      setExecError(null)
    },
    onError: (err: Error) => {
      setExecError(err.message)
      setResult(null)
    },
  })

  return (
    <div className="flex flex-col h-full overflow-hidden">
      {/* Header */}
      <div className="flex items-center gap-3 px-4 py-3 border-b bg-white">
        <span className="text-lg">🔗</span>
        <h2 className="font-semibold text-gray-800">
          {isEditing ? 'Edit Join' : 'New Join'}
        </h2>
      </div>

      <div className="flex-1 overflow-auto p-4 space-y-4">
        {/* Name */}
        <div className="max-w-sm">
          <label className="block text-xs font-medium text-gray-600 mb-1">Join Name</label>
          <input
            type="text"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="e.g. Users × Orders"
            className="w-full border rounded px-3 py-1.5 text-sm focus:outline-none focus:ring-2 focus:ring-blue-400"
          />
        </div>

        {/* Two-column source configuration */}
        <div className="flex gap-4">
          <SideForm
            label="Source A"
            connectionId={sourceA} setConnectionId={setSourceA}
            schema={schemaA} setSchema={setSchemaA}
            table={tableA} setTable={setTableA}
            keyField={keyFieldA} setKeyField={setKeyFieldA}
            connections={connections}
            schemas={schemasA}
            schemasLoading={schemasALoading}
          />
          <div className="flex items-center text-gray-400 font-bold text-xl select-none">⟺</div>
          <SideForm
            label="Source B"
            connectionId={sourceB} setConnectionId={setSourceB}
            schema={schemaB} setSchema={setSchemaB}
            table={tableB} setTable={setTableB}
            keyField={keyFieldB} setKeyField={setKeyFieldB}
            connections={connections}
            schemas={schemasB}
            schemasLoading={schemasBLoading}
          />
        </div>

        {/* Action bar */}
        <div className="flex items-center gap-2">
          <button
            onClick={() => saveMutation.mutate()}
            disabled={!canSave || saveMutation.isPending}
            className="px-4 py-2 bg-blue-500 hover:bg-blue-600 text-white text-sm rounded font-medium disabled:opacity-50 transition-colors"
          >
            {saveMutation.isPending ? 'Saving…' : isEditing ? 'Save Changes' : 'Save Join'}
          </button>
          {canExecute && (
            <button
              onClick={() => executeMutation.mutate()}
              disabled={executeMutation.isPending}
              className="px-4 py-2 bg-emerald-500 hover:bg-emerald-600 text-white text-sm rounded font-medium disabled:opacity-50 transition-colors"
            >
              {executeMutation.isPending ? '⏳ Running…' : '▶ Execute'}
            </button>
          )}
          {saveError && (
            <span className="text-sm text-red-600">{saveError}</span>
          )}
          {saveMutation.isSuccess && !saveError && (
            <span className="text-sm text-green-600">Saved</span>
          )}
        </div>

        {/* Note about left-join semantics */}
        {isEditing && (
          <p className="text-xs text-gray-400">
            Left join: all rows from Source A are returned; Source B columns are <em>null</em> when no key match is found.
            Result columns are prefixed <code>a.</code> and <code>b.</code>.
          </p>
        )}

        {/* Results */}
        {execError && (
          <div className="p-4 bg-red-50 border border-red-200 rounded text-red-700 text-sm">
            {execError}
          </div>
        )}
        {result && (
          <div className="border rounded overflow-hidden" style={{ minHeight: 200 }}>
            <div className="px-3 py-2 bg-gray-50 border-b text-xs text-gray-500 font-medium">
              Join result — {result.total} rows{result.has_more ? ' (capped at 1 000)' : ''}
            </div>
            <ResultGrid result={result} />
          </div>
        )}
      </div>
    </div>
  )
}
