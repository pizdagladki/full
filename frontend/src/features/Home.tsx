import { useEffect, useRef } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { useAuth } from './auth/AuthContext';
import { defaultRatingsApi } from '../api/ratings';
import type { RatingsApi, RatingData } from '../api/ratings';
import { PointsWidget } from './PointsWidget';
import type { PointsApi } from '../api/points';
import { useState } from 'react';

// ---------------------------------------------------------------------------
// Home — «Субботний мультик» (owner-approved design, mockup round 4.3).
// A cartoon world: sky, rolling hills, drifting clouds and a cursor-tracking
// sun-eye. Store/Profile pinned to the top corners, the LVL+ELO widget on the
// right edge, and THREE mode windows instead of a single Play button.
// The camera preview / device picker / track picker moved to ModeSelect (the
// «Онлайн-батлы» screen) — the owner's call: the face gate belongs on the
// screens leading into a match, not on the storefront.
// ---------------------------------------------------------------------------

export interface HomeProps {
  /** Injectable ratings API (swap with a mock in tests). Defaults to the real client. */
  ratingsApi?: RatingsApi;
  /** Injectable points API (swap with a mock in tests). Defaults to the real client. */
  pointsApi?: PointsApi;
}

export function Home({ ratingsApi = defaultRatingsApi, pointsApi }: HomeProps) {
  const { user } = useAuth();
  const navigate = useNavigate();

  // --- rating / level ---
  const [rating, setRating] = useState<RatingData | null>(null);
  // If there's no user we'll never fetch, so start as not-loading.
  const [ratingLoading, setRatingLoading] = useState<boolean>(user != null);
  const [ratingError, setRatingError] = useState<string | null>(null);

  useEffect(() => {
    if (!user) return;
    let cancelled = false;
    ratingsApi
      .getRating(user.id)
      .then((data) => {
        if (cancelled) return;
        setRating(data);
        setRatingLoading(false);
      })
      .catch((err: unknown) => {
        if (cancelled) return;
        const msg = err instanceof Error ? err.message : 'Failed to load rating';
        setRatingError(msg);
        setRatingLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [user, ratingsApi]);

  // --- sun-eye: the pupil follows the cursor (design signature) ---
  const sunRef = useRef<HTMLDivElement>(null);
  const pupilRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const onMove = (e: MouseEvent) => {
      const sun = sunRef.current;
      const pupil = pupilRef.current;
      if (!sun || !pupil) return;
      const r = sun.getBoundingClientRect();
      const dx = e.clientX - (r.left + r.width / 2);
      const dy = e.clientY - (r.top + r.height / 2);
      const d = Math.min(1, Math.hypot(dx, dy) / 300);
      const a = Math.atan2(dy, dx);
      pupil.style.transform = `translate(${Math.cos(a) * 10 * d}px, ${Math.sin(a) * 10 * d}px)`;
    };
    document.addEventListener('mousemove', onMove);
    return () => document.removeEventListener('mousemove', onMove);
  }, []);

  const MAX_LEVEL = 10;
  const levelPercent = rating != null ? Math.min(100, (rating.level / MAX_LEVEL) * 100) : 0;

  return (
    <div className="world" data-testid="home-screen">
      {/* ambient world */}
      <div className="world-cloud" style={{ top: '9%', width: 104, animationDuration: '40s' }} />
      <div
        className="world-cloud"
        style={{ top: '22%', width: 70, animationDuration: '58s', animationDelay: '-22s' }}
      />
      <span className="world-bird" style={{ top: '15%' }} aria-hidden="true">
        🕊️
      </span>
      <div className="sun" ref={sunRef} data-testid="sun-eye">
        <div className="sun-white">
          <div className="sun-pupil" ref={pupilRef} />
        </div>
      </div>
      <div className="world-hill world-hill--left" />
      <div className="world-hill world-hill--right" />

      {/* Points widget (balance + info panel) */}
      <PointsWidget pointsApi={pointsApi} />

      {/* Top AdSense banner slot */}
      <div data-testid="ad-slot" aria-hidden="true" />

      {/* corner navigation */}
      <Link className="corner-link corner-link--left" to="/store">
        🛒 Магазин
      </Link>
      <Link className="corner-link corner-link--right" to="/profile">
        😎 Профиль
      </Link>

      {/* LVL + ELO widget */}
      <section className="rank" aria-label="Level progress">
        {ratingLoading || ratingError || rating === null ? (
          <div className="rank-elo" data-testid="level-placeholder" aria-label="Level loading">
            {ratingError ? 'Could not load level' : 'Loading level…'}
          </div>
        ) : (
          <>
            <div className="rank-level">{rating.level}</div>
            <div className="rank-elo">
              <small>рейтинг</small>
              ELO {rating.elo}
              <div className="rank-elo-bar">
                <div
                  role="progressbar"
                  aria-valuenow={levelPercent}
                  aria-valuemin={0}
                  aria-valuemax={100}
                  aria-label={`Level ${rating.level} progress`}
                  style={{ width: `${levelPercent}%` }}
                />
              </div>
            </div>
          </>
        )}
      </section>

      <div className="home-shell">
        <div className="home-mid">
          <h1 className="home-logo">ГЛЯДЕЛКИ</h1>

          {/* mode windows */}
          <nav className="modes" aria-label="Game modes">
            <button
              type="button"
              className="mode"
              data-testid="mode-battles"
              onClick={() => navigate('/mode-select')}
            >
              <div className="mode-frame">
                <div className="mode-scene scene-battle">
                  <div className="scene-battle-vs">VS!</div>
                  <div className="scene-battle-duel">
                    <div className="scene-battle-eye scene-battle-eye--left">
                      <b />
                    </div>
                    <div className="scene-battle-zap" aria-hidden="true">
                      ⚡
                    </div>
                    <div className="scene-battle-eye scene-battle-eye--right">
                      <b />
                    </div>
                  </div>
                </div>
              </div>
              <div className="mode-plaque">Онлайн-батлы</div>
              <div className="mode-sub">ранк и не ранк — внутри</div>
            </button>

            <button
              type="button"
              className="mode"
              data-testid="mode-koth"
              onClick={() => navigate('/koth')}
            >
              <div className="mode-frame">
                <div className="mode-scene scene-koth">
                  <div className="scene-koth-cloud" style={{ left: -60 }} />
                  <div className="scene-koth-mountain" />
                  <div className="scene-koth-cap" />
                  <div className="scene-koth-crown" aria-hidden="true">
                    👑
                  </div>
                  <div className="scene-koth-flag" aria-hidden="true">
                    🚩
                  </div>
                </div>
              </div>
              <div className="mode-plaque mode-plaque--koth">Царь горы</div>
              <div className="mode-sub">вершина занята — сгони</div>
            </button>

            <button
              type="button"
              className="mode"
              data-testid="mode-invite"
              onClick={() => navigate('/invite')}
            >
              <div className="mode-frame">
                <div className="mode-scene scene-friend">
                  <div className="scene-friend-code">код: 7FK2</div>
                  <div className="scene-friend-peek" aria-hidden="true">
                    👀
                  </div>
                  <div className="scene-friend-door">
                    <div className="scene-friend-knob" />
                  </div>
                </div>
              </div>
              <div className="mode-plaque mode-plaque--friend">С другом</div>
              <div className="mode-sub">приватная комната по коду</div>
            </button>
          </nav>
        </div>
      </div>

      {/* Bottom AdSense banner slot */}
      <div data-testid="ad-slot" aria-hidden="true" />
    </div>
  );
}
