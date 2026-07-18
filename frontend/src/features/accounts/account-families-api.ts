import { apiRequest, type PaginatedDTO } from "@/shared/api/client";
import { createPaginatedDecoder, decodeBooleanResult, decodeCountResult, hasShape, isArrayOf, isBoolean, isOneOf, isOptional, isString } from "@/shared/api/decoder";

import type { AccountProvider } from "@/features/accounts/accounts-api";

export type AccountFamilyMemberDTO = {
  id: string;
  provider: AccountProvider;
  name: string;
  email?: string;
  enabled: boolean;
  authStatus: "active" | "reauthRequired";
};

export type AccountFamilyDTO = {
  id: string;
  proxyId?: string;
  proxyName?: string;
  proxyEnabled: boolean;
  members: AccountFamilyMemberDTO[];
  createdAt: string;
  updatedAt: string;
};

type AccountFamilyListInput = {
  page: number;
  pageSize: number;
  search?: string;
  proxyBinding?: "bound" | "unbound";
};

export type AccountFamilyProxyInput = {
  proxyId?: string;
  clearProxy?: boolean;
};

const accountFamilyMemberValidator = hasShape({
  id: isString, provider: isOneOf("grok_build", "grok_web", "grok_console"), name: isString,
  email: isOptional(isString), enabled: isBoolean, authStatus: isOneOf("active", "reauthRequired"),
});
const accountFamilyValidator = hasShape({
  id: isString, proxyId: isOptional(isString), proxyName: isOptional(isString), proxyEnabled: isBoolean,
  members: isArrayOf(accountFamilyMemberValidator), createdAt: isString, updatedAt: isString,
});
const decodeAccountFamilyPage = createPaginatedDecoder<AccountFamilyDTO>(accountFamilyValidator);

// listAccountFamilies 分页查询逻辑账号组；input 包含分页、搜索和绑定筛选；返回分页账号组。
export function listAccountFamilies(input: AccountFamilyListInput): Promise<PaginatedDTO<AccountFamilyDTO>> {
  const query = new URLSearchParams({ page: String(input.page), pageSize: String(input.pageSize) });
  if (input.search) query.set("search", input.search);
  if (input.proxyBinding) query.set("proxyBinding", input.proxyBinding);
  return apiRequest(`/api/admin/v1/account-families?${query}`, {}, decodeAccountFamilyPage);
}

// deleteAccountFamily 删除逻辑账号组及其全部 Provider 成员；id 为组标识；返回删除成员数量。
export function deleteAccountFamily(id: string): Promise<{ deleted: number }> {
  return apiRequest(`/api/admin/v1/account-families/${id}`, { method: "DELETE" }, decodeCountResult<{ deleted: number }>("deleted"));
}

// updateAccountFamilyProxy 更新单个账号组代理；id 为组标识，input 为绑定或解绑指令；返回是否更新。
export function updateAccountFamilyProxy(id: string, input: AccountFamilyProxyInput): Promise<{ updated: boolean }> {
  return apiRequest(`/api/admin/v1/account-families/${id}/proxy`, { method: "PATCH", body: input }, decodeBooleanResult<{ updated: boolean }>("updated"));
}

// batchUpdateAccountFamilyProxy 批量更新勾选账号组代理；ids 为组标识，input 为绑定或解绑指令；返回更新数。
export function batchUpdateAccountFamilyProxy(ids: string[], input: AccountFamilyProxyInput): Promise<{ updated: number }> {
  return apiRequest("/api/admin/v1/account-families/batch/proxy", { method: "PATCH", body: { ids, ...input } }, decodeCountResult<{ updated: number }>("updated"));
}
