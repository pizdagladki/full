import { ApiError } from './auth';

const DEFAULT_BASE_URL = (import.meta.env?.VITE_API_URL as string | undefined) ?? '';

export interface Product {
  id: number;
  kind: 'distraction' | 'edit';
  tier: number | null;
  name: string;
  price_cents: number;
  is_free: boolean;
  points_price: number | null;
}

export interface InventoryItem {
  product_id: number;
  quantity: number;
}

export interface MoneyPurchaseResult {
  client_secret: string;
  product_id: number;
}

export interface PointsPurchaseResult {
  product_id: number;
  balance: number;
}

export interface StoreApi {
  getCatalog(): Promise<Product[]>;
  getInventory(): Promise<InventoryItem[]>;
  purchaseWithMoney(productId: number): Promise<MoneyPurchaseResult>;
  purchaseWithPoints(productId: number): Promise<PointsPurchaseResult>;
}

export class StoreApiClient implements StoreApi {
  private readonly baseUrl: string;

  constructor(baseUrl: string = DEFAULT_BASE_URL) {
    this.baseUrl = baseUrl.replace(/\/$/, '');
  }

  async getCatalog(): Promise<Product[]> {
    const res = await fetch(`${this.baseUrl}/v1/store/catalog`, {
      method: 'GET',
      credentials: 'include',
    });
    if (!res.ok) {
      throw new ApiError(
        res.status,
        `GET /v1/store/catalog failed: ${res.status} ${res.statusText}`,
      );
    }
    return res.json() as Promise<Product[]>;
  }

  async getInventory(): Promise<InventoryItem[]> {
    const res = await fetch(`${this.baseUrl}/v1/store/inventory`, {
      method: 'GET',
      credentials: 'include',
    });
    if (!res.ok) {
      throw new ApiError(
        res.status,
        `GET /v1/store/inventory failed: ${res.status} ${res.statusText}`,
      );
    }
    return res.json() as Promise<InventoryItem[]>;
  }

  async purchaseWithMoney(productId: number): Promise<MoneyPurchaseResult> {
    const res = await fetch(`${this.baseUrl}/v1/store/purchase`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ product_id: productId, pay_with: 'money' }),
      credentials: 'include',
    });
    if (!res.ok) {
      throw new ApiError(
        res.status,
        `POST /v1/store/purchase failed: ${res.status} ${res.statusText}`,
      );
    }
    return res.json() as Promise<MoneyPurchaseResult>;
  }

  async purchaseWithPoints(productId: number): Promise<PointsPurchaseResult> {
    const res = await fetch(`${this.baseUrl}/v1/store/purchase`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ product_id: productId, pay_with: 'points' }),
      credentials: 'include',
    });
    if (!res.ok) {
      throw new ApiError(
        res.status,
        `POST /v1/store/purchase failed: ${res.status} ${res.statusText}`,
      );
    }
    return res.json() as Promise<PointsPurchaseResult>;
  }
}

export const defaultStoreApi: StoreApi = new StoreApiClient();
