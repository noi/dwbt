import { useState } from 'react'
import { ActionInfo, Binding, Expect, Problem, Step, StepResult } from '../model'
import { Field, inputFor, Ref, refsFor, Scope } from '../refs'
import { ActionPalette } from './ActionPalette'
import { Bindings } from './Bindings'
import { Popover } from './Popover'
import { ValueField } from './ValueField'

const methods = ['GET', 'POST', 'PUT', 'PATCH', 'DELETE', 'HEAD', 'OPTIONS']

export interface StepCardProps {
  step: Step
  index: number
  scope: Scope
  servers: string[]
  palette: ActionInfo[]
  problems: Problem[]
  result?: StepResult
  onChange: (s: Step) => void
  onRemove: () => void
  onDuplicate: () => void
  onMove: (delta: number) => void
  dragHandle: React.HTMLAttributes<HTMLSpanElement>
}

export function StepCard(props: StepCardProps) {
  const { step, index, scope } = props
  const [collapsed, setCollapsed] = useState(false)
  const callee = scope.actions.get(step.use)
  const set = (patch: Partial<Step>) => props.onChange({ ...step, ...patch })

  // Inserts a reference into a field. A reference to the output of a
  // preceding step adds an input receiving it and inserts that input.
  const pick = (ref: Ref, insert: (expr: string) => string, write: (s: Step, v: string) => Step) => {
    let s = step
    let expr = ref.expr
    if (ref.via) {
      const r = inputFor(s, ref.via)
      s = r.step
      expr = `inputs.${r.name}`
    }
    props.onChange(write(s, insert(expr)))
  }
  const refs = (f: Field) => refsFor(scope, index, f)
  const bindingPick =
    (key: 'inputs' | 'params' | 'outputs') => (i: number, ref: Ref, insert: (expr: string) => string) =>
      pick(
        ref,
        insert,
        (s, v) => ({ ...s, [key]: s[key].map((b, j) => (j === i ? { ...b, value: v } : b)) }),
      )

  const status = props.result?.status
  const failed = props.problems.length > 0
  return (
    <div className={`step-card${failed ? ' has-problems' : ''}${status ? ` result-${statusClass(status)}` : ''}`}>
      <div className="step-header">
        <span className="handle" title="ドラッグで並べ替え" {...props.dragHandle}>
          ⠿
        </span>
        <span className="step-index">#{index + 1}</span>
        <input
          className="step-id"
          value={step.id}
          placeholder="id"
          spellCheck={false}
          title="ステップ id（outputs を他のステップで使う場合に必要）"
          onChange={(e) => set({ id: e.target.value })}
        />
        <Popover
          label={<span className={`action-chip${callee?.builtin ? ' builtin' : ''}${callee ? '' : ' unknown'}`}>{step.use || 'アクションを選択'}</span>}
          className="use-button"
          title="アクションを変更"
        >
          {(close) => (
            <ActionPalette
              actions={props.palette}
              onPick={(a) => {
                close()
                set({ use: a.name })
              }}
            />
          )}
        </Popover>
        <span className="step-summary muted">{summary(step)}</span>
        <span className="spacer" />
        {status && (
          <span className={`badge ${statusClass(status)}`} title={props.result?.duration}>
            {status}
            {props.result?.duration && <small> {props.result.duration}</small>}
          </span>
        )}
        <button type="button" className="icon" title="上へ" onClick={() => props.onMove(-1)}>
          ↑
        </button>
        <button type="button" className="icon" title="下へ" onClick={() => props.onMove(1)}>
          ↓
        </button>
        <button type="button" className="icon" title="複製" onClick={props.onDuplicate}>
          ⧉
        </button>
        <button type="button" className="icon danger" title="削除" onClick={props.onRemove}>
          ×
        </button>
        <button type="button" className="icon" title={collapsed ? '開く' : '畳む'} onClick={() => setCollapsed(!collapsed)}>
          {collapsed ? '▸' : '▾'}
        </button>
      </div>

      {!collapsed && (
        <div className="step-body">
          {step.inputs.length > 0 && (
            <Section title="inputs" hint="前のステップの出力を受け取る">
              <Bindings
                items={step.inputs}
                onChange={(inputs) => set({ inputs })}
                refs={refs('inputs')}
                onPick={bindingPick('inputs')}
                addLabel="input"
              />
            </Section>
          )}

          {step.foreach !== '' && (
            <Section title="foreach" hint="配列の要素ごとに繰り返す" onRemove={() => set({ foreach: '' })}>
              <ValueField
                value={step.foreach}
                onChange={(v) => set({ foreach: v === '' ? ' ' : v })}
                refs={refs('foreach')}
                onPick={(ref, insert) => pick(ref, insert, (s, v) => ({ ...s, foreach: v }))}
                placeholder="[1, 2, 3]"
              />
            </Section>
          )}

          <Section title="params">
            <Params
              step={step}
              callee={callee}
              servers={props.servers}
              refs={refs('params')}
              onChange={(params) => set({ params })}
              onPick={(i, ref, insert, name) =>
                pick(ref, insert, (s, v) => ({
                  ...s,
                  params: i < s.params.length
                    ? s.params.map((b, j) => (j === i ? { ...b, value: v } : b))
                    : [...s.params, { name: name ?? '', value: v }],
                }))
              }
            />
          </Section>

          {step.expects.length > 0 && (
            <Section title="expects" hint="期待する結果">
              {step.expects.map((ex, i) => (
                <ExpectEditor
                  key={i}
                  expect={ex}
                  kind={callee?.kind ?? ''}
                  refs={refs('expects')}
                  onChange={(e) => set({ expects: step.expects.map((x, j) => (j === i ? e : x)) })}
                  onPick={(ai, ref, insert) =>
                    pick(ref, insert, (s, v) => ({
                      ...s,
                      expects: s.expects.map((x, j) =>
                        j === i ? { ...x, asserts: x.asserts.map((a, k) => (k === ai ? v : a)) } : x,
                      ),
                    }))
                  }
                  onRemove={() => set({ expects: step.expects.filter((_, j) => j !== i) })}
                />
              ))}
              <button type="button" className="add" onClick={() => set({ expects: [...step.expects, newExpect(callee)] })}>
                + expectation
              </button>
            </Section>
          )}

          {step.outputs.length > 0 && (
            <Section title="outputs" hint={step.id ? `outputs.${step.id}.* として公開` : 'outputs を公開するには id が必要です'}>
              <Bindings
                items={step.outputs}
                onChange={(outputs) => set({ outputs })}
                refs={refs('outputs')}
                onPick={bindingPick('outputs')}
                addLabel="output"
              />
            </Section>
          )}

          <div className="step-add-bar">
            {step.inputs.length === 0 && (
              <button type="button" className="add" onClick={() => set({ inputs: [{ name: '', value: '' }] })}>
                + inputs
              </button>
            )}
            {step.foreach === '' && (
              <button type="button" className="add" onClick={() => set({ foreach: ' ' })}>
                + foreach
              </button>
            )}
            {step.expects.length === 0 && (
              <button type="button" className="add" onClick={() => set({ expects: [newExpect(callee)] })}>
                + expects
              </button>
            )}
            {step.outputs.length === 0 && (
              <button type="button" className="add" onClick={() => set({ outputs: [{ name: '', value: '' }] })}>
                + outputs
              </button>
            )}
          </div>

          {props.problems.map((p, i) => (
            <div className="problem" key={i} title={p.message}>
              {p.message.replace(/^.*?:\d+:\d+: /, '')}
            </div>
          ))}
          {props.result?.error && <pre className="result-error">{props.result.error}</pre>}
        </div>
      )}
    </div>
  )
}

