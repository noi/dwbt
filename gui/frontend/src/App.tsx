import { useCallback, useEffect, useMemo, useState } from 'react'
import { api, message } from './api'
import { ActionEditor } from './components/ActionEditor'
import { Popover } from './components/Popover'
import { statusClass } from './components/StepCard'
import { WorkflowEditor } from './components/WorkflowEditor'
import {
  Action,
  ActionInfo,
  emptyAction,
  emptyWorkflow,
  infoOf,
  Problem,
  Project,
  RunResult,
  Section,
  StepResult,
  Workflow,
} from './model'

type Kind = 'workflow' | 'action'

interface Target {
  kind: Kind
  /** A workflow file relative to .dwbt/workflows, or an action name. */
  name: string
}

/** An opened document, possibly with unsaved changes. */
interface Draft {
  target: Target
  doc: Workflow | Action
  /** The serialized document as last loaded or saved; "" if never saved. */
  saved: string
  result?: RunResult
}

const keyOf = (t: Target) => `${t.kind}:${t.name}`

/** Serializes a document without the client-side keys of the steps. */
const serialize = (doc: Workflow | Action) => JSON.stringify(doc, (k, v) => (k === '_key' ? undefined : v))

type Tab = 'result' | 'problems' | 'yaml'

export default function App() {
  const [project, setProject] = useState<Project | null>(null)
  const [loading, setLoading] = useState(true)
  const [drafts, setDrafts] = useState<Record<string, Draft>>({})
  const [current, setCurrent] = useState<string | null>(null)
  const [problems, setProblems] = useState<Problem[]>([])
  const [yaml, setYaml] = useState('')
  const [yamlError, setYamlError] = useState('')
  const [tab, setTab] = useState<Tab>('problems')
  const [error, setError] = useState('')
  const [running, setRunning] = useState(false)
  const [env, setEnv] = useState('')
  const [servers, setServers] = useState<Record<string, string>>({})
  const [creating, setCreating] = useState<Kind | null>(null)

  const draft = current ? drafts[current] : undefined

  const opened = (p: Project | null) => {
    if (!p) return
    setProject(p)
    setDrafts({})
    setCurrent(null)
    setEnv(p.defaultEnv)
    setServers({})
  }

  useEffect(() => {
    api
      .initial()
      .then(opened)
      .catch((e) => setError(message(e)))
      .finally(() => setLoading(false))
  }, [])

  const reload = useCallback(async () => {
    try {
      setProject(await api.project())
    } catch (e) {
      setError(message(e))
    }
  }, [])

  const open = async (t: Target) => {
    const key = keyOf(t)
    if (!drafts[key]) {
      try {
        const doc = t.kind === 'workflow' ? await api.workflow(t.name) : await api.action(t.name)
        setDrafts((d) => ({ ...d, [key]: { target: t, doc, saved: serialize(doc) } }))
      } catch (e) {
        setError(message(e))
        return
      }
    }
    setCurrent(key)
  }

  const create = (kind: Kind, name: string) => {
    name = name.trim()
    if (!name) return
    if (kind === 'workflow' && !name.endsWith('.yaml')) name += '.yaml'
    const t = { kind, name }
    const key = keyOf(t)
    const exists = kind === 'workflow' ? project?.workflows.includes(name) : project?.actions.some((a) => a.name === name)
    if (exists || drafts[key]) {
      setError(`${name} は既に存在します`)
      return
    }
    const doc = kind === 'workflow' ? emptyWorkflow() : emptyAction()
    setDrafts((d) => ({ ...d, [key]: { target: t, doc, saved: '' } }))
    setCurrent(key)
    setCreating(null)
  }

  const update = (doc: Workflow | Action) => {
    if (!current) return
    setDrafts((d) => ({ ...d, [current]: { ...d[current], doc } }))
  }

  const save = useCallback(async () => {
    if (!draft || !current) return
    const { target, doc } = draft
    try {
      if (target.kind === 'workflow') await api.saveWorkflow(target.name, doc as Workflow)
      else await api.saveAction(target.name, doc as Action)
      setDrafts((d) => ({ ...d, [current]: { ...d[current], saved: serialize(doc) } }))
      setError('')
      await reload()
    } catch (e) {
      setError(message(e))
    }
  }, [draft, current, reload])

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key === 's') {
        e.preventDefault()
        save()
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [save])

  // Validates and renders the edited document shortly after each change.
  useEffect(() => {
    if (!draft) {
      setProblems([])
      setYaml('')
      return
    }
    const { target, doc } = draft
    const timer = setTimeout(async () => {
      try {
        if (target.kind === 'workflow') {
          const [y, p] = await Promise.all([api.renderWorkflow(doc as Workflow), api.checkWorkflow(target.name, doc as Workflow)])
          setYaml(y)
          setProblems(p)
        } else {
          const [y, p] = await Promise.all([api.renderAction(doc as Action), api.checkAction(target.name, doc as Action)])
          setYaml(y)
          setProblems(p)
        }
        setYamlError('')
      } catch (e) {
        setYamlError(message(e))
        setProblems([{ message: message(e), file: '', line: 0, section: '', step: -1 }])
      }
    }, 250)
    return () => clearTimeout(timer)
  }, [draft?.doc, draft?.target.name])

  const run = async () => {
    if (!draft || draft.target.kind !== 'workflow' || !current) return
    const key = current
    setRunning(true)
    setTab('result')
    try {
      const overrides = Object.fromEntries(Object.entries(servers).filter(([, v]) => v.trim()))
      const result = await api.run(draft.target.name, draft.doc as Workflow, { env, servers: overrides })
      setDrafts((d) => ({ ...d, [key]: { ...d[key], result } }))
      if (result.problems.length > 0) setTab('problems')
    } catch (e) {
      setError(message(e))
    } finally {
      setRunning(false)
    }
  }

  // The actions callable from the edited document.
  const actions = useMemo(() => {
    const m = new Map<string, ActionInfo>()
    for (const a of project?.actions ?? []) m.set(a.name, a)
    if (draft?.target.kind === 'workflow') {
      for (const a of (draft.doc as Workflow).actions) if (a.name) m.set(a.name, infoOf(a, true))
    }
    return m
  }, [project, draft?.doc, draft?.target.kind])

  const problemsByStep = useMemo(() => {
    const m = new Map<Section, Map<number, Problem[]>>()
    for (const p of problems) {
      if (p.step < 0) continue
      const sec = m.get(p.section as Section) ?? new Map<number, Problem[]>()
      sec.set(p.step, [...(sec.get(p.step) ?? []), p])
      m.set(p.section as Section, sec)
    }
    return m
  }, [problems])

  const results = useMemo(() => {
    const m = new Map<Section, Map<number, StepResult>>()
    for (const s of draft?.result?.steps ?? []) {
      const sec = m.get(s.section) ?? new Map<number, StepResult>()
      sec.set(s.index, s)
      m.set(s.section, sec)
    }
    return m
  }, [draft?.result])

  if (loading) return <div className="center muted">読み込み中…</div>
  if (!project) {
    return (
      <div className="center welcome">
        <h1>dwbt</h1>
        <p className="muted">.dwbt ディレクトリを含むディレクトリを開いてください。</p>
        <button type="button" className="primary" onClick={() => api.choose().then(opened).catch((e) => setError(message(e)))}>
          プロジェクトを開く
        </button>
        {error && <div className="problem">{error}</div>}
      </div>
    )
  }

  const isDirty = (d: Draft) => d.saved !== serialize(d.doc)
  const fileActions = project.actions.filter((a) => !a.builtin)
  const draftList = Object.values(drafts)
  const unsaved = (kind: Kind) => draftList.filter((d) => d.target.kind === kind && d.saved === '').map((d) => d.target.name)

  return (
    <div className="app">
      <aside className="sidebar">
        <div className="project">
          <div className="project-root" title={project.root}>
            {project.root.split('/').pop()}
          </div>
          <button type="button" className="link" onClick={() => api.choose().then(opened).catch((e) => setError(message(e)))}>
            開く…
          </button>
        </div>
        {(['workflow', 'action'] as Kind[]).map((kind) => {
          const names =
            kind === 'workflow'
              ? [...project.workflows, ...unsaved('workflow')]
              : [...fileActions.map((a) => a.name), ...unsaved('action')]
          return (
            <nav key={kind}>
              <div className="nav-title">
                {kind === 'workflow' ? 'ワークフロー' : 'アクション'}
                <button type="button" className="icon" title="新規作成" onClick={() => setCreating(kind)}>
                  +
                </button>
              </div>
              {creating === kind && (
                <input
                  autoFocus
                  className="new-name"
                  placeholder={kind === 'workflow' ? 'name.yaml' : 'group/name'}
                  onBlur={() => setCreating(null)}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter') create(kind, e.currentTarget.value)
                    if (e.key === 'Escape') setCreating(null)
                  }}
                />
              )}
              {[...new Set(names)].map((name) => {
                const key = keyOf({ kind, name })
                const d = drafts[key]
                return (
                  <button
                    type="button"
                    key={name}
                    className={`nav-item${current === key ? ' active' : ''}`}
                    onClick={() => open({ kind, name })}
                  >
                    <span className="nav-name">{name}</span>
                    {d && isDirty(d) && <span className="dirty" title="未保存の変更があります" />}
                    {d?.result && <span className={`dot ${statusClass(d.result.status)}`} />}
                  </button>
                )
              })}
            </nav>
          )
        })}
        {project.warnings.map((w) => (
          <div className="warning" key={w}>
            {w}
          </div>
        ))}
      </aside>

      <main className="main">
        {draft ? (
          <>
            <header className="doc-header">
              <div className="doc-title">
                <span className="doc-kind">{draft.target.kind === 'workflow' ? 'workflow' : 'action'}</span>
                <span className="doc-name">{draft.target.name}</span>
                {isDirty(draft) && <span className="dirty" title="未保存の変更があります" />}
              </div>
              <span className="spacer" />
              {draft.target.kind === 'workflow' && (
                <RunControls
                  project={project}
                  env={env}
                  setEnv={setEnv}
                  servers={servers}
                  setServers={setServers}
                  running={running}
                  onRun={run}
                />
              )}
              <button type="button" className="primary" disabled={!isDirty(draft)} onClick={save} title="⌘S">
                保存
              </button>
            </header>
            {error && (
              <div className="banner" onClick={() => setError('')}>
                {error}
              </div>
            )}
            <div className="editor-scroll">
              <input
                className="description"
                value={draft.doc.description}
                placeholder="説明"
                onChange={(e) => update({ ...draft.doc, description: e.target.value })}
              />
              {draft.target.kind === 'workflow' ? (
                <WorkflowEditor
                  workflow={draft.doc as Workflow}
                  onChange={update}
                  actions={actions}
                  servers={project.servers}
                  problems={problemsByStep}
                  results={results}
                />
              ) : (
                <ActionEditor
                  action={draft.doc as Action}
                  onChange={update}
                  actions={actions}
                  servers={project.servers}
                  problems={problemsByStep.get('steps')}
                />
              )}
            </div>
          </>
        ) : (
          <div className="center muted">
            左のリストからワークフローかアクションを選ぶか、+ で新しく作成してください。
            {error && <div className="problem">{error}</div>}
          </div>
        )}
      </main>

      <aside className="panel">
        <div className="tabs">
          {(
            [
              ['problems', `問題${problems.length ? ` (${problems.length})` : ''}`],
              ['result', '実行結果'],
              ['yaml', 'YAML'],
            ] as [Tab, string][]
          ).map(([t, label]) => (
            <button type="button" key={t} className={tab === t ? 'active' : ''} onClick={() => setTab(t)}>
              {label}
            </button>
          ))}
        </div>
        <div className="panel-body">
          {tab === 'problems' && <Problems problems={problems} />}
          {tab === 'result' && <Result result={draft?.result} running={running} />}
          {tab === 'yaml' && (yamlError ? <div className="problem">{yamlError}</div> : <pre className="yaml">{yaml}</pre>)}
        </div>
      </aside>
    </div>
  )
}

