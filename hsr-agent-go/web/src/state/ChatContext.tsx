import { createContext, useContext, useRef, useState, type ReactNode } from 'react'
import { streamChat, type ChatStreamHandle } from '@/api/chatStream'
import type { ChatEvent } from '@/api/types'
import type { ChatMessage } from '@/components/chat/MessageBubble'
import type { ToolStep } from '@/components/chat/ToolTrace'

// 聊天会话状态上提到全局 Provider:挂在 Layout 之上,路由切换(跳实体详情页)不卸载,
// 返回对话页时历史与侧栏内容都在;进行中的 SSE 流也因 streamRef 在此而不中断。

interface ChatContextValue {
  messages: ChatMessage[]
  busy: boolean
  send: (text: string) => void
  stop: () => void
  sidebarOpen: boolean
  setSidebarOpen: (open: boolean | ((v: boolean) => boolean)) => void
}

const ChatContext = createContext<ChatContextValue | null>(null)

let counter = 0
const nextId = () => `m${++counter}`

export function ChatProvider({ children }: { children: ReactNode }) {
  const [messages, setMessages] = useState<ChatMessage[]>([])
  const [busy, setBusy] = useState(false)
  // 桌面默认展开侧栏,窄屏默认收起(避免抽屉盖住对话)
  const [sidebarOpen, setSidebarOpen] = useState(
    () => typeof window !== 'undefined' && window.innerWidth >= 1024,
  )
  const streamRef = useRef<ChatStreamHandle | null>(null)

  const send = (text: string) => {
    if (busy) return
    const userMsg: ChatMessage = { id: nextId(), role: 'user', content: text }
    const assistantId = nextId()
    const assistantMsg: ChatMessage = {
      id: assistantId,
      role: 'assistant',
      content: '',
      steps: [],
      streaming: true,
    }
    setMessages((prev) => [...prev, userMsg, assistantMsg])
    setBusy(true)

    const patch = (fn: (m: ChatMessage) => ChatMessage) =>
      setMessages((prev) => prev.map((m) => (m.id === assistantId ? fn(m) : m)))

    const onEvent = (event: ChatEvent) => {
      switch (event.kind) {
        case 'status':
          break
        case 'tool_call':
          patch((m) => ({
            ...m,
            steps: [
              ...(m.steps ?? []),
              { name: event.name, args: event.args, toolCallId: event.toolCallId } as ToolStep,
            ],
          }))
          break
        case 'tool_result':
          patch((m) => {
            const steps = [...(m.steps ?? [])]
            // 有 tool_call_id 时按它精确配对;否则回退到最近一个同名、还没 result 的 step
            let idx = -1
            if (event.toolCallId) {
              idx = steps.findIndex((s) => s.toolCallId === event.toolCallId)
            }
            if (idx < 0) {
              for (let i = steps.length - 1; i >= 0; i--) {
                if (steps[i].name === event.name && steps[i].result === undefined) {
                  idx = i
                  break
                }
              }
            }
            if (idx >= 0) steps[idx] = { ...steps[idx], result: event.result }
            return { ...m, steps }
          })
          break
        case 'final':
          patch((m) => ({ ...m, content: event.message, streaming: false }))
          break
        case 'error':
          patch((m) => ({
            ...m,
            error: `${event.message}（${event.code}）`,
            streaming: false,
          }))
          break
      }
    }

    streamRef.current = streamChat(text, onEvent, () => {
      patch((m) => ({ ...m, streaming: false }))
      setBusy(false)
      streamRef.current = null
    })
  }

  const stop = () => {
    streamRef.current?.abort()
    streamRef.current = null
    setBusy(false)
  }

  return (
    <ChatContext.Provider value={{ messages, busy, send, stop, sidebarOpen, setSidebarOpen }}>
      {children}
    </ChatContext.Provider>
  )
}

export function useChat(): ChatContextValue {
  const ctx = useContext(ChatContext)
  if (!ctx) throw new Error('useChat must be used within ChatProvider')
  return ctx
}