function Section(props: { title: string; hint?: string; onRemove?: () => void; children: React.ReactNode }) {
  return (
    <div className="section">
      <div className="section-title">
        <span>{props.title}</span>
        {props.hint && <span className="muted">{props.hint}</span>}
        {props.onRemove && (
          <button type="button" className="icon" title="削除" onClick={props.onRemove}>
            ×
          </button>
        )}
      </div>
      {props.children}
    </div>
  )
}

/**
 * Edits the params of a step. The parameters of the action are listed with
 * their types, the required ones first even when not set yet.
 */
function Params(props: {
  step: Step
  callee?: ActionInfo
  servers: string[]
  refs: ReturnType<typeof refsFor>
  onChange: (params: Binding[]) => void
  /** name is given for a parameter not set yet, which is then added at i. */
  onPick: (i: number, ref: Ref, insert: (expr: string) => string, name?: string) => void
}) {
  const { step, callee } = props
  const spec = new Map((callee?.params ?? []).map((p) => [p.name, p]))
  const given = new Set(step.params.map((p) => p.name))
  const missing = (callee?.params ?? []).filter((p) => p.required && !given.has(p.name))
  const optional = (callee?.params ?? []).filter((p) => !p.required && !given.has(p.name))

  const set = (i: number, b: Binding) => props.onChange(step.params.map((x, j) => (j === i ? b : x)))
  const add = (b: Binding) => props.onChange([...step.params, b])
  const choices = (name: string) => {
    if (callee?.kind !== 'http') return undefined
    if (name === 'server') return props.servers
    if (name === 'method') return methods
    return undefined
  }

  return (
    <div className="bindings">
      {step.params.map((b, i) => {
        const p = spec.get(b.name)
        return (
          <div className="row" key={i}>
            {p ? (
              <label className="name fixed" title={p.type}>
                {b.name}
                {p.required && <span className="req">*</span>}
                <span className="type">{p.type}</span>
              </label>
            ) : (
              <input
                className={`name${callee ? ' unknown' : ''}`}
                value={b.name}
                placeholder="name"
                spellCheck={false}
                title={callee ? `${callee.name} にこのパラメータはありません` : undefined}
                onChange={(e) => set(i, { ...b, name: e.target.value })}
              />
            )}
            <ValueField
              value={b.value}
              choices={choices(b.name)}
              refs={props.refs}
              onChange={(v) => set(i, { ...b, value: v })}
              onPick={(ref, insert) => props.onPick(i, ref, insert)}
            />
            <button
              type="button"
              className="icon"
              title="削除"
              onClick={() => props.onChange(step.params.filter((_, j) => j !== i))}
            >
              ×
            </button>
          </div>
        )
      })}
      {missing.map((p) => (
        <div className="row" key={`missing-${p.name}`}>
          <label className="name fixed missing" title="必須パラメータ">
            {p.name}
            <span className="req">*</span>
            <span className="type">{p.type}</span>
          </label>
          <ValueField
            value=""
            choices={choices(p.name)}
            refs={props.refs}
            placeholder="未入力"
            onChange={(v) => add({ name: p.name, value: v })}
            onPick={(ref, insert) => props.onPick(step.params.length, ref, insert, p.name)}
          />
          <span className="icon-placeholder" />
        </div>
      ))}
      <Popover label="+ パラメータ" className="add">
        {(close) => (
          <div className="ref-menu">
            {optional.map((p) => (
              <button
                type="button"
                key={p.name}
                onClick={() => {
                  close()
                  add({ name: p.name, value: '' })
                }}
              >
                <code>{p.name}</code>
                <span className="muted">{p.type}</span>
              </button>
            ))}
            <button
              type="button"
              onClick={() => {
                close()
                add({ name: '', value: '' })
              }}
            >
              <span>名前を指定して追加</span>
            </button>
          </div>
        )}
      </Popover>
    </div>
  )
}

