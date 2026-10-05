// Computes the references available in each field of a step, following the
// scope rules of internal/check.
import { ActionInfo, Step } from './model'

/** Where a value is written. */
export type Field = 'inputs' | 'foreach' | 'params' | 'expects' | 'outputs' | 'actionOutputs'

export interface Ref {
  /** The expression to insert, such as "inputs.users". */
  expr: string
  detail?: string
  /**
   * Set for an output of a preceding step, which can only be received via
   * inputs: picking it adds an input bound to it and inserts that input.
   */
  via?: { step: string; output: string }
}

export interface RefGroup {
  label: string
  refs: Ref[]
}

/** The steps a step belongs to, and the enclosing action, if any. */
export interface Scope {
  steps: Step[]
  /** Parameters of the enclosing action; undefined at the top level of a workflow. */
  params?: string[]
  actions: Map<string, ActionInfo>
}

const httpRes = ['status', 'headers', 'body', 'req']

export function refsFor(scope: Scope, index: number, field: Field): RefGroup[] {
  const groups: RefGroup[] = []
  if (scope.params) {
    groups.push({ label: 'アクションのパラメータ', refs: scope.params.map((p) => ({ expr: `params.${p}` })) })
  }
  if (field === 'actionOutputs') {
    // Action outputs receive the outputs of the action's steps through
    // outputs.steps, not via inputs.
    const refs = published(scope.steps, scope.steps.length).map((r) => ({
      expr: `outputs.steps.${r.via!.step}.${r.via!.output}`,
    }))
    groups.push({ label: 'ステップの出力', refs })
    return groups.filter((g) => g.refs.length > 0)
  }

  const step = scope.steps[index]
  const preceding = published(scope.steps, index)
  if (field === 'inputs') {
    groups.push({ label: '前のステップの出力', refs: preceding })
    return groups.filter((g) => g.refs.length > 0)
  }

  groups.push({
    label: 'このステップの inputs',
    refs: step.inputs.filter((i) => i.name).map((i) => ({ expr: `inputs.${i.name}` })),
  })
  if (preceding.length > 0) {
    groups.push({
      label: '前のステップの出力 (inputs に追加)',
      refs: preceding.map((r) => ({ ...r, detail: 'inputs 経由で受け取る' })),
    })
  }
  if (step.foreach.trim() && field !== 'foreach') {
    groups.push({
      label: 'foreach',
      refs: [
        { expr: 'item', detail: '現在の要素' },
        { expr: 'index', detail: '0 から始まる番号' },
      ],
    })
  }
  if (field === 'expects' || field === 'outputs') {
    const callee = scope.actions.get(step.use)
    const refs: Ref[] = []
    for (const o of callee?.outputs ?? []) {
      refs.push({ expr: `outputs.current.${o}` })
      if (callee?.kind === 'http' && o === 'res') {
        refs.push(...httpRes.map((k) => ({ expr: `outputs.current.res.${k}` })))
      }
    }
    groups.push({ label: `${step.use || 'アクション'} の出力`, refs })
  }
  return groups.filter((g) => g.refs.length > 0)
}

/** Lists the outputs published by the steps before index. */
function published(steps: Step[], index: number): Ref[] {
  const refs: Ref[] = []
  for (const s of steps.slice(0, index)) {
    if (!s.id) continue
    for (const o of s.outputs) {
      if (o.name) refs.push({ expr: `outputs.${s.id}.${o.name}`, via: { step: s.id, output: o.name } })
    }
  }
  return refs
}

/**
 * Returns the name of the input bound to the output of a preceding step,
 * adding one to inputs if needed.
 */
export function inputFor(step: Step, via: { step: string; output: string }): { name: string; step: Step } {
  const value = `<<outputs.${via.step}.${via.output}>>`
  const existing = step.inputs.find((i) => i.value.trim() === value)
  if (existing) return { name: existing.name, step }
  const taken = new Set(step.inputs.map((i) => i.name))
  let name = via.output
  if (taken.has(name)) name = `${via.step}_${via.output}`
  for (let n = 2; taken.has(name); n++) name = `${via.step}_${via.output}${n}`
  return { name, step: { ...step, inputs: [...step.inputs, { name, value }] } }
}
