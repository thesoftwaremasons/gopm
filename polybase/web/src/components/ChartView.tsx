import {
  BarChart, Bar,
  LineChart, Line,
  AreaChart, Area,
  PieChart, Pie, Cell,
  XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer,
} from 'recharts'
import type { ResultSet, ChartType } from '../types'

const PALETTE = [
  '#3b82f6', '#10b981', '#f59e0b', '#ef4444',
  '#8b5cf6', '#06b6d4', '#f97316', '#84cc16',
]

interface Props {
  result: ResultSet
  xColumn: string
  yColumn: string
  chartType: ChartType
  height?: number
}

interface DataPoint {
  x: string
  y: number
}

function buildData(result: ResultSet, xColumn: string, yColumn: string): DataPoint[] {
  const xi = result.columns.indexOf(xColumn)
  const yi = result.columns.indexOf(yColumn)
  if (xi === -1 || yi === -1) return []
  return result.rows.map((row) => ({
    x: row[xi] != null ? String(row[xi]) : '(null)',
    y: typeof row[yi] === 'number' ? (row[yi] as number) : parseFloat(String(row[yi])) || 0,
  }))
}

const axisStyle = { fontSize: 11, fill: '#6b7280' }
const margin = { top: 10, right: 24, bottom: 60, left: 24 }

// Pie label rendered as SVG text — simpler than a callback to avoid TS issues
const RADIAN = Math.PI / 180
function renderPieLabel(props: Record<string, unknown>) {
  const { cx, cy, midAngle, innerRadius, outerRadius, percent, name } = props as {
    cx: number; cy: number; midAngle: number
    innerRadius: number; outerRadius: number
    percent: number; name: string
  }
  if (percent < 0.04) return null
  const r = Number(innerRadius) + (Number(outerRadius) - Number(innerRadius)) * 1.35
  const x = Number(cx) + r * Math.cos(-Number(midAngle) * RADIAN)
  const y = Number(cy) + r * Math.sin(-Number(midAngle) * RADIAN)
  return (
    <text x={x} y={y} fill="#374151" fontSize={11} textAnchor={x > Number(cx) ? 'start' : 'end'} dominantBaseline="central">
      {`${name} ${(Number(percent) * 100).toFixed(1)}%`}
    </text>
  )
}

export function ChartView({ result, xColumn, yColumn, chartType, height = 320 }: Props) {
  if (!xColumn || !yColumn) {
    return (
      <div className="flex items-center justify-center text-gray-400 text-sm" style={{ height }}>
        Pick X and Y columns to render the chart
      </div>
    )
  }

  const data = buildData(result, xColumn, yColumn)

  if (data.length === 0) {
    return (
      <div className="flex items-center justify-center text-gray-400 text-sm" style={{ height }}>
        No data — check column names
      </div>
    )
  }

  if (chartType === 'pie') {
    return (
      <ResponsiveContainer width="100%" height={height}>
        <PieChart>
          <Pie
            data={data}
            dataKey="y"
            nameKey="x"
            cx="50%"
            cy="50%"
            outerRadius={Math.min(height / 2 - 40, 120)}
            labelLine={false}
            label={renderPieLabel as never}
          >
            {data.map((_, i) => (
              <Cell key={i} fill={PALETTE[i % PALETTE.length]} />
            ))}
          </Pie>
          <Tooltip />
        </PieChart>
      </ResponsiveContainer>
    )
  }

  if (chartType === 'line') {
    return (
      <ResponsiveContainer width="100%" height={height}>
        <LineChart data={data} margin={margin}>
          <CartesianGrid strokeDasharray="3 3" stroke="#e5e7eb" />
          <XAxis dataKey="x" tick={{ ...axisStyle, angle: -30, textAnchor: 'end' }} interval="preserveStartEnd" />
          <YAxis tick={axisStyle} />
          <Tooltip />
          <Line type="monotone" dataKey="y" stroke={PALETTE[0]} dot={data.length < 50} strokeWidth={2} />
        </LineChart>
      </ResponsiveContainer>
    )
  }

  if (chartType === 'area') {
    return (
      <ResponsiveContainer width="100%" height={height}>
        <AreaChart data={data} margin={margin}>
          <CartesianGrid strokeDasharray="3 3" stroke="#e5e7eb" />
          <XAxis dataKey="x" tick={{ ...axisStyle, angle: -30, textAnchor: 'end' }} interval="preserveStartEnd" />
          <YAxis tick={axisStyle} />
          <Tooltip />
          <Area type="monotone" dataKey="y" stroke={PALETTE[0]} fill={PALETTE[0]} fillOpacity={0.25} strokeWidth={2} />
        </AreaChart>
      </ResponsiveContainer>
    )
  }

  // default: bar
  return (
    <ResponsiveContainer width="100%" height={height}>
      <BarChart data={data} margin={margin}>
        <CartesianGrid strokeDasharray="3 3" stroke="#e5e7eb" />
        <XAxis dataKey="x" tick={{ ...axisStyle, angle: -30, textAnchor: 'end' }} interval={0} />
        <YAxis tick={axisStyle} />
        <Tooltip />
        <Bar dataKey="y" radius={[3, 3, 0, 0]}>
          {data.map((_, i) => (
            <Cell key={i} fill={PALETTE[i % PALETTE.length]} />
          ))}
        </Bar>
      </BarChart>
    </ResponsiveContainer>
  )
}
