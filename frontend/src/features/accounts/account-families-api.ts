import { apiRequest, type PaginatedDTO } from "@/shared/api/client";
import { createPaginatedDecoder, decodeBooleanResult, hasShape, isArrayOf, isBoolean, isOneOf, isOptional, isString } from "@/shared/api/decoder";

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
};

type AccountFamilyProxyInput = {
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

// listAccountFamilies 分页查询逻辑账号组及其三类 Provider 成员。
export function listAccountFamilies(input: AccountFamilyListInput): Promise<PaginatedDTO<AccountFamilyDTO>> {
  const query = new URLSearchParams({ page: String(input.page), pageSize: String(input.pageSize) });
  if (input.search) query.set("search", input.search);
  return apiRequest(`/api/admin/v1/account-families?${query}`, {}, decodeAccountFamilyPage);
}

// updateAccountFamilyProxy 绑定、切换或解除一个逻辑账号组的固定代理。
export function updateAccountFamilyProxy(id: string, input: AccountFamilyProxyInput): Promise<{ updated: boolean }> {
  return apiRequest(`/api/admin/v1/account-families/${id}/proxy`, { method: "PATCH", body: input }, decodeBooleanResult<{ updated: boolean }>("updated"));
}
