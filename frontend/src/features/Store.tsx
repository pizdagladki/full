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
    <div data-testid="store-screen">
      <h1>Store</h1>

      {/* AdSense banner slot */}
      <div data-testid="ad-slot" aria-hidden="true" />

      {catalogLoading ? (
        <div data-testid="store-loading">Loading store…</div>
      ) : catalogError ? (
        <div data-testid="store-error">Could not load store</div>
      ) : catalog && catalog.length === 0 ? (
        <div data-testid="store-empty">No products available</div>
      ) : (
        GROUP_KINDS.map((kind) => {
          const items = (catalog ?? []).filter((product) => product.kind === kind);
          if (items.length === 0) return null;
          return (
            <section key={kind} aria-label={`${kind} products`}>
              <h2 data-testid={`group-${kind}`}>
                {kind === 'distraction' ? 'Distractions' : 'Edits'}
              </h2>
              <ul>
                {items.map((product) => {
                  const owned = inventory.has(product.id);
                  const quantity = inventory.get(product.id);
                  const buyError = buyErrors.get(product.id);
                  const isInsufficient = insufficient.has(product.id);
                  return (
                    <li key={product.id} data-testid={`product-${product.id}`}>
                      <span data-testid={`name-${product.id}`}>{product.name}</span>

                      {product.is_free ? (
                        <span data-testid={`price-free-${product.id}`}>Free</span>
                      ) : (
                        <span data-testid={`price-money-${product.id}`}>
                          ${(product.price_cents / 100).toFixed(2)}
                        </span>
                      )}

                      {product.points_price != null && (
                        <span data-testid={`price-points-${product.id}`}>
                          {product.points_price} pts
                        </span>
                      )}

                      {owned && (
                        <span data-testid={`owned-${product.id}`}>
                          Owned
                          {product.kind === 'distraction' && (
                            <span data-testid={`qty-${product.id}`}> x{quantity}</span>
                          )}
                        </span>
                      )}

                      <button
                        type="button"
                        data-testid={`buy-money-${product.id}`}
                        onClick={() => void handleBuyWithMoney(product.id)}
                      >
                        Buy with money
                      </button>

                      {product.points_price != null && (
                        <button
                          type="button"
                          data-testid={`buy-points-${product.id}`}
                          onClick={() => void handleBuyWithPoints(product.id, product.kind)}
                        >
                          Buy with points
                        </button>
                      )}

                      {isInsufficient && (
                        <div data-testid={`insufficient-${product.id}`}>Not enough points</div>
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
