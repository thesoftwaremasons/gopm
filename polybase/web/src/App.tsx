import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { Sidebar } from './components/Sidebar'
import { DataBrowser } from './components/DataBrowser'
import { QueryEditor } from './components/QueryEditor'
import { JoinBuilder } from './components/JoinBuilder'

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: 1,
      staleTime: 30_000,
    },
  },
})

function Layout({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex h-screen overflow-hidden bg-gray-50">
      <Sidebar />
      <main className="flex-1 overflow-hidden flex flex-col">
        {children}
      </main>
    </div>
  )
}

function Home() {
  return (
    <div className="flex items-center justify-center h-full text-gray-400">
      <div className="text-center">
        <div className="text-6xl mb-4">🗄</div>
        <h2 className="text-2xl font-semibold text-gray-600 mb-2">Welcome to Polybase</h2>
        <p className="text-sm text-gray-400 max-w-sm">
          Add a database connection from the sidebar to start browsing your data.
        </p>
      </div>
    </div>
  )
}

function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <Routes>
          <Route
            path="/"
            element={
              <Layout>
                <Home />
              </Layout>
            }
          />
          <Route
            path="/connections/:id/browse"
            element={
              <Layout>
                <DataBrowser />
              </Layout>
            }
          />
          <Route
            path="/connections/:id/query"
            element={
              <Layout>
                <QueryEditor />
              </Layout>
            }
          />
          <Route
            path="/joins/new"
            element={
              <Layout>
                <JoinBuilder />
              </Layout>
            }
          />
          <Route
            path="/joins/:joinId"
            element={
              <Layout>
                <JoinBuilder />
              </Layout>
            }
          />
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </BrowserRouter>
    </QueryClientProvider>
  )
}

export default App
