import { act, render, screen, waitFor, fireEvent } from '@testing-library/react';
import { MemoryRouter, Routes, Route, useLocation } from 'react-router-dom';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { Home } from './Home';
import { AuthContext } from './auth/AuthContext';
import type { AuthState } from './auth/AuthContext';
import type { RatingsApi, RatingData } from '../api/ratings';
import type { FaceLandmarkResult, LandmarkRunner } from '../cv';

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

/** A fully-authenticated AuthState for wrapping the component under test. */
const AUTH_STATE: AuthState = {
  user: { id: 'user-42', email: 'test@example.com' },
  loading: false,
  error: null,
  refreshUser: vi.fn().mockResolvedValue(undefined),
};

/** Renders <Home /> inside the auth context and a MemoryRouter. */
function renderHome(
  authState: AuthState = AUTH_STATE,
  ratingsApi?: RatingsApi,
  cvRunner?: LandmarkRunner,
) {
  return render(
    <AuthContext.Provider value={authState}>
      <MemoryRouter>
        <Home ratingsApi={ratingsApi} cvRunner={cvRunner} />
      </MemoryRouter>
    </AuthContext.Provider>,
  );
}

/** Build a mock RatingsApi that resolves to the given data. */
function makeRatingsApi(data: RatingData): RatingsApi {
  return { getRating: vi.fn().mockResolvedValue(data) };
}

/** A mock RatingsApi that always rejects. */
function makeRatingsApiError(message = 'fetch failed'): RatingsApi {
  return { getRating: vi.fn().mockRejectedValue(new Error(message)) };
}

// ---------------------------------------------------------------------------
// Mock navigator.mediaDevices before each test
// ---------------------------------------------------------------------------

const TWO_CAMERAS: MediaDeviceInfo[] = [
  {
    kind: 'videoinput',
    deviceId: 'cam1',
    label: 'Front Cam',
    groupId: '',
    toJSON: () => ({}),
  },
  {
    kind: 'videoinput',
    deviceId: 'cam2',
    label: 'Back Cam',
    groupId: '',
    toJSON: () => ({}),
  },
];

function setupMediaDevices(devices: MediaDeviceInfo[] = TWO_CAMERAS) {
  Object.defineProperty(globalThis.navigator, 'mediaDevices', {
    value: {
      enumerateDevices: vi.fn().mockResolvedValue(devices),
      getUserMedia: vi.fn().mockResolvedValue({ getTracks: () => [] }),
    },
    writable: true,
    configurable: true,
  });
}

// ---------------------------------------------------------------------------
// RAF stub — same pattern as Search.test.tsx: collect scheduled callbacks and
// tick them manually so CvEngine frames are driven deterministically.
// ---------------------------------------------------------------------------

let rafCallbacks: FrameRequestCallback[] = [];

