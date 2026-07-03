import { render, screen, waitFor, fireEvent } from '@testing-library/react';
import { describe, it, expect, vi, afterEach } from 'vitest';
import { Store } from './Store';
import { ApiError } from '../api/auth';
import type {
  StoreApi,
  Product,
  InventoryItem,
  MoneyPurchaseResult,
  PointsPurchaseResult,
} from '../api/store';

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function makeProduct(overrides: Partial<Product> = {}): Product {
  return {
    id: 1,
    kind: 'distraction',
    tier: null,
    name: 'Confetti bomb',
    price_cents: 199,
    is_free: false,
    points_price: null,
    ...overrides,
  };
}

function makeStoreApi(options: {
  catalog?: Product[] | Error;
  inventory?: InventoryItem[] | Error;
  purchaseWithMoney?: MoneyPurchaseResult | Error;
  purchaseWithPoints?: PointsPurchaseResult | Error;
}): StoreApi {
  const { catalog = [], inventory = [], purchaseWithMoney, purchaseWithPoints } = options;

  return {
    getCatalog:
      catalog instanceof Error
        ? vi.fn().mockRejectedValue(catalog)
        : vi.fn().mockResolvedValue(catalog),
    getInventory:
      inventory instanceof Error
        ? vi.fn().mockRejectedValue(inventory)
        : vi.fn().mockResolvedValue(inventory),
    purchaseWithMoney:
      purchaseWithMoney instanceof Error
        ? vi.fn().mockRejectedValue(purchaseWithMoney)
        : vi
            .fn()
            .mockResolvedValue(purchaseWithMoney ?? { client_secret: 'secret', product_id: 1 }),
    purchaseWithPoints:
      purchaseWithPoints instanceof Error
        ? vi.fn().mockRejectedValue(purchaseWithPoints)
        : vi.fn().mockResolvedValue(purchaseWithPoints ?? { product_id: 1, balance: 0 }),
  };
}

function makePendingStoreApi(): StoreApi {
  return {
    getCatalog: vi.fn().mockReturnValue(new Promise(() => {})),
    getInventory: vi.fn().mockReturnValue(new Promise(() => {})),
    purchaseWithMoney: vi.fn(),
    purchaseWithPoints: vi.fn(),
  };
}

afterEach(() => {
  vi.restoreAllMocks();
});

// ---------------------------------------------------------------------------
// Criterion 1 — loading / error / empty states
// ---------------------------------------------------------------------------

