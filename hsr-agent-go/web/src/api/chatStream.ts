import type { ChatEvent } from './types'

// 解析后端 SSE 流。后端用 POST + text/event-stream,EventSource 只支持 GET,
// 所以用 fetch + ReadableStream 手动按帧解析。
//
// 帧格式(server.go writeSSE):
//   event: <type>\n
//   data: <json>\n\n

export interface ChatStreamHandle {
  abort: () => void
}

export function streamChat(
  message: string,
  onEvent: (event: ChatEvent) => void,
  onDone: () => void,
): ChatStreamHandle {
  const controller = new AbortController()

  ;(async () => {
    try {
      const res = await fetch('/api/agent/chat/stream', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ message }),
        signal: controller.signal,
      })

      if (!res.ok || !res.body) {
        let code = 'STREAM_FAILED'
        let detail = res.statusText
        try {
          const body = await res.json()
          code = body?.error?.code ?? code
          detail = body?.error?.message ?? detail
        } catch {
          // 非 JSON 错误体,沿用 statusText
        }
        onEvent({ kind: 'error', code, message: detail })
        onDone()
        return
      }

      const reader = res.body.getReader()
      const decoder = new TextDecoder()
      let buffer = ''

      while (true) {
        const { done, value } = await reader.read()
        if (done) break
        buffer += decoder.decode(value, { stream: true })

        // 按空行分帧
        let sep: number
        while ((sep = buffer.indexOf('\n\n')) !== -1) {
          const frame = buffer.slice(0, sep)
          buffer = buffer.slice(sep + 2)
          const event = parseFrame(frame)
          if (event) onEvent(event)
        }
      }
    } catch (err) {
      if (controller.signal.aborted) {
        // 用户主动取消,不报错
      } else {
        onEvent({
          kind: 'error',
          code: 'NETWORK_ERROR',
          message: err instanceof Error ? err.message : '连接中断',
        })
      }
    } finally {
      onDone()
    }
  })()

  return { abort: () => controller.abort() }
}

function parseFrame(frame: string): ChatEvent | null {
  let eventType = 'message'
  const dataLines: string[] = []
  for (const line of frame.split('\n')) {
    if (line.startsWith('event:')) {
      eventType = line.slice(6).trim()
    } else if (line.startsWith('data:')) {
      dataLines.push(line.slice(5).trim())
    }
  }
  if (dataLines.length === 0) return null

  let data: Record<string, unknown> = {}
  try {
    data = JSON.parse(dataLines.join('\n'))
  } catch {
    return null
  }

  switch (eventType) {
    case 'status':
      return { kind: 'status', message: String(data.message ?? '') }
    case 'tool_call':
      return { kind: 'tool_call', name: String(data.name ?? ''), args: data.args }
    case 'tool_result':
      return { kind: 'tool_result', name: String(data.name ?? ''), result: data.result }
    case 'final':
      return { kind: 'final', message: String(data.message ?? '') }
    case 'error':
      return {
        kind: 'error',
        code: String(data.code ?? 'ERROR'),
        message: String(data.message ?? '未知错误'),
      }
    default:
      return null
  }
}