function RunControls(props: {
  project: Project
  env: string
  setEnv: (e: string) => void
  servers: Record<string, string>
  setServers: (s: Record<string, string>) => void
  running: boolean
  onRun: () => void
}) {
  const { project } = props
  return (
    <div className="run-controls">
      {project.environments.length > 0 && (
        <select value={props.env} onChange={(e) => props.setEnv(e.target.value)} title="環境">
          {project.environments.map((e) => (
            <option key={e} value={e}>
              {e}
            </option>
          ))}
        </select>
      )}
      <Popover label="サーバー" className="secondary" align="right" title="サーバーの URL を上書き">
        {() => (
          <div className="servers">
            <div className="muted">サーバーの URL を上書き（空欄なら環境の設定を使用）</div>
            {project.servers.map((id) => (
              <label key={id} className="row">
                <span className="name">{id}</span>
                <input
                  value={props.servers[id] ?? ''}
                  placeholder="http://127.0.0.1:8080"
                  onChange={(e) => props.setServers({ ...props.servers, [id]: e.target.value })}
                />
              </label>
            ))}
            {project.servers.length === 0 && <div className="muted">config.yaml にサーバーがありません</div>}
          </div>
        )}
      </Popover>
      {props.running ? (
        <button type="button" className="danger" onClick={() => api.stop()}>
          停止
        </button>
      ) : (
        <button type="button" className="run" onClick={props.onRun}>
          ▶ 実行
        </button>
      )}
    </div>
  )
}

