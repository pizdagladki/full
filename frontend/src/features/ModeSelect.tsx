import { useLocation, useNavigate } from 'react-router-dom';

// ---------------------------------------------------------------------------
// Mode definitions — declarative list drives both rendering and routing so
// adding a fifth mode later is a one-line change.
// ---------------------------------------------------------------------------

type GameMode = 'ranked' | 'unranked';

/** location.state carried in by Home's Play link (`navigate('/mode-select', { state: { trackId } })`). */
interface ModeSelectLocationState {
  trackId?: string;
}

interface ModeOption {
  testId: string;
  label: string;
  /** Performs the navigation for this option. `trackId` (#159) is threaded through to /search for
   * the ranked/unranked branches only — invite/koth don't carry a win-clip edit-audio selection. */
  onSelect: (navigate: ReturnType<typeof useNavigate>, trackId: string | undefined) => void;
}

const MODE_OPTIONS: ModeOption[] = [
  {
    testId: 'mode-ranked',
    label: 'Ranked',
    onSelect: (navigate, trackId) =>
      navigate('/search', { state: { mode: 'ranked' satisfies GameMode, trackId } }),
  },
  {
    testId: 'mode-unranked',
    label: 'Unranked',
    onSelect: (navigate, trackId) =>
      navigate('/search', { state: { mode: 'unranked' satisfies GameMode, trackId } }),
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
  const location = useLocation();
  const trackId = (location.state as ModeSelectLocationState | null)?.trackId;

  return (
    <div data-testid="mode-select-screen">
      <h1>Choose a mode</h1>
      <nav>
        {MODE_OPTIONS.map((option) => (
          <button
            key={option.testId}
            type="button"
            data-testid={option.testId}
            onClick={() => option.onSelect(navigate, trackId)}
          >
            {option.label}
          </button>
        ))}
      </nav>
    </div>
  );
}
