import { useEffect, useRef } from 'react'
import { useChat } from '@/state/ChatContext'
import { MessageBubble } from './MessageBubble'
import { ChatInput } from './ChatInput'

export function ChatView() {
  const { messages, busy, send, stop } = useChat()
  const bottomRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messages])

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
