import { memo, useCallback, useEffect, useRef, useState } from 'react'

type Props = { from: string; to: string }
type CapacityWeek = { start: string; end: string }
type PersonWeek = {
  weekStart: string
  allocatedHours: number
  capacityHours: number
  isOverallocated: boolean
}
type PersonCapacity = {
  id: number
  name: string
  weeklyHours: number
  weeks: PersonWeek[]
}
type CapacityResponse = {
  from: string
  to: string
  weeks: CapacityWeek[]
  people: PersonCapacity[]
}
type LoadState = 'loading' | 'ready' | 'refreshing' | 'error' | 'stale'
type SaveState = { kind: 'saving' | 'success' | 'error'; message: string }
type LoadResult = 'loaded' | 'failed' | 'superseded'

const shortDate = new Intl.DateTimeFormat('en', {
  month: 'short',
  day: 'numeric',
  timeZone: 'UTC',
})

function formatDate(value: string) {
  return shortDate.format(new Date(`${value}T00:00:00Z`))
}

function isOverallocated(person: PersonCapacity) {
  return person.weeks.some((week) => week.isOverallocated)
}

function formatHours(value: number) {
  return Number.isInteger(value) ? String(value) : value.toFixed(1)
}

async function responseError(response: Response, fallback: string) {
  try {
    const body = (await response.json()) as { error?: unknown }
    if (typeof body.error === 'string' && body.error.trim()) return body.error
  } catch {
    // The HTTP status remains useful if an upstream error is not JSON.
  }
  return `${fallback} (${response.status})`
}

