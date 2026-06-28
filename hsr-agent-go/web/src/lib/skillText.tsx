import { Fragment, type CSSProperties, type ReactNode } from 'react'

// 解析米哈游技能描述里的数值占位符 + 富文本标签,输出 React 节点(不用 innerHTML)。
//
// 占位符: #K[i] / #K[f1] / #K[f2],可紧跟 % -> 取 param_list[K-1] 按格式渲染。
//   #4[i]%  且 param=1.296  -> "130%"
//   #1[f1]% 且 param=0.2432 -> "24.3%"
// 富文本: <unbreak>..</unbreak> 去壳, <u>..</u> 下划线,
//   <color=#RRGGBBAA>..</color> 上色(8 位 hex 取前 6), <icon .../> 删除。

function formatPlaceholder(idx: number, fmt: string, percent: boolean, params: number[]): string {
  const raw = params[idx - 1]
  if (raw == null) return percent ? '?%' : '?'
  const scaled = percent ? raw * 100 : raw
  let text: string
  if (fmt === 'i') text = String(Math.round(scaled))
  else if (fmt === 'f1') text = scaled.toFixed(1)
  else if (fmt === 'f2') text = scaled.toFixed(2)
  else text = String(scaled)
  return percent ? `${text}%` : text
}

// 单次扫描:在每个位置匹配「标签」或「占位符」,其余为纯文本。
const TOKEN = /<(\/?)([a-zA-Z]+)([^>]*)>|#(\d+)\[(i|f1|f2)\](%?)/g

export function renderSkillDesc(desc: string, params: number[]): ReactNode {
  if (!desc) return null

  const out: ReactNode[] = []
  const stack: { tag: string; color?: string }[] = []
  let key = 0
  let last = 0

  const styleOf = (): CSSProperties => {
    const style: CSSProperties = {}
    for (const s of stack) {
      if (s.tag === 'u') style.textDecoration = 'underline'
      if (s.tag === 'color' && s.color) style.color = s.color
    }
    return style
  }

  const pushText = (raw: string) => {
    if (!raw) return
    const style = styleOf()
    const hasStyle = Object.keys(style).length > 0
    const lines = raw.split(/\\n|\n/)
    lines.forEach((line, i) => {
      if (i > 0) out.push(<br key={key++} />)
      if (!line) return
      out.push(
        hasStyle ? (
          <span key={key++} style={style}>
            {line}
          </span>
        ) : (
          <Fragment key={key++}>{line}</Fragment>
        ),
      )
    })
  }

  TOKEN.lastIndex = 0
  let m: RegExpExecArray | null
  while ((m = TOKEN.exec(desc)) !== null) {
    if (m.index > last) pushText(desc.slice(last, m.index))
    last = TOKEN.lastIndex

    if (m[2] !== undefined) {
      // 标签分支
      const closing = m[1] === '/'
      const tag = m[2].toLowerCase()
      const attr = m[3] ?? ''
      if (tag === 'icon' || tag === 'unbreak') {
        // icon 丢弃;unbreak 去壳(开闭都不产出节点,也不入栈)
        continue
      }
      if (closing) {
        const i = stack.map((s) => s.tag).lastIndexOf(tag)
        if (i >= 0) stack.splice(i, 1)
      } else {
        let color: string | undefined
        const cm = /=\s*#([0-9a-fA-F]{6})[0-9a-fA-F]{0,2}/.exec(attr)
        if (cm) color = `#${cm[1]}`
        stack.push({ tag, color })
      }
    } else {
      // 占位符分支:m[4]=K m[5]=fmt m[6]=%
      const value = formatPlaceholder(Number(m[4]), m[5], m[6] === '%', params)
      out.push(
        <span
          key={key++}
          className="font-medium text-amber-600 dark:text-amber-400"
          style={styleOf()}
        >
          {value}
        </span>,
      )
    }
  }
  if (last < desc.length) pushText(desc.slice(last))
  return out
}