beforeEach(() => {
  setupMediaDevices();

  rafCallbacks = [];
  vi.stubGlobal(
    'requestAnimationFrame',
    vi.fn((cb: FrameRequestCallback) => {
      rafCallbacks.push(cb);
      return rafCallbacks.length;
    }),
  );
  vi.stubGlobal('cancelAnimationFrame', vi.fn());
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

function makeCvRunner(): {
  runner: LandmarkRunner;
  setResult: (r: FaceLandmarkResult) => void;
} {
  let nextResult: FaceLandmarkResult = { faceLandmarks: [] };
  const runner: LandmarkRunner = {
    detectForVideo: vi.fn(() => nextResult),
  };
  return {
    runner,
    setResult: (r: FaceLandmarkResult) => {
      nextResult = r;
    },
  };
}

const FACE_FRAME: FaceLandmarkResult = { faceLandmarks: [[{ x: 0, y: 0, z: 0 }]] };
const NO_FACE_FRAME: FaceLandmarkResult = { faceLandmarks: [] };

/** Sets the next detection result, then ticks the latest pending RAF callback. */
function tickFrame(
  setResult: (r: FaceLandmarkResult) => void,
  result: FaceLandmarkResult,
  ts = 0,
): void {
  setResult(result);
  const cb = rafCallbacks[rafCallbacks.length - 1];
  act(() => {
    cb(ts);
  });
}

// ---------------------------------------------------------------------------
// Criterion 1 — camera device listing and selection
// ---------------------------------------------------------------------------

describe('Criterion 1 — camera device listing/selection', () => {
  it('criterion-1: lists video input devices in the <select> from enumerateDevices', async () => {
    // criterion: 1 — the camera <select> must show all videoinput devices returned by enumerateDevices
    renderHome();

    // Wait for the async enumerateDevices call to resolve
    await waitFor(() => {
      const select = screen.getByLabelText('Camera') as HTMLSelectElement;
      expect(select.options).toHaveLength(2);
    });

    const select = screen.getByLabelText('Camera') as HTMLSelectElement;
    expect(select.options[0].value).toBe('cam1');
    expect(select.options[0].text).toBe('Front Cam');
    expect(select.options[1].value).toBe('cam2');
    expect(select.options[1].text).toBe('Back Cam');
  });

  it('criterion-1 violation guard: with NO cameras the <select> shows NO options', async () => {
    // criterion: 1 — if enumerateDevices returns no video inputs, the select is empty (no phantom options)
    setupMediaDevices([]);
    renderHome();

    await waitFor(() => {
      const select = screen.getByLabelText('Camera') as HTMLSelectElement;
      expect(select.options).toHaveLength(0);
    });
  });

  it('criterion-1: changing selection updates the selected device id in state', async () => {
    // criterion: 1 — user can pick a different camera; the <select> reflects the chosen value
    renderHome();

    await waitFor(() => {
      expect((screen.getByLabelText('Camera') as HTMLSelectElement).options).toHaveLength(2);
    });

    const select = screen.getByLabelText('Camera') as HTMLSelectElement;
    fireEvent.change(select, { target: { value: 'cam2' } });

    expect(select.value).toBe('cam2');
  });

  it('criterion-1 violation guard: first device is pre-selected by default', async () => {
    // criterion: 1 — after enumeration the first device is automatically selected
    renderHome();

    await waitFor(() => {
      const select = screen.getByLabelText('Camera') as HTMLSelectElement;
      expect(select.value).toBe('cam1');
    });
  });
});

// ---------------------------------------------------------------------------
// Criterion 2 — TikTok track selector
// ---------------------------------------------------------------------------

describe('Criterion 2 — TikTok track selector', () => {
  it('criterion-2: renders a track selector with at least 2 options', async () => {
    // criterion: 2 — the placeholder track list must have multiple entries
    renderHome();

    const select = screen.getByLabelText('Track') as HTMLSelectElement;
    expect(select.options.length).toBeGreaterThanOrEqual(2);
  });

  it('criterion-2 violation guard: track selector with only 1 option would fail the ≥2 check', async () => {
    // criterion: 2 guard — baseline that the test above actually discriminates
    renderHome();

    const select = screen.getByLabelText('Track') as HTMLSelectElement;
    expect(select.options.length).not.toBe(1);
  });

  it('criterion-2: selecting a different track updates state', async () => {
    // criterion: 2 — user can pick a track; the <select> reflects the new value
    renderHome();

    const select = screen.getByLabelText('Track') as HTMLSelectElement;
    const secondOption = select.options[1].value;
    fireEvent.change(select, { target: { value: secondOption } });

    expect(select.value).toBe(secondOption);
  });
});

// ---------------------------------------------------------------------------
// Criterion 3 — level progress bar from rating
// ---------------------------------------------------------------------------

describe('Criterion 3 — level progress bar', () => {
  it('criterion-3: renders a progressbar when getRating resolves', async () => {
    // criterion: 3 — a [role=progressbar] must appear after the rating is fetched
    const api = makeRatingsApi({ elo: 1200, level: 5, games_played: 30 });
    renderHome(AUTH_STATE, api);

    await waitFor(() => {
      expect(screen.getByRole('progressbar')).toBeInTheDocument();
    });
  });

  it('criterion-3 violation guard: without a resolved rating no progressbar exists', async () => {
    // criterion: 3 — while loading there is no progressbar (loading placeholder is shown instead)
    // We make the promise never resolve to keep it in loading state
    const api: RatingsApi = { getRating: vi.fn().mockReturnValue(new Promise(() => {})) };
    renderHome(AUTH_STATE, api);

    // Should NOT have a progressbar yet
    expect(screen.queryByRole('progressbar')).not.toBeInTheDocument();
    // Should show loading placeholder
    expect(screen.getByTestId('level-placeholder')).toBeInTheDocument();
  });

  it('criterion-3: progressbar aria-valuenow reflects level/10 * 100', async () => {
    // criterion: 3 — the progress bar value equals (level / 10) * 100
    const api = makeRatingsApi({ elo: 1500, level: 3, games_played: 10 });
    renderHome(AUTH_STATE, api);

    await waitFor(() => {
      const bar = screen.getByRole('progressbar');
      expect(bar.getAttribute('aria-valuenow')).toBe('30');
    });
  });

  it('criterion-3: shows neutral placeholder on API error, does not crash', async () => {
    // criterion: 3 — on rating fetch error the component renders a placeholder and does not throw
    const api = makeRatingsApiError('Network Error');
    renderHome(AUTH_STATE, api);

    await waitFor(() => {
      expect(screen.getByTestId('level-placeholder')).toBeInTheDocument();
    });

    expect(screen.queryByRole('progressbar')).not.toBeInTheDocument();
  });

  it('criterion-3: shows neutral placeholder when user is null (no user id to fetch)', async () => {
    // criterion: 3 — null user should not crash the component; placeholder is shown
    const nullUserState: AuthState = { user: null, loading: false, error: null, refreshUser: vi.fn().mockResolvedValue(undefined) };
    renderHome(nullUserState);

    // No progressbar, placeholder visible
    expect(screen.queryByRole('progressbar')).not.toBeInTheDocument();
    expect(screen.getByTestId('level-placeholder')).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Criterion 4 — navigation controls + AdSense banner slots
// ---------------------------------------------------------------------------

describe('Criterion 4 — navigation and banner slots', () => {
  it('criterion-4: renders a link to /store', () => {
    // criterion: 4 — nav must have a link to the store
    renderHome();

    const storeLink = screen.getByRole('link', { name: /store/i }) as HTMLAnchorElement;
    expect(storeLink).toBeInTheDocument();
    expect(storeLink.getAttribute('href')).toBe('/store');
  });

  it('criterion-4: renders a link to /profile', () => {
    // criterion: 4 — nav must have a link to the user's profile
    renderHome();

    const profileLink = screen.getByRole('link', { name: /profile/i }) as HTMLAnchorElement;
    expect(profileLink).toBeInTheDocument();
    expect(profileLink.getAttribute('href')).toBe('/profile');
  });

  it('criterion-4: renders a link to /mode-select (mode-select entry)', () => {
    // criterion: 4 — nav must have a link to mode-select (/mode-select)
    renderHome();

    const playLink = screen.getByRole('link', { name: /play/i }) as HTMLAnchorElement;
    expect(playLink).toBeInTheDocument();
    expect(playLink.getAttribute('href')).toBe('/mode-select');
  });

  it('criterion-4 violation guard: missing store link would fail', () => {
    // criterion: 4 guard — confirms the assertion actually detects absence
    renderHome();
    expect(screen.queryByRole('link', { name: /store/i })).not.toBeNull();
  });

  // criterion: 4 (#159) — the Play link carries the selected track id in location.state so it can
  // be threaded through ModeSelect → Search → Battle as the win-clip edit audio.
  it('criterion-4/#159: the Play link carries the selected trackId in location.state', () => {
    function ModeSelectProbe() {
      const location = useLocation();
      const state = location.state as { trackId?: string } | null;
      return <div data-testid="mode-select-probe">{state?.trackId}</div>;
    }

    render(
      <AuthContext.Provider value={AUTH_STATE}>
        <MemoryRouter initialEntries={['/home']}>
          <Routes>
            <Route path="/home" element={<Home />} />
            <Route path="/mode-select" element={<ModeSelectProbe />} />
          </Routes>
        </MemoryRouter>
      </AuthContext.Provider>,
    );

    fireEvent.click(screen.getByRole('link', { name: /play/i }));

    expect(screen.getByTestId('mode-select-probe').textContent).toBe('track-1');
  });

  // criterion: 4 (#159) violation guard — selecting a DIFFERENT track before clicking Play must
  // carry THAT track, not always the default; a hard-coded trackId would fail this.
  it('criterion-4/#159 violation guard: a newly-selected track (not the default) is carried through', () => {
    function ModeSelectProbe() {
      const location = useLocation();
      const state = location.state as { trackId?: string } | null;
      return <div data-testid="mode-select-probe">{state?.trackId}</div>;
    }

    render(
      <AuthContext.Provider value={AUTH_STATE}>
        <MemoryRouter initialEntries={['/home']}>
          <Routes>
            <Route path="/home" element={<Home />} />
            <Route path="/mode-select" element={<ModeSelectProbe />} />
          </Routes>
        </MemoryRouter>
      </AuthContext.Provider>,
    );

    const trackSelect = screen.getByLabelText('Track') as HTMLSelectElement;
    fireEvent.change(trackSelect, { target: { value: 'track-2' } });
    fireEvent.click(screen.getByRole('link', { name: /play/i }));

    expect(screen.getByTestId('mode-select-probe').textContent).toBe('track-2');
  });

  it('criterion-4: at least one ad-slot placeholder is present', () => {
    // criterion: 4 — AdSense banner slot elements must be on the home screen
    renderHome();

    const slots = screen.getAllByTestId('ad-slot');
    expect(slots.length).toBeGreaterThanOrEqual(1);
  });

  it('criterion-4 violation guard: two ad-slot elements present (top + bottom)', () => {
    // criterion: 4 — the implementation places one banner at the top and one at the bottom
    renderHome();

    const slots = screen.getAllByTestId('ad-slot');
    expect(slots.length).toBe(2);
  });
});

// ---------------------------------------------------------------------------
// Criterion 5 — camera preview
// ---------------------------------------------------------------------------

describe('Criterion 5 — camera preview', () => {
  it('criterion-5: renders a <video> element for camera preview', () => {
    // criterion: 5 — a <video> element must be present in the home screen
    renderHome();

    const video = screen.getByTestId('camera-preview') as HTMLVideoElement;
    expect(video).toBeInTheDocument();
    expect(video.tagName.toLowerCase()).toBe('video');
  });

  it('criterion-5: video element has autoPlay and muted attributes', () => {
    // criterion: 5 — the preview must be muted and auto-play (no user gesture required)
    renderHome();

    const video = screen.getByTestId('camera-preview') as HTMLVideoElement;
    expect(video.autoplay).toBe(true);
    expect(video.muted).toBe(true);
  });

  it('criterion-5: calibration-status placeholder is rendered', () => {
    // criterion: 5 — a calibration placeholder must be shown next to the camera preview
    renderHome();

    expect(screen.getByTestId('calibration-status')).toBeInTheDocument();
  });

  it('criterion-5 violation guard: without a video element criterion 5 fails', () => {
    // criterion: 5 guard — sanity check that the video element is really there
    renderHome();
    expect(screen.queryByTestId('camera-preview')).not.toBeNull();
  });

  it('criterion-5: getUserMedia is called on mount with video constraint', async () => {
    // criterion: 5 — getUserMedia must be invoked to acquire the camera stream
    renderHome();

    await waitFor(() => {
      expect(navigator.mediaDevices.getUserMedia).toHaveBeenCalledWith(
        expect.objectContaining({ video: expect.anything() }),
      );
    });
  });
});

// ---------------------------------------------------------------------------
// Issue #158 — invisible auto-calibration CV engine wired onto the preview
// ---------------------------------------------------------------------------

/** Waits until the camera <select> has settled on 'cam1' (enumerateDevices resolved and picked
 * the first device) and the corresponding getUserMedia call has resolved and started the CV
 * engine on the preview (its .then() ran, including cvRef.current.start(...)). */
async function waitForEngineStarted(): Promise<void> {
  await waitFor(() => {
    const select = screen.getByLabelText('Camera') as HTMLSelectElement;
    expect(select.value).toBe('cam1');
  });
  await waitFor(() => {
    expect(navigator.mediaDevices.getUserMedia).toHaveBeenCalledWith(
      expect.objectContaining({ video: { deviceId: { exact: 'cam1' } } }),
    );
  });
  // Flush the resolved getUserMedia promise's .then() (sets status + calls cvRef.start()).
  await act(async () => {});
}

describe('Issue #158 — CV engine auto-calibration on the camera preview', () => {
  it('criterion-1/3: engine is started against the camera-preview video element', async () => {
    const { runner, setResult } = makeCvRunner();
    renderHome(AUTH_STATE, undefined, runner);

    await waitForEngineStarted();
    tickFrame(setResult, FACE_FRAME);

    const previewVideo = screen.getByTestId('camera-preview');
    expect(runner.detectForVideo).toHaveBeenCalled();
    expect(vi.mocked(runner.detectForVideo).mock.calls[0][0]).toBe(previewVideo);
  });

  it('criterion-2/3: initial status is "Calibrating…" and flips to ready once the engine reports a face', async () => {
    const { runner, setResult } = makeCvRunner();
    renderHome(AUTH_STATE, undefined, runner);

    const status = screen.getByTestId('calibration-status');
    expect(status.textContent).toBe('Calibrating…');
    expect(status.getAttribute('data-status')).toBe('calibrating');

    await waitForEngineStarted();
    tickFrame(setResult, FACE_FRAME);

    expect(status.getAttribute('data-status')).toBe('ready');
    expect(status.textContent).not.toBe('Calibrating…');
  });

  it('criterion-2 violation guard: with only no-face frames the status never flips to ready', async () => {
    const { runner, setResult } = makeCvRunner();
    renderHome(AUTH_STATE, undefined, runner);

    await waitForEngineStarted();
    tickFrame(setResult, NO_FACE_FRAME);
    tickFrame(setResult, NO_FACE_FRAME);
    tickFrame(setResult, NO_FACE_FRAME);

    const status = screen.getByTestId('calibration-status');
    expect(status.getAttribute('data-status')).toBe('calibrating');
    expect(status.textContent).toBe('Calibrating…');
  });

  it('criterion-2: losing the face after being ready reverts the status to calibrating', async () => {
    const { runner, setResult } = makeCvRunner();
    renderHome(AUTH_STATE, undefined, runner);

    await waitForEngineStarted();
    tickFrame(setResult, FACE_FRAME);

    const status = screen.getByTestId('calibration-status');
    expect(status.getAttribute('data-status')).toBe('ready');

    // NO_FACE_WINDOW = 3 consecutive no-face frames trigger onFaceLost
    tickFrame(setResult, NO_FACE_FRAME);
    tickFrame(setResult, NO_FACE_FRAME);
    tickFrame(setResult, NO_FACE_FRAME);

    expect(status.getAttribute('data-status')).toBe('calibrating');
    expect(status.textContent).toBe('Calibrating…');
  });

  it('criterion-1: stops the CV engine when the selected camera changes', async () => {
    const { runner } = makeCvRunner();
    renderHome(AUTH_STATE, undefined, runner);

    await waitForEngineStarted();
    // The engine actually scheduled a frame — confirms it was running before the camera change.
    expect(rafCallbacks.length).toBeGreaterThan(0);

    // Snapshot BEFORE the camera change — the initial ''→'cam1' device-selection settle already
    // triggered its own cleanup/cancelAnimationFrame call, so asserting plain "was called" would
    // pass even if the camera-change cleanup were removed. Asserting a STRICT increase from this
    // snapshot is what actually discriminates the camera-change stop.
    const before = vi.mocked(cancelAnimationFrame).mock.calls.length;

    const select = screen.getByLabelText('Camera') as HTMLSelectElement;
    fireEvent.change(select, { target: { value: 'cam2' } });

    // Changing the selected camera re-runs the camera-preview effect; its cleanup stops the
    // previous engine run, which cancels the scheduled RAF frame — a NEW cancelAnimationFrame
    // call beyond the pre-change count.
    expect(vi.mocked(cancelAnimationFrame).mock.calls.length).toBeGreaterThan(before);
  });

  // criterion: 1c — stopping on UNMOUNT (distinct from stopping on camera change above): Home's
  // camera-preview effect cleanup calls `cv?.stop()` on unmount. Snapshot-then-strict-increase
  // discriminates removing that cleanup (CvComponent's own unmount safety-net stop is a second,
  // independent path, but this test is written against Home's own contract: unmounting stops the
  // engine it started).
  it('criterion-1c: stops the CV engine on unmount', async () => {
    const { runner } = makeCvRunner();
    const { unmount } = renderHome(AUTH_STATE, undefined, runner);

    await waitForEngineStarted();
    // The engine actually scheduled a frame — confirms it was running before unmount.
    expect(rafCallbacks.length).toBeGreaterThan(0);

    const before = vi.mocked(cancelAnimationFrame).mock.calls.length;

    unmount();

    expect(vi.mocked(cancelAnimationFrame).mock.calls.length).toBeGreaterThan(before);
  });

  // criterion: 2 — no blink side effects: Home wires ONLY {onFacePresent, onFaceLost} into
  // CvCallbacks (no onBlink). This drives a GENUINE blink through the real CvEngine (not a
  // synthetic call): FACE_FRAME's single-point landmarks make computeEAR degenerate to 0 for
  // every eye (the eye-corner indices are out of bounds for a 1-point landmark array), so once
  // calibration completes (CALIBRATION_FRAMES = 30) the EAR is already below the default blink
  // threshold; two more frames in the 'running' state accumulate BLINK_FRAMES (=2) consecutive
  // below-threshold samples and the engine fires a real onBlink() internally. If Home ever wired
  // onBlink to a status-changing side effect, this assertion would catch it — the status must
  // stay exactly 'ready' across the blink.
  it('criterion-2: a genuine blink does not change the calibration status (no blink side effects on Home)', async () => {
    const { runner, setResult } = makeCvRunner();
    renderHome(AUTH_STATE, undefined, runner);

    await waitForEngineStarted();

    const status = screen.getByTestId('calibration-status');

    // Drive calibration to completion (30 frames) — status flips to 'ready' as soon as a face is
    // first detected (frame 1), well before calibration finishes.
    for (let i = 0; i < 30; i++) {
      tickFrame(setResult, FACE_FRAME);
    }
    expect(status.getAttribute('data-status')).toBe('ready');

    // Two more frames in 'running' state — this is where the real onBlink() fires internally.
    tickFrame(setResult, FACE_FRAME);
    tickFrame(setResult, FACE_FRAME);

    expect(status.getAttribute('data-status')).toBe('ready');
    expect(status.textContent).toBe('Face detected — ready');
  });
});