function Problems(props: { problems: Problem[] }) {
  if (props.problems.length === 0) return <div className="muted pad">問題はありません</div>
  return (
    <div className="problem-list">
      {props.problems.map((p, i) => (
        <div className="problem" key={i}>
          {p.step >= 0 && <span className="badge skip">#{p.step + 1}</span>} {p.message}
        </div>
      ))}
    </div>
  )
}

function Result(props: { result?: RunResult; running: boolean }) {
  if (props.running) return <div className="muted pad">実行中…</div>
  const r = props.result
  if (!r) return <div className="muted pad">まだ実行していません</div>
  if (!r.status) return <div className="problem">検証エラーのため実行できませんでした。問題タブを確認してください。</div>
  return (
    <div className="result">
      <div className={`result-summary ${statusClass(r.status)}`}>
        {r.status === 'ok' ? '成功' : r.status === 'FAIL' ? '失敗' : 'エラー'} <small>{r.duration}</small>
      </div>
      {r.error && <pre className="result-error">{r.error}</pre>}
      {r.steps.map((s) => (
        <div className="result-step" key={`${s.section}:${s.index}`}>
          <div className="row">
            <span className={`badge ${statusClass(s.status)}`}>{s.status}</span>
            <span className="grow">{s.section === 'steps' ? s.label : `${s.section}: ${s.label}`}</span>
            <small className="muted">{s.duration}</small>
          </div>
          {s.error && <pre className="result-error">{s.error}</pre>}
        </div>
      ))}
    </div>
  )
}
