import { useSearchParams } from 'react-router-dom'
import { PageHead } from '../components/ui'
import FactsEditor from './settings/FactsEditor'
import SlotsManager from './settings/SlotsManager'
import SpendEditor from './settings/SpendEditor'
import TeamManager from './settings/TeamManager'
import './staff.css'

// The sales head's admin area: the fact sheet the assistant answers from, the
// visit slots buyers can pick, what each lead source costs, and the team.

const tabs = [
  { id: 'facts', label: 'Fact sheet' },
  { id: 'slots', label: 'Visit slots' },
  { id: 'spend', label: 'Ad spend' },
  { id: 'team', label: 'Team' },
] as const

type Tab = (typeof tabs)[number]['id']

export default function SettingsPage() {
  const [params, setParams] = useSearchParams()
  const tab: Tab = tabs.some((t) => t.id === params.get('tab')) ? (params.get('tab') as Tab) : 'facts'

  return (
    <section>
      <PageHead title="Settings" />
      <div className="tabs" role="tablist" aria-label="Settings">
        {tabs.map((t) => (
          <button
            key={t.id}
            role="tab"
            aria-selected={tab === t.id}
            onClick={() => setParams({ tab: t.id }, { replace: true })}
          >
            {t.label}
          </button>
        ))}
      </div>
      <div role="tabpanel">
        {tab === 'facts' && <FactsEditor />}
        {tab === 'slots' && <SlotsManager />}
        {tab === 'spend' && <SpendEditor />}
        {tab === 'team' && <TeamManager />}
      </div>
    </section>
  )
}
