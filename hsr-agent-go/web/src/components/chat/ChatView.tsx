import { useEffect, useRef, useState } from 'react'
import { streamChat, type ChatStreamHandle } from '@/api/chatStream'
import type { ChatEvent } from '@/api/types'
import { MessageBubble, type ChatMessage } from './MessageBubble'
import { ChatInput } from './ChatInput'
import type { ToolStep } from './ToolTrace'

let counter = 0
const nextId = () => `m${++counter}`

export function ChatView() {
  const [messages, setMessages] = useState<ChatMessage[]>([])
  const [busy, setBusy] = useState(false)
  const streamRef = useRef<ChatStreamHandle | null>(null)
  const bottomRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messages])

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
            steps: [...(m.steps ?? []), { name: event.name, args: event.args } as ToolStep],
          }))
          break
        case 'tool_result':
          patch((m) => {
            const steps = [...(m.steps ?? [])]
            // 把 result 回填到最近一个同名、还没有 result 的 step
            for (let i = steps.length - 1; i >= 0; i--) {
              if (steps[i].name === event.name && steps[i].result === undefined) {
                steps[i] = { ...steps[i], result: event.result }
                break
              }
            }
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

  const empty = messages.length === 0

  return (
    <div className="flex h-full flex-col">
      <div className="flex-1 overflow-y-auto">
        {empty ? (
          <div className="flex h-full flex-col items-center justify-center px-4 text-center">
            <h1 className="font-serif text-2xl font-semibold tracking-tight">星穹铁道配队助手</h1>
            <p className="mt-2 max-w-md text-sm text-muted-foreground">
              基于角色机制与社区配队数据，给出有依据的配队和抽取建议。
            </p>
          </div>
        ) : (
          <div className="mx-auto w-full max-w-3xl space-y-4 px-4 py-6">
            {messages.map((m) => (
              <MessageBubble key={m.id} message={m} />
            ))}
            <div ref={bottomRef} />
          </div>
        )}
      </div>
      <div className="border-t border-border/60 bg-background/80 px-4 py-4 backdrop-blur">
        <ChatInput onSend={send} onStop={stop} busy={busy} showSuggestions={empty} />
      </div>
    </div>
  )
}