describe('criterion 1 — loading, error, empty catalog states', () => {
  it('criterion-1: shows a loading placeholder until catalog resolves', () => {
    // criterion: 1 — before catalog resolves, store-loading must be shown
    const api = makePendingStoreApi();
    render(<Store storeApi={api} />);

    expect(screen.getByTestId('store-loading')).toBeInTheDocument();
    expect(screen.queryByTestId('store-empty')).not.toBeInTheDocument();
  });

  it('criterion-1: shows a non-crashing error placeholder when catalog fetch fails', async () => {
    // criterion: 1 — catalog rejection must degrade to store-error, never throw
    const api = makeStoreApi({ catalog: new Error('network down') });
    expect(() => render(<Store storeApi={api} />)).not.toThrow();

    await waitFor(() => {
      expect(screen.getByTestId('store-error')).toBeInTheDocument();
    });
    expect(screen.queryByTestId('store-loading')).not.toBeInTheDocument();
  });

  it('criterion-1: shows an empty-state when the catalog resolves to an empty array', async () => {
    // criterion: 1 — an empty catalog array must show store-empty, not an error
    const api = makeStoreApi({ catalog: [] });
    render(<Store storeApi={api} />);

    await waitFor(() => {
      expect(screen.getByTestId('store-empty')).toBeInTheDocument();
    });
    expect(screen.queryByTestId('store-error')).not.toBeInTheDocument();
  });

  it('criterion-1: inventory fetch failure does not crash the screen (nothing owned)', async () => {
    // criterion: 1 — inventory rejection must be treated as "nothing owned", never crash
    const product = makeProduct({ id: 5 });
    const api = makeStoreApi({ catalog: [product], inventory: new Error('inventory down') });
    expect(() => render(<Store storeApi={api} />)).not.toThrow();

    await waitFor(() => {
      expect(screen.getByTestId('product-5')).toBeInTheDocument();
    });
    expect(screen.queryByTestId('owned-5')).not.toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Criterion 2 — catalog grouped/labeled by kind
// ---------------------------------------------------------------------------

describe('criterion 2 — catalog grouped by kind', () => {
  it('criterion-2: renders distraction and edit groups with visible labels, each only if it has items', async () => {
    // criterion: 2 — both group headings must appear when both kinds are present
    const distraction = makeProduct({ id: 1, kind: 'distraction', name: 'Confetti' });
    const edit = makeProduct({ id: 2, kind: 'edit', name: 'Neon filter' });
    const api = makeStoreApi({ catalog: [distraction, edit] });
    render(<Store storeApi={api} />);

    await waitFor(() => {
      expect(screen.getByTestId('group-distraction')).toBeInTheDocument();
    });
    expect(screen.getByTestId('group-edit')).toBeInTheDocument();
  });

  it('criterion-2 violation guard: only the present kind group renders, the absent one does not', async () => {
    // criterion: 2 — a kind with no items must NOT render its group heading
    const distraction = makeProduct({ id: 1, kind: 'distraction' });
    const api = makeStoreApi({ catalog: [distraction] });
    render(<Store storeApi={api} />);

    await waitFor(() => {
      expect(screen.getByTestId('group-distraction')).toBeInTheDocument();
    });
    expect(screen.queryByTestId('group-edit')).not.toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Criterion 3 — product row prices (money + points, free path)
// ---------------------------------------------------------------------------

describe('criterion 3 — product row pricing', () => {
  it('criterion-3: shows dual money + points prices when points_price is set', async () => {
    // criterion: 3 — both price-money and price-points must be shown for a dual-priced product
    const product = makeProduct({ id: 10, price_cents: 250, points_price: 500 });
    const api = makeStoreApi({ catalog: [product] });
    render(<Store storeApi={api} />);

    await waitFor(() => {
      expect(screen.getByTestId('product-10')).toBeInTheDocument();
    });
    expect(screen.getByTestId('price-money-10')).toHaveTextContent('$2.50');
    expect(screen.getByTestId('price-points-10')).toHaveTextContent('500');
    expect(screen.queryByTestId('price-free-10')).not.toBeInTheDocument();
  });

  it('criterion-3: shows "Free" instead of a money price when is_free is true', async () => {
    // criterion: 3 — a free product must show price-free, not price-money
    const product = makeProduct({ id: 11, is_free: true, price_cents: 0 });
    const api = makeStoreApi({ catalog: [product] });
    render(<Store storeApi={api} />);

    await waitFor(() => {
      expect(screen.getByTestId('price-free-11')).toBeInTheDocument();
    });
    expect(screen.getByTestId('price-free-11')).toHaveTextContent('Free');
    expect(screen.queryByTestId('price-money-11')).not.toBeInTheDocument();
  });

  it('criterion-3 violation guard: money-only product (points_price null) shows no points price', async () => {
    // criterion: 3 — points_price:null must NOT render a price-points element
    const product = makeProduct({ id: 12, points_price: null });
    const api = makeStoreApi({ catalog: [product] });
    render(<Store storeApi={api} />);

    await waitFor(() => {
      expect(screen.getByTestId('product-12')).toBeInTheDocument();
    });
    expect(screen.queryByTestId('price-points-12')).not.toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Criterion 4 — owned marking from inventory (incl. distraction quantity)
// ---------------------------------------------------------------------------

describe('criterion 4 — owned marking from inventory', () => {
  it('criterion-4: marks a product owned and shows quantity for distractions', async () => {
    // criterion: 4 — a distraction present in inventory must show owned + qty
    const product = makeProduct({ id: 20, kind: 'distraction' });
    const api = makeStoreApi({
      catalog: [product],
      inventory: [{ product_id: 20, quantity: 3 }],
    });
    render(<Store storeApi={api} />);

    await waitFor(() => {
      expect(screen.getByTestId('owned-20')).toBeInTheDocument();
    });
    expect(screen.getByTestId('qty-20')).toHaveTextContent('3');
  });

  it('criterion-4 violation guard: a product absent from inventory shows no owned marker', async () => {
    // criterion: 4 — a product not in inventory must NOT show an owned marker
    const product = makeProduct({ id: 21, kind: 'distraction' });
    const api = makeStoreApi({ catalog: [product], inventory: [] });
    render(<Store storeApi={api} />);

    await waitFor(() => {
      expect(screen.getByTestId('product-21')).toBeInTheDocument();
    });
    expect(screen.queryByTestId('owned-21')).not.toBeInTheDocument();
  });

  it('criterion-4: edit products owned show no quantity element', async () => {
    // criterion: 4 — an owned edit product must show owned but not a qty element
    const product = makeProduct({ id: 22, kind: 'edit' });
    const api = makeStoreApi({
      catalog: [product],
      inventory: [{ product_id: 22, quantity: 1 }],
    });
    render(<Store storeApi={api} />);

    await waitFor(() => {
      expect(screen.getByTestId('owned-22')).toBeInTheDocument();
    });
    expect(screen.queryByTestId('qty-22')).not.toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Criterion 5 — buy with money -> stripeConfirm
// ---------------------------------------------------------------------------

describe('criterion 5 — buy with money', () => {
  it('criterion-5: clicking buy-money calls purchaseWithMoney then stripeConfirm with the client secret', async () => {
    // criterion: 5 — a successful money purchase must invoke stripeConfirm with the returned client_secret
    const product = makeProduct({ id: 30 });
    const api = makeStoreApi({
      catalog: [product],
      purchaseWithMoney: { client_secret: 'pi_secret_123', product_id: 30 },
    });
    const stripeConfirm = vi.fn();
    render(<Store storeApi={api} stripeConfirm={stripeConfirm} />);

    await waitFor(() => {
      expect(screen.getByTestId('buy-money-30')).toBeInTheDocument();
    });
    fireEvent.click(screen.getByTestId('buy-money-30'));

    await waitFor(() => {
      expect(api.purchaseWithMoney).toHaveBeenCalledWith(30);
    });
    await waitFor(() => {
      expect(stripeConfirm).toHaveBeenCalledWith('pi_secret_123');
    });
  });

  it('criterion-5: a failed money purchase shows a non-crashing buy-error and never calls stripeConfirm', async () => {
    // criterion: 5 — a rejected purchaseWithMoney must show buy-error-<id>, not crash, not confirm
    const product = makeProduct({ id: 31 });
    const api = makeStoreApi({ catalog: [product], purchaseWithMoney: new Error('card declined') });
    const stripeConfirm = vi.fn();
    expect(() => render(<Store storeApi={api} stripeConfirm={stripeConfirm} />)).not.toThrow();

    await waitFor(() => {
      expect(screen.getByTestId('buy-money-31')).toBeInTheDocument();
    });
    fireEvent.click(screen.getByTestId('buy-money-31'));

    await waitFor(() => {
      expect(screen.getByTestId('buy-error-31')).toBeInTheDocument();
    });
    expect(stripeConfirm).not.toHaveBeenCalled();
  });
});

// ---------------------------------------------------------------------------
// Criterion 6 — buy with points, 402 insufficient, generic error
// ---------------------------------------------------------------------------

describe('criterion 6 — buy with points', () => {
  it('criterion-6: points button is rendered only when points_price is set', async () => {
    // criterion: 6 — money-only product must NOT render a buy-points button
    const product = makeProduct({ id: 40, points_price: null });
    const api = makeStoreApi({ catalog: [product] });
    render(<Store storeApi={api} />);

    await waitFor(() => {
      expect(screen.getByTestId('product-40')).toBeInTheDocument();
    });
    expect(screen.queryByTestId('buy-points-40')).not.toBeInTheDocument();
  });

  it('criterion-6: a successful points purchase marks the product owned without a refetch', async () => {
    // criterion: 6 — a successful points purchase must update owned state locally (no extra getInventory/getCatalog call)
    const product = makeProduct({ id: 41, kind: 'distraction', points_price: 100 });
    const api = makeStoreApi({
      catalog: [product],
      inventory: [],
      purchaseWithPoints: { product_id: 41, balance: 400 },
    });
    render(<Store storeApi={api} />);

    await waitFor(() => {
      expect(screen.getByTestId('buy-points-41')).toBeInTheDocument();
    });
    const getCatalogCallsBefore = (api.getCatalog as ReturnType<typeof vi.fn>).mock.calls.length;
    const getInventoryCallsBefore = (api.getInventory as ReturnType<typeof vi.fn>).mock.calls
      .length;

    fireEvent.click(screen.getByTestId('buy-points-41'));

    await waitFor(() => {
      expect(screen.getByTestId('owned-41')).toBeInTheDocument();
    });
    expect(screen.getByTestId('qty-41')).toHaveTextContent('1');
    expect((api.getCatalog as ReturnType<typeof vi.fn>).mock.calls.length).toBe(
      getCatalogCallsBefore,
    );
    expect((api.getInventory as ReturnType<typeof vi.fn>).mock.calls.length).toBe(
      getInventoryCallsBefore,
    );
  });

  it('criterion-6: a 402 ApiError shows a non-crashing insufficient-points state', async () => {
    // criterion: 6 — status 402 from purchaseWithPoints must show insufficient-<id>, not a generic error
    const product = makeProduct({ id: 42, points_price: 9999 });
    const api = makeStoreApi({
      catalog: [product],
      purchaseWithPoints: new ApiError(402, 'insufficient points'),
    });
    expect(() => render(<Store storeApi={api} />)).not.toThrow();

    await waitFor(() => {
      expect(screen.getByTestId('buy-points-42')).toBeInTheDocument();
    });
    fireEvent.click(screen.getByTestId('buy-points-42'));

    await waitFor(() => {
      expect(screen.getByTestId('insufficient-42')).toBeInTheDocument();
    });
    expect(screen.queryByTestId('buy-error-42')).not.toBeInTheDocument();
    expect(screen.queryByTestId('owned-42')).not.toBeInTheDocument();
  });

  it('criterion-6: a non-402 error from purchaseWithPoints shows the generic buy-error state', async () => {
    // criterion: 6 — any other error must show buy-error-<id>, never insufficient-<id>
    const product = makeProduct({ id: 43, points_price: 100 });
    const api = makeStoreApi({
      catalog: [product],
      purchaseWithPoints: new ApiError(409, 'already owned'),
    });
    expect(() => render(<Store storeApi={api} />)).not.toThrow();

    await waitFor(() => {
      expect(screen.getByTestId('buy-points-43')).toBeInTheDocument();
    });
    fireEvent.click(screen.getByTestId('buy-points-43'));

    await waitFor(() => {
      expect(screen.getByTestId('buy-error-43')).toBeInTheDocument();
    });
    expect(screen.queryByTestId('insufficient-43')).not.toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Criterion 7 — AdSense banner slot
// ---------------------------------------------------------------------------

describe('criterion 7 — AdSense banner slot', () => {
  it('criterion-7: renders at least one ad-slot placeholder', async () => {
    // criterion: 7 — an ad-slot element must always be present on the store screen
    const api = makeStoreApi({ catalog: [] });
    render(<Store storeApi={api} />);

    expect(screen.getByTestId('ad-slot')).toBeInTheDocument();
    await waitFor(() => {
      expect(screen.getByTestId('store-empty')).toBeInTheDocument();
    });
    expect(screen.getByTestId('ad-slot')).toBeInTheDocument();
  });
});
