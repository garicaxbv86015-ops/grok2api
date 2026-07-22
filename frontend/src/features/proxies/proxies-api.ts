import { apiRequest, type PaginatedDTO } from "@/shared/api/client";
import { createObjectDecoder, createPaginatedDecoder, createValidatedDecoder, decodeBooleanResult, hasShape, isArrayOf, isBoolean, isNumber, isOneOf, isOptional, isString } from "@/shared/api/decoder";
import type { SortOrder } from "@/shared/lib/table-sort";

export type ProxyProtocol = "http" | "https" | "socks5" | "socks5h";

export type ProxyDTO = {
  id: string;
  name: string;
  protocol: ProxyProtocol;
  address: string;
  authConfigured: boolean;
  enabled: boolean;
  lastTestOK?: boolean;
  lastLatencyMS?: number;
  lastTestError?: string;
  lastTestAt?: string;
  boundFamilyCount: number;
  createdAt: string;
  updatedAt: string;
};

export type ProxyInput = {
  name: string;
  enabled: boolean;
  proxyURL?: string;
};

export type ProxyListInput = {
  page: number;
  pageSize: number;
  search?: string;
  protocol?: string;
  enabled?: boolean;
  sortBy?: string;
  sortOrder?: SortOrder;
};

export type ProxyProbeDTO = {
  ok: boolean;
  latencyMS?: number;
  error: string;
  testedAt: string;
};

export type ProxyBatchProbeItemDTO = {
  id: string;
  name: string;
  ok: boolean;
  latencyMS?: number;
  error: string;
  testedAt: string;
};

export type ProxyBatchProbeDTO = {
  total: number;
  succeeded: number;
  failed: number;
  items: ProxyBatchProbeItemDTO[];
};

const proxyValidator = hasShape({
  id: isString, name: isString, protocol: isOneOf("http", "https", "socks5", "socks5h"), address: isString,
  authConfigured: isBoolean, enabled: isBoolean, lastTestOK: isOptional(isBoolean), lastLatencyMS: isOptional(isNumber),
  lastTestError: isOptional(isString), lastTestAt: isOptional(isString), boundFamilyCount: isNumber, createdAt: isString, updatedAt: isString,
});
const decodeProxy = createValidatedDecoder<ProxyDTO>("proxy", proxyValidator);
const decodeProxyPage = createPaginatedDecoder<ProxyDTO>(proxyValidator);
const decodeProxyOptions = createObjectDecoder<{ items: ProxyDTO[] }>("proxy options", { items: isArrayOf(proxyValidator) });
const decodeProbe = createObjectDecoder<ProxyProbeDTO>("proxy probe", { ok: isBoolean, latencyMS: isOptional(isNumber), error: isString, testedAt: isString });
const batchProbeItemValidator = hasShape({
  id: isString, name: isString, ok: isBoolean, latencyMS: isOptional(isNumber), error: isString, testedAt: isString,
});
const decodeBatchProbe = createObjectDecoder<ProxyBatchProbeDTO>("proxy batch probe", {
  total: isNumber, succeeded: isNumber, failed: isNumber, items: isArrayOf(batchProbeItemValidator),
});

// listProxies 查询通用代理分页列表。
export function listProxies(input: ProxyListInput): Promise<PaginatedDTO<ProxyDTO>> {
  const query = new URLSearchParams({ page: String(input.page), pageSize: String(input.pageSize) });
  if (input.search) query.set("search", input.search);
  if (input.protocol) query.set("protocol", input.protocol);
  if (input.enabled !== undefined) query.set("enabled", String(input.enabled));
  if (input.sortBy && input.sortOrder) {
    query.set("sortBy", input.sortBy);
    query.set("sortOrder", input.sortOrder);
  }
  return apiRequest(`/api/admin/v1/proxies?${query}`, {}, decodeProxyPage);
}

// listProxyOptions 返回账号编辑器可选择的启用代理。
export async function listProxyOptions(): Promise<ProxyDTO[]> {
  const result = await apiRequest("/api/admin/v1/proxies/options", {}, decodeProxyOptions);
  return result.items;
}

// createProxy 创建通用代理。
export function createProxy(input: ProxyInput): Promise<ProxyDTO> {
  return apiRequest("/api/admin/v1/proxies", { method: "POST", body: input }, decodeProxy);
}

// updateProxy 更新通用代理，proxyURL 缺省时保留原地址。
export function updateProxy(id: string, input: ProxyInput): Promise<ProxyDTO> {
  return apiRequest(`/api/admin/v1/proxies/${id}`, { method: "PUT", body: input }, decodeProxy);
}

// deleteProxy 删除未被账号组引用的通用代理。
export function deleteProxy(id: string): Promise<{ deleted: boolean }> {
  return apiRequest(`/api/admin/v1/proxies/${id}`, { method: "DELETE" }, decodeBooleanResult<{ deleted: boolean }>("deleted"));
}

// testProxyConnection 测试单个代理连接并返回安全摘要。
export function testProxyConnection(id: string): Promise<ProxyProbeDTO> {
  return apiRequest(`/api/admin/v1/proxies/${id}/test`, { method: "POST" }, decodeProbe);
}

// testAllProxyConnections 并发测试全部代理连接并返回汇总结果。
export function testAllProxyConnections(): Promise<ProxyBatchProbeDTO> {
  return apiRequest("/api/admin/v1/proxies/test-all", { method: "POST" }, decodeBatchProbe);
}
