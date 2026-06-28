import { useRef, useState } from 'react'
import { ArrowUp, Square } from 'lucide-react'
import { Button } from '@/components/ui/button'

const SUGGESTIONS = ['花火配什么队', '想抽个能带罗刹的 DPS', '我有花火、银狼、刃，缺什么', '知更鸟适合追击队吗']

export function ChatInput({
  onSend,
  onStop,
  busy,
  showSuggestions,
}: {
  onSend: (text: string) => void
  onStop: () => void
  busy: boolean
  showSuggestions: boolean
}) {
  const [value, setValue] = useState('')
  const ref = useRef<HTMLTextAreaElement>(null)

  const submit = () => {
    const text = value.trim()
    if (!text || busy) return
    onSend(text)
    setValue('')
    if (ref.current) ref.current.style.height = 'auto'
  }

  const onKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      submit()
    }
  }

  const autoGrow = () => {
    const el = ref.current
    if (!el) return
    el.style.height = 'auto'
    el.style.height = `${Math.min(el.scrollHeight, 200)}px`
  }

  return (
    <div className="mx-auto w-full max-w-3xl">
      {showSuggestions && (
        <div className="mb-3 flex flex-wrap gap-2">
          {SUGGESTIONS.map((s) => (
            <button
              key={s}
              onClick={() => onSend(s)}
              className="rounded-full border border-border bg-card px-3 py-1.5 text-xs text-muted-foreground ring-1 ring-foreground/5 transition-colors hover:bg-muted hover:text-foreground"
            >
              {s}
            </button>
          ))}
        </div>
      )}
      <div className="flex items-end gap-2 rounded-2xl border border-border bg-card p-2 ring-1 ring-foreground/5 focus-within:ring-2 focus-within:ring-ring/40">
        <textarea
          ref={ref}
          value={value}
          onChange={(e) => {
            setValue(e.target.value)
            autoGrow()
          }}
          onKeyDown={onKeyDown}
          rows={1}
          placeholder="问点什么…（Enter 发送，Shift+Enter 换行）"
          className="max-h-[200px] flex-1 resize-none bg-transparent px-2 py-1.5 text-sm outline-none placeholder:text-muted-foreground"
        />
        {busy ? (
          <Button size="icon" variant="secondary" onClick={onStop} title="停止">
            <Square className="size-4" />
          </Button>
        ) : (
          <Button size="icon" onClick={submit} disabled={!value.trim()} title="发送">
            <ArrowUp className="size-4" />
          </Button>
        )}
      </div>
    </div>
  )
}
