import { useEffect, useState } from 'react';
import { ApiError } from '../api/auth';
import { defaultStoreApi } from '../api/store';
import type { Product, StoreApi } from '../api/store';

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

export interface StoreProps {
  /** Injectable store API (swap with a mock in tests). Defaults to the real client. */
  storeApi?: StoreApi;
  /** The Stripe SDK client action that confirms a PaymentIntent given its client secret. */
  stripeConfirm?: (clientSecret: string) => void | Promise<void>;
}

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const GROUP_KINDS: Array<Product['kind']> = ['distraction', 'edit'];

function noopStripeConfirm(): void {
  // No-op default — real Stripe confirmation is wired by the caller.
}

// ---------------------------------------------------------------------------
// Store component
// ---------------------------------------------------------------------------

export function Store({
  storeApi = defaultStoreApi,
  stripeConfirm = noopStripeConfirm,
}: StoreProps) {
  const [catalog, setCatalog] = useState<Product[] | null>(null);
  const [catalogLoading, setCatalogLoading] = useState<boolean>(true);
  const [catalogError, setCatalogError] = useState<string | null>(null);

  // product_id -> quantity owned. Presence in the map means "owned".
  const [inventory, setInventory] = useState<Map<number, number>>(new Map());

  // Per-product buy state.
  const [buyErrors, setBuyErrors] = useState<Map<number, string>>(new Map());
  const [insufficient, setInsufficient] = useState<Set<number>>(new Set());

  // Fetch catalog.
  useEffect(() => {
    let cancelled = false;
    storeApi
      .getCatalog()
      .then((data) => {
        if (cancelled) return;
        setCatalog(data);
        setCatalogLoading(false);
      })
      .catch((err: unknown) => {
        if (cancelled) return;
        const msg = err instanceof Error ? err.message : 'Failed to load store catalog';
        setCatalogError(msg);
        setCatalogLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [storeApi]);

  // Fetch inventory. Failure degrades to "nothing owned" — never crashes the screen.
  useEffect(() => {
    let cancelled = false;
    storeApi
      .getInventory()
      .then((data) => {
        if (cancelled) return;
        setInventory(new Map(data.map((item) => [item.product_id, item.quantity])));
      })
      .catch(() => {
        if (cancelled) return;
        setInventory(new Map());
      });
    return () => {
      cancelled = true;
    };
  }, [storeApi]);

  // ---------------------------------------------------------------------------
  // Handlers
  // ---------------------------------------------------------------------------

  function clearBuyState(productId: number) {
    setBuyErrors((prev) => {
      if (!prev.has(productId)) return prev;
      const next = new Map(prev);
      next.delete(productId);
      return next;
    });
    setInsufficient((prev) => {
      if (!prev.has(productId)) return prev;
      const next = new Set(prev);
      next.delete(productId);
      return next;
    });
  }

  async function handleBuyWithMoney(productId: number) {
    clearBuyState(productId);
    try {
      const result = await storeApi.purchaseWithMoney(productId);
      await stripeConfirm(result.client_secret);
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Purchase failed';
      setBuyErrors((prev) => new Map(prev).set(productId, msg));
    }
  }

  async function handleBuyWithPoints(productId: number, kind: Product['kind']) {
    clearBuyState(productId);
    try {
      await storeApi.purchaseWithPoints(productId);
      // Reflect ownership locally without a full refetch.
      setInventory((prev) => {
        const next = new Map(prev);
        if (kind === 'distraction') {
          next.set(productId, (next.get(productId) ?? 0) + 1);
        } else {
          next.set(productId, next.get(productId) ?? 1);
        }
        return next;
      });
    } catch (err: unknown) {
      if (err instanceof ApiError && err.status === 402) {
        setInsufficient((prev) => new Set(prev).add(productId));
        return;
      }
      const msg = err instanceof Error ? err.message : 'Purchase failed';
      setBuyErrors((prev) => new Map(prev).set(productId, msg));
    }
  }

  // ---------------------------------------------------------------------------
  // Render
  // ---------------------------------------------------------------------------

  return (
    <div className="panel-screen" data-testid="store-screen">
      <h1 className="panel-title">Магазин</h1>

      {/* AdSense banner slot */}
      <div data-testid="ad-slot" aria-hidden="true" />

      {catalogLoading ? (
        <div className="results-note" data-testid="store-loading">Открываем магазин…</div>
      ) : catalogError ? (
        <div className="results-note" data-testid="store-error">Не удалось загрузить магазин</div>
      ) : catalog && catalog.length === 0 ? (
        <div className="results-note" data-testid="store-empty">Товаров пока нет</div>
      ) : (
        GROUP_KINDS.map((kind) => {
          const items = (catalog ?? []).filter((product) => product.kind === kind);
          if (items.length === 0) return null;
          return (
            <section key={kind} className="sheet" aria-label={`${kind} products`}>
              <h2 className="sheet-title" data-testid={`group-${kind}`}>
                {kind === 'distraction' ? 'Отвлекалки' : 'Эдиты'}
              </h2>
              <ul className="sheet-list">
                {items.map((product) => {
                  const owned = inventory.has(product.id);
                  const quantity = inventory.get(product.id);
                  const buyError = buyErrors.get(product.id);
                  const isInsufficient = insufficient.has(product.id);
                  return (
                    <li key={product.id} className="store-item" data-testid={`product-${product.id}`}>
                      <span className="store-name" data-testid={`name-${product.id}`}>{product.name}</span>

                      {product.is_free ? (
                        <span className="store-chip store-chip--free" data-testid={`price-free-${product.id}`}>Бесплатно</span>
                      ) : (
                        <span className="store-chip" data-testid={`price-money-${product.id}`}>
                          ${(product.price_cents / 100).toFixed(2)}
                        </span>
                      )}

                      {product.points_price != null && (
                        <span className="store-chip store-chip--pts" data-testid={`price-points-${product.id}`}>
                          {product.points_price} pts
                        </span>
                      )}

                      {owned && (
                        <span className="store-chip store-chip--owned" data-testid={`owned-${product.id}`}>
                          Куплено
                          {product.kind === 'distraction' && (
                            <span data-testid={`qty-${product.id}`}> x{quantity}</span>
                          )}
                        </span>
                      )}

                      <button
                        type="button"
                        data-testid={`buy-money-${product.id}`}
                        className="btn-mini"
                        onClick={() => void handleBuyWithMoney(product.id)}
                      >
                        Купить за деньги
                      </button>

                      {product.points_price != null && (
                        <button
                          type="button"
                          data-testid={`buy-points-${product.id}`}
                          className="btn-mini btn-mini--pts"
                          onClick={() => void handleBuyWithPoints(product.id, product.kind)}
                        >
                          Купить за поинты
                        </button>
                      )}

                      {isInsufficient && (
                        <div className="results-note" data-testid={`insufficient-${product.id}`}>Не хватает поинтов</div>
                      )}
                      {buyError && <div data-testid={`buy-error-${product.id}`}>{buyError}</div>}
                    </li>
                  );
                })}
              </ul>
            </section>
          );
        })
      )}
    </div>
  );
}