export function CapacityGrid({ from, to }: Props) {
  const [data, setData] = useState<CapacityResponse | null>(null)
  const [loadState, setLoadState] = useState<LoadState>('loading')
  const [loadError, setLoadError] = useState('')
  const [nameQuery, setNameQuery] = useState('')
  const [overCapacityOnly, setOverCapacityOnly] = useState(false)
  const [drafts, setDrafts] = useState<Record<number, string>>({})
  const [saveStates, setSaveStates] = useState<Record<number, SaveState>>({})
  const requestVersion = useRef(0)
  const activeLoad = useRef<AbortController | null>(null)
  const dirtyPeople = useRef(new Set<number>())

  const loadCapacity = useCallback(async (keepCurrentData: boolean): Promise<LoadResult> => {
    const version = ++requestVersion.current
    activeLoad.current?.abort()
    const controller = new AbortController()
    activeLoad.current = controller
    setLoadState(keepCurrentData ? 'refreshing' : 'loading')
    setLoadError('')

    try {
      const params = new URLSearchParams({ from, to })
      const response = await fetch(`/api/capacity?${params}`, { signal: controller.signal })
      if (!response.ok) throw new Error(await responseError(response, 'Could not load capacity'))
      const nextData = (await response.json()) as CapacityResponse
      if (version !== requestVersion.current) return 'superseded'

      setData(nextData)
      setDrafts((current) => {
        const next: Record<number, string> = {}
        for (const person of nextData.people) {
          next[person.id] = dirtyPeople.current.has(person.id)
            ? (current[person.id] ?? formatHours(person.weeklyHours))
            : formatHours(person.weeklyHours)
        }
        return next
      })
      setLoadState('ready')
      return 'loaded'
    } catch (error) {
      if (controller.signal.aborted || version !== requestVersion.current) return 'superseded'
      setLoadError(error instanceof Error ? error.message : 'Could not load capacity')
      setLoadState(keepCurrentData ? 'stale' : 'error')
      return 'failed'
    }
  }, [from, to])

  useEffect(() => {
    void loadCapacity(false)
    return () => {
      requestVersion.current += 1
      activeLoad.current?.abort()
    }
  }, [loadCapacity])

  const updateDraft = useCallback((personId: number, value: string) => {
    dirtyPeople.current.add(personId)
    setDrafts((current) => ({ ...current, [personId]: value }))
    setSaveStates((current) => {
      const next = { ...current }
      delete next[personId]
      return next
    })
  }, [])

  const saveWeeklyHours = useCallback(async (person: PersonCapacity, draft: string) => {
    const rawValue = draft.trim()
    const weeklyHours = Number(rawValue)
    if (rawValue === '' || !Number.isFinite(weeklyHours) || weeklyHours < 0) {
      setSaveStates((current) => ({
        ...current,
        [person.id]: { kind: 'error', message: 'Enter zero or a positive number.' },
      }))
      return
    }

    setSaveStates((current) => ({
      ...current,
      [person.id]: { kind: 'saving', message: 'Saving…' },
    }))

    try {
      const response = await fetch(`/api/people/${person.id}`, {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ weeklyHours }),
      })
      if (!response.ok) throw new Error(await responseError(response, 'Could not save weekly hours'))

      dirtyPeople.current.delete(person.id)
      setSaveStates((current) => ({
        ...current,
        [person.id]: { kind: 'saving', message: 'Saved. Updating grid…' },
      }))
      const result = await loadCapacity(true)
      setSaveStates((current) => ({
        ...current,
        [person.id]: result === 'failed'
          ? { kind: 'error', message: 'Saved, but the grid may be stale. Retry above.' }
          : { kind: 'success', message: 'Saved' },
      }))
    } catch (error) {
      setSaveStates((current) => ({
        ...current,
        [person.id]: {
          kind: 'error',
          message: error instanceof Error ? error.message : 'Could not save weekly hours',
        },
      }))
    }
  }, [loadCapacity])

  if (loadState === 'loading') {
    return (
      <section className="grid-state" aria-live="polite" aria-busy="true">
        <span className="loader" aria-hidden="true" />
        <div><strong>Building the capacity ledger</strong><p>Loading every person and week…</p></div>
      </section>
    )
  }

  if (loadState === 'error' || !data) {
    return (
      <section className="grid-state grid-state--error" role="alert">
        <div><strong>Capacity data is unavailable</strong><p>{loadError}</p></div>
        <button className="button button--dark" type="button" onClick={() => void loadCapacity(false)}>Try again</button>
      </section>
    )
  }

  const overallocatedPeople = data.people.filter(isOverallocated).length

  // Filters are applied to the loaded range rather than refetched: the API
  // already returned every person, and narrowing in the browser keeps the
  // over-capacity count honest against the whole team.
  const needle = nameQuery.trim().toLocaleLowerCase()
  const isFiltered = needle !== '' || overCapacityOnly
  const visiblePeople = data.people.filter((person) => {
    if (overCapacityOnly && !isOverallocated(person)) return false
    return needle === '' || person.name.toLocaleLowerCase().includes(needle)
  })

  return (
    <section className="capacity" aria-labelledby="capacity-heading">
      <div className="capacity-toolbar">
        <div>
          <p className="eyebrow">Allocation ledger</p>
          <h2 id="capacity-heading">{data.people.length} people · {data.weeks.length} weeks</h2>
        </div>
        <p className="risk-summary" aria-label={`${overallocatedPeople} people over capacity`}>
          <span aria-hidden="true">{overallocatedPeople}</span><span>over capacity</span>
        </p>
      </div>

      <div className="filters">
        <label className="visually-hidden" htmlFor="name-filter">Filter people by name</label>
        <input
          id="name-filter"
          className="filter-input"
          type="search"
          value={nameQuery}
          onChange={(event) => setNameQuery(event.target.value)}
          placeholder="Filter by name"
          autoComplete="off"
        />
        <label className="filter-toggle">
          <input
            type="checkbox"
            checked={overCapacityOnly}
            onChange={(event) => setOverCapacityOnly(event.target.checked)}
          />
          <span>Over capacity only</span>
        </label>
        {isFiltered ? (
          <button
            className="button button--text"
            type="button"
            onClick={() => {
              setNameQuery('')
              setOverCapacityOnly(false)
            }}
          >
            Clear filters
          </button>
        ) : null}
        <p className="filter-count" role="status">
          {isFiltered
            ? `Showing ${visiblePeople.length} of ${data.people.length} people`
            : `Showing all ${data.people.length} people`}
        </p>
      </div>

      {loadState === 'stale' ? (
        <div className="notice notice--warning" role="alert">
          <span><strong>Showing the last loaded data.</strong> {loadError}</span>
          <button className="button button--text" type="button" onClick={() => void loadCapacity(true)}>Retry</button>
        </div>
      ) : null}

      <div className="table-frame" aria-busy={loadState === 'refreshing'}>
        {loadState === 'refreshing' ? <div className="refresh-line" aria-hidden="true" /> : null}
        <table>
          <caption>
            Weekly allocation and capacity for {formatDate(data.from)} through {formatDate(data.to)}. Edit weekly hours in each person’s row.
          </caption>
          <thead>
            <tr>
              <th className="person-column" scope="col">Person / weekly hours</th>
              {data.weeks.map((week) => (
                <th scope="col" key={week.start}>
                  <span className="week-kicker">Week of</span>
                  <span className="week-range">{formatDate(week.start)}–{formatDate(week.end)}</span>
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {visiblePeople.length === 0 ? (
              <tr>
                <td className="no-matches" colSpan={data.weeks.length + 1}>
                  No one matches these filters.
                </td>
              </tr>
            ) : null}
            {visiblePeople.map((person) => (
              <PersonRow
                key={person.id}
                person={person}
                draft={drafts[person.id] ?? ''}
                saveState={saveStates[person.id]}
                onDraftChange={updateDraft}
                onSave={saveWeeklyHours}
              />
            ))}
          </tbody>
        </table>
      </div>
    </section>
  )
}

type PersonRowProps = {
  person: PersonCapacity
  draft: string
  saveState: SaveState | undefined
  onDraftChange: (personId: number, value: string) => void
  onSave: (person: PersonCapacity, draft: string) => void
}

// Memoised deliberately. Drafts and save states live in one object each on the
// grid, so without this every keystroke re-renders all 500 rows — measured at
// ~70ms per key on the seeded roster, which reads as typing lag.
const PersonRow = memo(function PersonRow({
  person,
  draft,
  saveState,
  onDraftChange,
  onSave,
}: PersonRowProps) {
  const isSaving = saveState?.kind === 'saving'
  return (
    <tr>
      <th className="person-column" scope="row">
        <span className="person-name">{person.name}</span>
        <form
          className="hours-editor"
          onSubmit={(event) => {
            event.preventDefault()
            onSave(person, draft)
          }}
        >
          <label className="visually-hidden" htmlFor={`weekly-hours-${person.id}`}>Weekly hours for {person.name}</label>
          <div className="hours-control">
            <input
              id={`weekly-hours-${person.id}`}
              type="number"
              min="0"
              step="any"
              inputMode="decimal"
              value={draft}
              onChange={(event) => onDraftChange(person.id, event.target.value)}
              aria-invalid={saveState?.kind === 'error' ? 'true' : undefined}
              aria-describedby={saveState ? `save-state-${person.id}` : undefined}
              disabled={isSaving}
            />
            <span aria-hidden="true">h/wk</span>
          </div>
          <button className="save-button" type="submit" disabled={isSaving}>{isSaving ? 'Saving' : 'Save'}</button>
        </form>
        <span
          className={`save-state${saveState ? ` save-state--${saveState.kind}` : ''}`}
          id={`save-state-${person.id}`}
          role={saveState?.kind === 'error' ? 'alert' : 'status'}
          aria-live="polite"
        >{saveState?.message ?? ''}</span>
      </th>
      {person.weeks.map((week) => {
        const overBy = week.allocatedHours - week.capacityHours
        return (
          <td className={week.isOverallocated ? 'capacity-cell capacity-cell--over' : 'capacity-cell'} key={week.weekStart}>
            <span className="hours-ratio">
              <strong>{formatHours(week.allocatedHours)}</strong><span aria-hidden="true"> / </span>
              <span className="visually-hidden"> allocated of </span><span>{formatHours(week.capacityHours)}</span>
              <span className="visually-hidden"> hours capacity</span>
            </span>
            {week.isOverallocated
              ? <span className="over-label">Over by {formatHours(overBy)}h</span>
              : <span className="within-label">Within capacity</span>}
          </td>
        )
      })}
    </tr>
  )
})
