import { Outlet } from 'react-router-dom'
import { ChatProvider } from '@/state/ChatContext'
import { Header } from './Header'
import { AgentSidebar } from '@/components/agent/AgentSidebar'

export function Layout() {
  return (
    <ChatProvider>
      <div className="flex h-full flex-col">
        <Header />
        <div className="flex min-h-0 flex-1">
          <main className="min-h-0 flex-1">
            <Outlet />
          </main>
          <AgentSidebar />
        </div>
      </div>
    </ChatProvider>
  )
}
