import { act, render, screen, waitFor, fireEvent } from '@testing-library/react';
import { MemoryRouter, Routes, Route, useLocation } from 'react-router-dom';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { ModeSelect } from './ModeSelect';
import type { FaceLandmarkResult, LandmarkRunner } from '../cv';

// ---------------------------------------------------------------------------
// Probe screens — capture the navigate target AND the location.state it carries.
// ---------------------------------------------------------------------------

function SearchProbe() {
  const location = useLocation();
  const state = location.state as { mode?: string; trackId?: string } | null;
  return (
    <div data-testid="search-probe">
      <span data-testid="search-mode">{state?.mode}</span>
      <span data-testid="search-track-id">{state?.trackId}</span>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Shared helpers (ported from the old Home.test.tsx together with the camera
// preview / auto-calibration blocks that moved into this screen).
// ---------------------------------------------------------------------------

const TWO_CAMERAS: MediaDeviceInfo[] = [
  { kind: 'videoinput', deviceId: 'cam1', label: 'Front Cam', groupId: '', toJSON: () => ({}) },
  { kind: 'videoinput', deviceId: 'cam2', label: 'Back Cam', groupId: '', toJSON: () => ({}) },
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

function makeCvRunner(): { runner: LandmarkRunner; setResult: (r: FaceLandmarkResult) => void } {
  let nextResult: FaceLandmarkResult = { faceLandmarks: [] };
  const runner: LandmarkRunner = { detectForVideo: vi.fn(() => nextResult) };
  return {
    runner,
    setResult: (r: FaceLandmarkResult) => {
      nextResult = r;
    },
  };
}

const FACE_FRAME: FaceLandmarkResult = { faceLandmarks: [[{ x: 0, y: 0, z: 0 }]] };
const NO_FACE_FRAME: FaceLandmarkResult = { faceLandmarks: [] };

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

function renderModeSelect(cvRunner?: LandmarkRunner) {
  return render(
    <MemoryRouter initialEntries={['/mode-select']}>
      <Routes>
        <Route
          path="/mode-select"
          element={<ModeSelect cvRunner={cvRunner ?? makeCvRunner().runner} />}
        />
        <Route path="/search" element={<SearchProbe />} />
      </Routes>
    </MemoryRouter>,
  );
}

// ---------------------------------------------------------------------------
// Mode buttons — the screen now hosts ONLY the online-battle branches; koth and
// invite are reached directly from Home's mode windows.
// ---------------------------------------------------------------------------

describe('ModeSelect — «Онлайн-батлы»', () => {
  it('renders exactly the ranked and unranked options (koth/invite moved to Home)', () => {
    renderModeSelect();
    expect(screen.getByTestId('mode-select-screen')).toBeInTheDocument();
    expect(screen.getByTestId('mode-ranked')).toBeInTheDocument();
    expect(screen.getByTestId('mode-unranked')).toBeInTheDocument();
    expect(screen.queryByTestId('mode-invite')).not.toBeInTheDocument();
    expect(screen.queryByTestId('mode-koth')).not.toBeInTheDocument();
  });

  it('ranked navigates to /search with mode=ranked and the default trackId', async () => {
    renderModeSelect();
    fireEvent.click(screen.getByTestId('mode-ranked'));
    expect(await screen.findByTestId('search-probe')).toBeInTheDocument();
    expect(screen.getByTestId('search-mode')).toHaveTextContent('ranked');
    expect(screen.getByTestId('search-track-id')).toHaveTextContent('track-1');
  });

  it('unranked navigates to /search with mode=unranked', async () => {
    renderModeSelect();
    fireEvent.click(screen.getByTestId('mode-unranked'));
    expect(await screen.findByTestId('search-probe')).toBeInTheDocument();
    expect(screen.getByTestId('search-mode')).toHaveTextContent('unranked');
  });

  it('#159: a newly-selected track is carried through to /search', async () => {
    renderModeSelect();
    fireEvent.change(screen.getByLabelText(/Трек/), { target: { value: 'track-2' } });
    fireEvent.click(screen.getByTestId('mode-ranked'));
    expect(await screen.findByTestId('search-probe')).toBeInTheDocument();
    expect(screen.getByTestId('search-track-id')).toHaveTextContent('track-2');
  });
});

// ---------------------------------------------------------------------------
// Camera device listing / selection (moved from Home)
// ---------------------------------------------------------------------------

describe('ModeSelect — camera selection', () => {
  it('lists video input devices in the <select> from enumerateDevices', async () => {
    renderModeSelect();
    await waitFor(() => {
      expect(screen.getByText('Front Cam')).toBeInTheDocument();
    });
    expect(screen.getByText('Back Cam')).toBeInTheDocument();
  });

  it('pre-selects the first device by default', async () => {
    renderModeSelect();
    await waitFor(() => {
      expect((screen.getByLabelText('Камера') as HTMLSelectElement).value).toBe('cam1');
    });
  });

  it('changing the selection re-acquires the stream and persists the device (#172)', async () => {
    const setItem = vi.fn();
    vi.stubGlobal('localStorage', { getItem: vi.fn().mockReturnValue(null), setItem });
    renderModeSelect();
    await waitFor(() => {
      expect(screen.getByText('Back Cam')).toBeInTheDocument();
    });
    fireEvent.change(screen.getByLabelText('Камера'), { target: { value: 'cam2' } });
    await waitFor(() => {
      expect(navigator.mediaDevices.getUserMedia).toHaveBeenCalledWith({
        video: { deviceId: { exact: 'cam2' } },
      });
    });
    expect(setItem).toHaveBeenCalledWith('cameraDeviceId', 'cam2');
  });

  it('renders the camera preview <video> and calls getUserMedia on mount', async () => {
    renderModeSelect();
    const video = screen.getByTestId('camera-preview');
    expect(video).toBeInTheDocument();
    expect(video).toHaveAttribute('autoplay');
    await waitFor(() => {
      expect(navigator.mediaDevices.getUserMedia).toHaveBeenCalled();
    });
  });
});

// ---------------------------------------------------------------------------
// Invisible auto-calibration on the preview (#158 behavior, moved from Home)
// ---------------------------------------------------------------------------

describe('ModeSelect — CV auto-calibration on the preview', () => {
  it('starts the engine against the preview video element', async () => {
    const { runner } = makeCvRunner();
    renderModeSelect(runner);
    await waitFor(() => {
      expect(rafCallbacks.length).toBeGreaterThan(0);
    });
    act(() => {
      rafCallbacks[rafCallbacks.length - 1](0);
    });
    const previewVideo = screen.getByTestId('camera-preview');
    expect(runner.detectForVideo).toHaveBeenCalled();
    expect(vi.mocked(runner.detectForVideo).mock.calls[0][0]).toBe(previewVideo);
  });

  it('status starts as calibrating and flips to ready once a face is reported', async () => {
    const { runner, setResult } = makeCvRunner();
    renderModeSelect(runner);
    expect(screen.getByTestId('calibration-status')).toHaveAttribute('data-status', 'calibrating');
    await waitFor(() => {
      expect(rafCallbacks.length).toBeGreaterThan(0);
    });
    tickFrame(setResult, FACE_FRAME);
    await waitFor(() => {
      expect(screen.getByTestId('calibration-status')).toHaveAttribute('data-status', 'ready');
    });
  });

  it('with only no-face frames the status never flips to ready', async () => {
    const { runner, setResult } = makeCvRunner();
    renderModeSelect(runner);
    await waitFor(() => {
      expect(rafCallbacks.length).toBeGreaterThan(0);
    });
    tickFrame(setResult, NO_FACE_FRAME);
    tickFrame(setResult, NO_FACE_FRAME, 33);
    expect(screen.getByTestId('calibration-status')).toHaveAttribute('data-status', 'calibrating');
  });

  it('losing the face after being ready reverts the status to calibrating', async () => {
    const { runner, setResult } = makeCvRunner();
    renderModeSelect(runner);
    await waitFor(() => {
      expect(rafCallbacks.length).toBeGreaterThan(0);
    });
    tickFrame(setResult, FACE_FRAME);
    await waitFor(() => {
      expect(screen.getByTestId('calibration-status')).toHaveAttribute('data-status', 'ready');
    });
    // NO_FACE_WINDOW = 3 consecutive no-face frames trigger onFaceLost
    tickFrame(setResult, NO_FACE_FRAME, 33);
    tickFrame(setResult, NO_FACE_FRAME, 66);
    tickFrame(setResult, NO_FACE_FRAME, 99);
    await waitFor(() => {
      expect(screen.getByTestId('calibration-status')).toHaveAttribute('data-status', 'calibrating');
    });
  });
});
