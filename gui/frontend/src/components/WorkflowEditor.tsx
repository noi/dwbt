import { useState } from 'react'
import { ActionInfo, emptyAction, Problem, Section, Setup, StepResult, Workflow } from '../model'
import { refsFor, workflowInputRefs } from '../refs'
import { ActionEditor } from './ActionEditor'
import { Bindings } from './Bindings'
import { StepList } from './StepList'

/** Edits a workflow: its local actions, setup, inputs, steps and teardown. */
export function WorkflowEditor(props: {
  workflow: Workflow
  onChange: (wf: Workflow) => void
  actions: Map<string, ActionInfo>
  servers: string[]
  /** Problems and results by section, then by step index. */
  problems: Map<Section, Map<number, Problem[]>>
  results?: Map<Section, Map<number, StepResult>>
}) {
  const { workflow: wf } = props
  const [openActions, setOpenActions] = useState(false)
  const set = (patch: Partial<Workflow>) => props.onChange({ ...wf, ...patch })
  const setSetup = (patch: Partial<Setup>) => wf.setup && set({ setup: { ...wf.setup, ...patch } })
  const inputs = wf.inputs.map((i) => i.name).filter(Boolean)
  const list = (section: Section) => ({
    servers: props.servers,
    problems: props.problems.get(section),
    results: props.results?.get(section),
  })

  return (
    <div className="workflow-editor">
      <details className="local-actions" open={openActions} onToggle={(e) => setOpenActions(e.currentTarget.open)}>
        <summary>
          このワークフロー内のアクション <span className="count">{wf.actions.length}</span>
        </summary>
        {wf.actions.map((a, i) => (
          <div className="local-action" key={i}>
            <div className="row">
              <input
                className="action-name"
                value={a.name}
                placeholder="local/name"
                spellCheck={false}
                onChange={(e) => set({ actions: wf.actions.map((x, j) => (j === i ? { ...x, name: e.target.value } : x)) })}
              />
              <input
                className="grow"
                value={a.description}
                placeholder="説明"
                onChange={(e) =>
                  set({ actions: wf.actions.map((x, j) => (j === i ? { ...x, description: e.target.value } : x)) })
                }
              />
              <button
                type="button"
                className="icon danger"
                title="削除"
                onClick={() => set({ actions: wf.actions.filter((_, j) => j !== i) })}
              >
                ×
              </button>
            </div>
            <ActionEditor
              action={a}
              onChange={(n) => set({ actions: wf.actions.map((x, j) => (j === i ? n : x)) })}
              actions={props.actions}
              servers={props.servers}
            />
          </div>
        ))}
        <button type="button" className="add" onClick={() => set({ actions: [...wf.actions, emptyAction()] })}>
          + アクションを定義
        </button>
      </details>

      <div className="field-block">
        <div className="field-title">
          setup <span className="muted">steps の前にデータを用意する</span>
          {wf.setup && (
            <button type="button" className="icon danger" title="setup を削除" onClick={() => set({ setup: null })}>
              ×
            </button>
          )}
        </div>
        {wf.setup ? (
          <>
            <StepList
              steps={wf.setup.steps}
              onChange={(steps) => setSetup({ steps })}
              scope={{ actions: props.actions }}
              {...list('setup')}
            />
            <div className="field-title">
              outputs <span className="muted">ワークフローの inputs に渡す値</span>
            </div>
            <Bindings
              items={wf.setup.outputs}
              onChange={(outputs) => setSetup({ outputs })}
              refs={refsFor({ steps: wf.setup.steps, actions: props.actions }, wf.setup.steps.length, 'actionOutputs')}
              addLabel="output"
            />
          </>
        ) : (
          <button type="button" className="add" onClick={() => set({ setup: { steps: [], outputs: [] } })}>
            + setup を追加
          </button>
        )}
      </div>

      <div className="field-block">
        <div className="field-title">
          inputs <span className="muted">steps と teardown のすべてのステップで使える値</span>
        </div>
        <Bindings
          items={wf.inputs}
          onChange={(inputs) => set({ inputs })}
          refs={workflowInputRefs((wf.setup?.outputs ?? []).map((o) => o.name).filter(Boolean))}
          addLabel="input"
        />
      </div>

      <div className="field-block">
        <div className="field-title">steps</div>
        <StepList
          steps={wf.steps}
          onChange={(steps) => set({ steps })}
          scope={{ actions: props.actions, inputs }}
          {...list('steps')}
        />
      </div>

      <div className="field-block">
        <div className="field-title">
          teardown <span className="muted">失敗しても最後に実行する後片付け</span>
        </div>
        <StepList
          steps={wf.teardown}
          onChange={(teardown) => set({ teardown })}
          scope={{ actions: props.actions, inputs, before: wf.steps }}
          {...list('teardown')}
        />
      </div>
    </div>
  )
}
