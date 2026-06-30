import { useMemo, type ReactNode } from 'react'
import {
  Calculator,
  ClipboardList,
  Filter,
  Gem,
  GitBranch,
  LayoutGrid,
  Link2,
  Network,
  PanelRightClose,
  PanelRightOpen,
  Scale,
  Search,
  SlidersHorizontal,
  Sparkles,
  User,
  Users,
  Wrench,
  X,
  type LucideIcon,
} from 'lucide-react'
import { useChat } from '@/state/ChatContext'
import type { ChatMessage } from '@/components/chat/MessageBubble'
import type { ToolStep } from '@/components/chat/ToolTrace'
import { toolLabel } from '@/lib/toolMeta'
import { cn } from '@/lib/utils'
import { renderToolResult } from './renderers'

const TOOL_ICONS: Record<string, LucideIcon> = {
  get_character: User,
  search_by_role: Filter,
  find_needs: ClipboardList,
  find_buffers_for: Sparkles,
  find_synergies: Users,
  suggest_team: LayoutGrid,
  co_occurrence: Network,
  recommend_lightcones: Sparkles,
  recommend_relics: Gem,
  semantic_search: Search,
  keyword_search: Search,
  resolve_entities: Link2,
  list_character_modifiers: SlidersHorizontal,
  explain_modifier_sources: GitBranch,
  compare_character_fit: Scale,
}

function toolIcon(name: string): LucideIcon {
  if (TOOL_ICONS[name]) return TOOL_ICONS[name]
  if (name.startsWith('estimate_')) return Calculator
  return Wrench
}

interface Turn {
  id: string
  question: string
  steps: ToolStep[]
  streaming: boolean
}

function buildTurns(messages: ChatMessage[]): Turn[] {
  const turns: Turn[] = []
  for (let i = 0; i < messages.length; i++) {
    const m = messages[i]
    if (m.role !== 'assistant') continue
    const prev = messages[i - 1]
    turns.push({
      id: m.id,
      question: prev && prev.role === 'user' ? prev.content : '',
      steps: m.steps ?? [],
      streaming: !!m.streaming,
    })
  }
  return turns.filter((t) => t.steps.length > 0 || t.streaming)
}

// 从 args 里抽一个简短提示(query/name/role 等首个字符串值)
function argHint(args: unknown): string | undefined {
  if (!args || typeof args !== 'object') return undefined
  for (const key of ['query', 'name', 'role', 'q', 'attack_tag']) {
    const v = (args as Record<string, unknown>)[key]
    if (typeof v === 'string' && v) return v
  }
  return undefined
}

function StepCard({ step }: { step: ToolStep }) {
  const Icon = toolIcon(step.name)
  const hint = argHint(step.args)
  const running = step.result === undefined
  return (
    <div className="space-y-1.5">
      <div className="flex items-center gap-1.5 text-xs">
        <Icon className="size-3.5 shrink-0 text-muted-foreground" />
        <span className="font-medium text-foreground">{toolLabel(step.name)}</span>
        {hint && <span className="truncate text-muted-foreground">{hint}</span>}
        {running && (
          <span className="ml-auto inline-flex gap-0.5">
            <span className="size-1 animate-pulse rounded-full bg-current" />
            <span className="size-1 animate-pulse rounded-full bg-current [animation-delay:150ms]" />
            <span className="size-1 animate-pulse rounded-full bg-current [animation-delay:300ms]" />
          </span>
        )}
      </div>
      {running ? (
        <div className="h-8 animate-pulse rounded-lg bg-muted/50" />
      ) : (
        <div>{renderToolResult(step.name, step.result, step.args)}</div>
      )}
    </div>
  )
}

function TurnBlock({ turn }: { turn: Turn }) {
  return (
    <div className="space-y-3 border-b border-border/50 px-3 py-3 last:border-b-0">
      {turn.question && (
        <div className="truncate text-[0.7rem] font-medium text-muted-foreground" title={turn.question}>
          {turn.question}
        </div>
      )}
      {turn.steps.map((step, i) => (
        <StepCard key={step.toolCallId ?? i} step={step} />
      ))}
      {turn.streaming && turn.steps.length === 0 && (
        <p className="text-xs text-muted-foreground">正在思考…</p>
      )}
    </div>
  )
}

function SidebarBody({ turns }: { turns: Turn[] }) {
  if (turns.length === 0) {
    return (
      <div className="flex flex-1 items-center justify-center px-6 text-center">
        <p className="text-xs leading-relaxed text-muted-foreground">
          这里实时显示 agent 查阅的角色、光锥、遗器和配队。
          <br />
          在左侧提问后,它翻资料的过程会在这里长出,可点击跳转。
        </p>
      </div>
    )
  }
  return (
    <div className="flex-1 overflow-y-auto">
      {turns.map((t) => (
        <TurnBlock key={t.id} turn={t} />
      ))}
    </div>
  )
}

function SidebarHeader({ onClose, closeIcon }: { onClose: () => void; closeIcon: ReactNode }) {
  return (
    <div className="flex h-11 shrink-0 items-center gap-2 border-b border-border/60 px-3">
      <Wrench className="size-4 text-muted-foreground" />
      <span className="text-sm font-medium">Agent 工作台</span>
      <button
        onClick={onClose}
        className="ml-auto rounded-md p-1 text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
        aria-label="收起工作台"
      >
        {closeIcon}
      </button>
    </div>
  )
}

export function AgentSidebar() {
  const { messages, sidebarOpen, setSidebarOpen } = useChat()
  const turns = useMemo(() => buildTurns(messages), [messages])

  return (
    <>
      {/* 桌面常驻面板:展开为面板,收起为细条 */}
      <aside
        className={cn(
          'hidden shrink-0 flex-col border-l border-border/60 bg-background/60 lg:flex',
          sidebarOpen ? 'w-80 xl:w-96' : 'w-10',
        )}
      >
        {sidebarOpen ? (
          <>
            <SidebarHeader
              onClose={() => setSidebarOpen(false)}
              closeIcon={<PanelRightClose className="size-4" />}
            />
            <SidebarBody turns={turns} />
          </>
        ) : (
          <button
            onClick={() => setSidebarOpen(true)}
            className="flex h-full w-full flex-col items-center gap-2 py-3 text-muted-foreground transition-colors hover:text-foreground"
            aria-label="展开工作台"
          >
            <PanelRightOpen className="size-4" />
            {turns.length > 0 && (
              <span className="rounded-full bg-primary px-1 text-[0.6rem] text-primary-foreground">
                {turns.length}
              </span>
            )}
          </button>
        )}
      </aside>

      {/* 窄屏抽屉 */}
      {sidebarOpen && (
        <div className="fixed inset-0 z-40 lg:hidden">
          <div
            className="absolute inset-0 bg-foreground/20 backdrop-blur-sm"
            onClick={() => setSidebarOpen(false)}
          />
          <aside className="absolute right-0 top-0 flex h-full w-80 max-w-[85vw] flex-col border-l border-border/60 bg-background shadow-xl">
            <SidebarHeader
              onClose={() => setSidebarOpen(false)}
              closeIcon={<X className="size-4" />}
            />
            <SidebarBody turns={turns} />
          </aside>
        </div>
      )}
    </>
  )
}
