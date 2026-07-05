export { HttpClient } from './http';
export { WsClient } from './ws';
export type { WsClientApi } from './ws';
export { AuthApiClient, defaultAuthApi, ApiError } from './auth';
export type { AuthApi, User, ConsentInfo, ConsentPayload } from './auth';
export { RatingsApiClient, defaultRatingsApi } from './ratings';
export type { RatingsApi, RatingData } from './ratings';
export { MatchHistoryApiClient, defaultMatchHistoryApi } from './matches';
export type { MatchHistoryApi, MatchEntry } from './matches';
export { ClipsApiClient, defaultClipsApi } from './clips';
export type { ClipsApi, Clip } from './clips';
export { PointsApiClient, defaultPointsApi } from './points';
export type { PointsApi, PointsBalance } from './points';
export { StoreApiClient, defaultStoreApi } from './store';
export type {
  StoreApi,
  Product,
  InventoryItem,
  MoneyPurchaseResult,
  PointsPurchaseResult,
} from './store';
export { KingClipsApiClient, defaultKingClipsApi } from './kingClips';
export type { KingClipsApi, CurrentKingClip, KingClipUploadResult } from './kingClips';
export { KothApiClient, defaultKothApi } from './koth';
export type {
  KothApi,
  ChallengeHillRequest,
  ChallengeHillResult,
  KingInfo,
  RankedAttemptRequest,
  RankedAttemptResult,
} from './koth';
export { ReportsApiClient, defaultReportsApi } from './reports';
export type {
  ReportsApi,
  CheatReportRequest,
  BugReportRequest,
  BugReportDevice,
} from './reports';
