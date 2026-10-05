import { Action, ActionInfo, Param, paramTypes, Problem, StepResult } from '../model'
import { refsFor } from '../refs'
import { Bindings } from './Bindings'
import { StepList } from './StepList'

/** Edits a user-defined action: its parameters, steps and outputs. */
export function ActionEditor(props: {
  action: Action
  onChange: (a: Action) => void
  actions: Map<string, ActionInfo>
  servers: string[]
  problems?: Map<number, Problem[]>
  results?: Map<number, StepResult>
}) {
  const { action: a } = props
  const set = (patch: Partial<Action>) => props.onChange({ ...a, ...patch })
  const setParam = (i: number, p: Param) => set({ params: a.params.map((x, j) => (j === i ? p : x)) })
  const params = a.params.map((p) => p.name).filter(Boolean)
  const outputRefs = refsFor({ steps: a.steps, params, actions: props.actions }, a.steps.length, 'actionOutputs')

  return (
    <div className="action-editor">
      <div className="field-block">
        <div className="field-title">params</div>
        <div className="bindings">
          {a.params.map((p, i) => (
            <div className="row" key={i}>
              <input
                className="name"
                value={p.name}
                placeholder="name"
                spellCheck={false}
                onChange={(e) => setParam(i, { ...p, name: e.target.value })}
              />
              <select className="param-type" value={p.type} onChange={(e) => setParam(i, { ...p, type: e.target.value })}>
                {paramTypes.map((t) => (
                  <option key={t}>{t}</option>
                ))}
              </select>
              <span className="spacer" />
              <button
                type="button"
                className="icon"
                title="削除"
                onClick={() => set({ params: a.params.filter((_, j) => j !== i) })}
              >
                ×
              </button>
            </div>
          ))}
          <button type="button" className="add" onClick={() => set({ params: [...a.params, { name: '', type: 'string' }] })}>
            + param
          </button>
        </div>
      </div>

      <div className="field-block">
        <div className="field-title">steps</div>
        <StepList
          steps={a.steps}
          onChange={(steps) => set({ steps })}
          scope={{ params, actions: props.actions }}
          servers={props.servers}
          problems={props.problems}
          results={props.results}
        />
      </div>

      <div className="field-block">
        <div className="field-title">
          outputs <span className="muted">アクションの呼び出し元に返す値</span>
        </div>
        <Bindings items={a.outputs} onChange={(outputs) => set({ outputs })} refs={outputRefs} addLabel="output" />
      </div>
    </div>
  )
}
