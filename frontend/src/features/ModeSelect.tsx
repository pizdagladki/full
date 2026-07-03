import { useNavigate } from 'react-router-dom';

// ---------------------------------------------------------------------------
// Mode definitions — declarative list drives both rendering and routing so
// adding a fifth mode later is a one-line change.
// ---------------------------------------------------------------------------

type GameMode = 'ranked' | 'unranked';

interface ModeOption {
  testId: string;
  label: string;
  /** Performs the navigation for this option. */
  onSelect: (navigate: ReturnType<typeof useNavigate>) => void;
}

const MODE_OPTIONS: ModeOption[] = [
  {
    testId: 'mode-ranked',
    label: 'Ranked',
    onSelect: (navigate) => navigate('/search', { state: { mode: 'ranked' satisfies GameMode } }),
  },
  {
    testId: 'mode-unranked',
    label: 'Unranked',
    onSelect: (navigate) => navigate('/search', { state: { mode: 'unranked' satisfies GameMode } }),
  },
  {
    testId: 'mode-invite',
    label: 'Invite a friend',
    onSelect: (navigate) => navigate('/invite'),
  },
  {
    testId: 'mode-koth',
    label: 'King of the Hill',
    onSelect: (navigate) => navigate('/koth'),
  },
];

// ---------------------------------------------------------------------------
// ModeSelect component — Play nav lands here; each option routes onward.
// ---------------------------------------------------------------------------

export function ModeSelect() {
  const navigate = useNavigate();

  return (
    <div data-testid="mode-select-screen">
      <h1>Choose a mode</h1>
      <nav>
        {MODE_OPTIONS.map((option) => (
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
