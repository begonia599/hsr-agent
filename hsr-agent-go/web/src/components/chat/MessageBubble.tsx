import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { cn } from '@/lib/utils'
import { ToolTrace, type ToolStep } from './ToolTrace'

export interface ChatMessage {
  id: string
  role: 'user' | 'assistant'
  content: string
  steps?: ToolStep[]
  streaming?: boolean
  error?: string
}

export function MessageBubble({ message }: { message: ChatMessage }) {
  if (message.role === 'user') {
    return (
      <div className="flex justify-end">
        <div className="max-w-[80%] rounded-2xl rounded-br-sm bg-primary px-4 py-2.5 text-sm text-primary-foreground">
          {message.content}
        </div>
      </div>
    )
  }

  return (
    <div className="flex justify-start">
      <div className="w-full max-w-[90%]">
        {message.steps && <ToolTrace steps={message.steps} busy={!!message.streaming} />}
        {message.error ? (
          <div className="rounded-2xl rounded-bl-sm border border-destructive/40 bg-destructive/10 px-4 py-2.5 text-sm text-destructive">
            {message.error}
          </div>
        ) : message.content ? (
          <div
            className={cn(
              'prose-hsr rounded-2xl rounded-bl-sm bg-card px-4 py-2.5 text-sm ring-1 ring-foreground/10',
            )}
          >
            <ReactMarkdown remarkPlugins={[remarkGfm]}>{message.content}</ReactMarkdown>
          </div>
        ) : message.streaming ? (
          <div className="flex items-center gap-1 px-4 py-2.5 text-muted-foreground">
            <span className="size-1.5 animate-pulse rounded-full bg-current" />
            <span className="size-1.5 animate-pulse rounded-full bg-current [animation-delay:150ms]" />
            <span className="size-1.5 animate-pulse rounded-full bg-current [animation-delay:300ms]" />
          </div>
        ) : null}
      </div>
    </div>
  )
}
