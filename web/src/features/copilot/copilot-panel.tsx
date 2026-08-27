import { Bot, LoaderCircle, Send, Square, X } from 'lucide-react'
import { useRef, useState } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'

import { authenticatedFetch } from '@/api/client'
import { Button } from '@/components/ui/button'

interface ChatMessage {
  role: 'user' | 'assistant'
  content: string
}

export function CopilotPanel() {
  const location = useLocation()
  const navigate = useNavigate()
  const params = new URLSearchParams(location.search)
  const open = params.get('copilot') === '1'
  const product = params.get('product') ?? ''
  const [messages, setMessages] = useState<ChatMessage[]>([])
  const [input, setInput] = useState(product ? `${product} 상품이 출시되지 않는 이유를 설명해 줘.` : '')
  const [model, setModel] = useState(() => localStorage.getItem('dataworks:copilot-model') || 'qwen')
  const [maxTokens, setMaxTokens] = useState(4096)
  const [error, setError] = useState('')
  const [running, setRunning] = useState(false)
  const abortRef = useRef<AbortController | null>(null)

  const close = () => {
    abortRef.current?.abort()
    abortRef.current = null
    params.delete('copilot')
    params.delete('product')
    void navigate({ pathname: location.pathname, search: params.toString() }, { replace: true })
  }

  const send = async () => {
    const question = input.trim()
    if (!question || running) return
    const nextMessages = [...messages, { role: 'user' as const, content: question }]
    setMessages([...nextMessages, { role: 'assistant', content: '' }])
    setInput('')
    setError('')
    localStorage.setItem('dataworks:copilot-model', model.trim())
    const controller = new AbortController()
    abortRef.current = controller
    setRunning(true)
    try {
      await streamCopilot({
        model: model.trim() || 'qwen',
        maxTokens,
        messages: [
          { role: 'system', content: `당신은 Data Works 데이터 상품 운영 전문가입니다. 답변과 권장 조치는 한국어로 간결하게 작성하세요.${product ? ` 현재 상품 키는 ${product}입니다.` : ''}` },
          ...nextMessages,
        ],
        signal: controller.signal,
        onDelta: (delta) => setMessages((current) => current.map((item, index) => index === current.length - 1 ? { ...item, content: item.content + delta } : item)),
      })
    } catch (cause) {
      if (!controller.signal.aborted) setError(cause instanceof Error ? cause.message : 'AI 응답을 받지 못했습니다.')
    } finally {
      abortRef.current = null
      setRunning(false)
      setMessages((current) => current.filter((item) => item.role !== 'assistant' || item.content))
    }
  }

  if (!open) return null
  return (
    <aside className="copilot-panel" aria-label="Data Works 코파일럿">
      <header>
        <span className="grid size-10 place-items-center rounded-xl bg-[var(--ai-soft)] text-[var(--ai)]"><Bot className="size-5" /></span>
        <div className="min-w-0 flex-1"><h2>Data Works 코파일럿</h2><p>스트리밍 AI 운영 도우미</p></div>
        <Button variant="ghost" size="icon" onClick={close} aria-label="코파일럿 닫기"><X className="size-4" /></Button>
      </header>
      <div className="copilot-config">
        <label><span>모델</span><input value={model} onChange={(event) => setModel(event.target.value)} aria-label="코파일럿 모델" /></label>
        <label><span>최대 토큰</span><input type="number" min={1} max={262144} value={maxTokens} onChange={(event) => setMaxTokens(Math.min(262144, Math.max(1, Number(event.target.value) || 1)))} aria-label="코파일럿 최대 토큰" /></label>
      </div>
      <div className="copilot-messages" aria-live="polite">
        {!messages.length ? <div className="copilot-empty"><Bot className="size-7" /><strong>무엇을 점검할까요?</strong><p>출시 차단 이유, 데이터 준비도, 상품 후보와 다음 조치를 물어보세요.</p></div> : null}
        {messages.map((message, index) => <article className={`copilot-message is-${message.role}`} key={`${message.role}-${index}`}><span>{message.role === 'user' ? '나' : '코파일럿'}</span><p>{message.content || <LoaderCircle className="size-4 animate-spin" />}</p></article>)}
      </div>
      {error ? <p className="copilot-error">{error}</p> : null}
      <form className="copilot-compose" onSubmit={(event) => { event.preventDefault(); void send() }}>
        <textarea value={input} onChange={(event) => setInput(event.target.value)} placeholder="예: 이 상품이 왜 출시되지 않나요?" rows={3} />
        <div><span>응답은 기본적으로 실시간 스트리밍됩니다.</span>{running ? <Button type="button" variant="danger" size="sm" onClick={() => abortRef.current?.abort()}><Square className="size-3.5" /> 중지</Button> : <Button type="submit" variant="accent" size="sm" disabled={!input.trim()}><Send className="size-3.5" /> 보내기</Button>}</div>
      </form>
    </aside>
  )
}

async function streamCopilot({ model, maxTokens, messages, signal, onDelta }: { model: string; maxTokens: number; messages: Array<{ role: string; content: string }>; signal: AbortSignal; onDelta: (value: string) => void }) {
  const response = await authenticatedFetch('/v1/chat/completions', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', Accept: 'text/event-stream' },
    body: JSON.stringify({ model, messages, max_tokens: maxTokens, stream: true }),
    signal,
  })
  if (!response.ok) {
    const payload = await response.text()
    throw new Error(payload || `AI 요청 실패 (${response.status})`)
  }
  if (!response.body) throw new Error('스트리밍 응답 본문이 없습니다.')
  const reader = response.body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''
  while (true) {
    const { done, value } = await reader.read()
    buffer += decoder.decode(value, { stream: !done })
    const blocks = buffer.split(/\r?\n\r?\n/)
    buffer = blocks.pop() ?? ''
    for (const block of blocks) {
      for (const line of block.split(/\r?\n/)) {
        if (!line.startsWith('data:')) continue
        const data = line.slice(5).trim()
        if (!data || data === '[DONE]') continue
        try {
          const chunk = JSON.parse(data) as { choices?: Array<{ delta?: { content?: string }; message?: { content?: string } }> }
          const content = chunk.choices?.[0]?.delta?.content ?? chunk.choices?.[0]?.message?.content ?? ''
          if (content) onDelta(content)
        } catch { /* 일부 공급자의 비 JSON keep-alive 프레임은 건너뜁니다. */ }
      }
    }
    if (done) break
  }
}