function ExpectEditor(props: {
  expect: Expect
  kind: string
  refs: ReturnType<typeof refsFor>
  onChange: (e: Expect) => void
  onPick: (assert: number, ref: Ref, insert: (expr: string) => string) => void
  onRemove: () => void
}) {
  const { expect: ex } = props
  const http = props.kind === 'http'
  const setAssert = (i: number, v: string) => props.onChange({ ...ex, asserts: ex.asserts.map((a, j) => (j === i ? v : a)) })
  return (
    <div className="expect">
      <div className="row">
        {http && (
          <>
            <span className="expect-label">status</span>
            <input
              className="status"
              type="number"
              value={ex.status ?? ''}
              placeholder="200"
              onChange={(e) => props.onChange({ ...ex, status: e.target.value === '' ? null : Number(e.target.value) })}
            />
          </>
        )}
        <span className="spacer" />
        <button type="button" className="icon" title="削除" onClick={props.onRemove}>
          ×
        </button>
      </div>
      {ex.asserts.map((a, i) => (
        <div className="row" key={i}>
          <span className="expect-label">assert</span>
          <ValueField
            expression
            value={a}
            refs={props.refs}
            placeholder="outputs.current.res.body.id != nil"
            onChange={(v) => setAssert(i, v)}
            onPick={(ref, insert) => props.onPick(i, ref, insert)}
          />
          <button
            type="button"
            className="icon"
            title="削除"
            onClick={() => props.onChange({ ...ex, asserts: ex.asserts.filter((_, j) => j !== i) })}
          >
            ×
          </button>
        </div>
      ))}
      <button type="button" className="add" onClick={() => props.onChange({ ...ex, asserts: [...ex.asserts, ''] })}>
        + assert
      </button>
    </div>
  )
}

function newExpect(callee?: ActionInfo): Expect {
  return callee?.kind === 'http' ? { status: 200, asserts: [] } : { status: null, asserts: [''] }
}

/** Summarizes a step in its header, such as "POST /api/users". */
function summary(step: Step): string {
  const get = (n: string) => step.params.find((p) => p.name === n)?.value ?? ''
  if (step.use === 'http') return `${get('method')} ${get('path')}`.trim()
  return step.foreach.trim() ? `foreach ${step.foreach.trim()}` : ''
}

export function statusClass(status: string): string {
  switch (status) {
    case 'ok':
      return 'ok'
    case 'FAIL':
      return 'fail'
    case 'ERROR':
      return 'error'
    default:
      return 'skip'
  }
}
