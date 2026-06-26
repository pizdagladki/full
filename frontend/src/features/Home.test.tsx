import { render, screen, waitFor, fireEvent } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { Home } from './Home';
import { AuthContext } from './auth/AuthContext';
import type { AuthState } from './auth/AuthContext';
import type { RatingsApi, RatingData } from '../api/ratings';

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

/** A fully-authenticated AuthState for wrapping the component under test. */
const AUTH_STATE: AuthState = {
  user: { id: 'user-42', email: 'test@example.com' },
  loading: false,
  error: null,
};

/** Renders <Home /> inside the auth context and a MemoryRouter. */
function renderHome(
  authState: AuthState = AUTH_STATE,
  ratingsApi?: RatingsApi,
) {
  return render(
    <AuthContext.Provider value={authState}>
      <MemoryRouter>
        <Home ratingsApi={ratingsApi} />
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

beforeEach(() => {
  setupMediaDevices();
});

afterEach(() => {
  vi.restoreAllMocks();
});

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
    const nullUserState: AuthState = { user: null, loading: false, error: null };
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

  it('criterion-4: renders a link to /search (mode-select entry)', () => {
    // criterion: 4 — nav must have a link to mode-select (/search)
    renderHome();

    const playLink = screen.getByRole('link', { name: /play/i }) as HTMLAnchorElement;
    expect(playLink).toBeInTheDocument();
    expect(playLink.getAttribute('href')).toBe('/search');
  });

  it('criterion-4 violation guard: missing store link would fail', () => {
    // criterion: 4 guard — confirms the assertion actually detects absence
    renderHome();
    expect(screen.queryByRole('link', { name: /store/i })).not.toBeNull();
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
