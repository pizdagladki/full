import { useNavigate } from 'react-router-dom';
import type { HillType } from './KothBattle';

// ---------------------------------------------------------------------------
// KotH hill-select screen — Play->King of the Hill lands here; each option
// routes onward to the neutral leaderboard scaffold for that hill.
// Declarative list drives both rendering and routing (mirrors ModeSelect.tsx's
// MODE_OPTIONS pattern) so adding a fourth hill later is a one-line change.
// ---------------------------------------------------------------------------

interface HillOption {
  testId: string;
  label: string;
  /** Performs the navigation for this option. */
  onSelect: (navigate: ReturnType<typeof useNavigate>) => void;
}

const HILL_OPTIONS: HillOption[] = [
  {
    testId: 'hill-daily',
    label: 'Daily',
    onSelect: (navigate) =>
      navigate('/koth/mountain', { state: { hillType: 'daily' satisfies HillType } }),
  },
  {
    testId: 'hill-monthly',
    label: 'Monthly',
    onSelect: (navigate) =>
      navigate('/koth/mountain', { state: { hillType: 'monthly' satisfies HillType } }),
  },
  {
    testId: 'hill-ranked',
    label: 'Ranked',
    onSelect: (navigate) => navigate('/koth/ranked'),
  },
];

export function KothHillSelect() {
  const navigate = useNavigate();

  return (
    <div data-testid="koth-hill-select-screen">
      <h1>King of the Hill</h1>
      <nav>
        {HILL_OPTIONS.map((option) => (
          <button
            key={option.testId}
            type="button"
            data-testid={option.testId}
            onClick={() => option.onSelect(navigate)}
          >
            {option.label}
          </button>
        ))}
      </nav>
    </div>
  );
}
