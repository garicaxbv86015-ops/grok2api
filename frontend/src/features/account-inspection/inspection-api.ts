import { ApiError, apiEventStream, apiRequest } from "@/shared/api/client";
import { createObjectDecoder, hasShape, isArrayOf, isBoolean, isNumber, isOneOf, isOptional, isString } from "@/shared/api/decoder";
import { i18n } from "@/shared/i18n";

export type InspectionState = "healthy" | "unavailable" | "uncertain" | "skipped" | "uninspected";
export type InspectionClassification = "healthy" | "disabled" | "reauth_required" | "quota_exhausted" | "permission_denied" | "probe_model_unavailable" | "temporary_rate_limited" | "probe_error" | "uninspected";
export type InspectionSuggestion = "keep" | "disable" | "reauth" | "review" | "none";
export type InspectionMode = "full" | "incremental";

export type InspectionItemDTO = {
  accountId: string;
  name: string;
  provider: "grok_build";
  enabled: boolean;
  authStatus: "active" | "reauthRequired";
  state: InspectionState;
  classification: InspectionClassification;
  reason: string;
  httpStatus?: number;
  model?: string;
  suggestion: InspectionSuggestion;
  inspectedAt?: string;
};

export type InspectionOverviewDTO = {
  total: number;
  healthy: number;
  permissionDenied: number;
  quotaExhausted: number;
  reauthRequired: number;
  exception: number;
  uninspected: number;
  results: InspectionItemDTO[];
};

export type InspectionProgressDTO = { completed: number; total: number };

export type InspectionInput = {
  provider: "grok_build";
  ids?: string[];
  includeDisabled?: boolean;
  concurrency?: number;
  mode?: InspectionMode;
};

const inspectionItemValidator = hasShape({
  accountId: isString, name: isString, provider: isOneOf("grok_build"), enabled: isBoolean, authStatus: isOneOf("active", "reauthRequired"),
  state: isOneOf("healthy", "unavailable", "uncertain", "skipped", "uninspected"),
  classification: isOneOf("healthy", "disabled", "reauth_required", "quota_exhausted", "permission_denied", "probe_model_unavailable", "temporary_rate_limited", "probe_error", "uninspected"),
  reason: isString, httpStatus: isOptional(isNumber), model: isOptional(isString), suggestion: isOneOf("keep", "disable", "reauth", "review", "none"), inspectedAt: isOptional(isString),
});

const decodeInspectionOverview = createObjectDecoder<InspectionOverviewDTO>("account inspection overview", {
  total: isNumber, healthy: isNumber, permissionDenied: isNumber, quotaExhausted: isNumber, reauthRequired: isNumber,
  exception: isNumber, uninspected: isNumber, results: isArrayOf(inspectionItemValidator),
});

type InspectionStreamPayload = Partial<InspectionOverviewDTO & InspectionProgressDTO> & {
  unavailable?: number;
  uncertain?: number;
  skipped?: number;
  code?: string;
  message?: string;
};

const decodeInspectionStreamPayload = createObjectDecoder<InspectionStreamPayload>("account inspection event", {
  completed: isOptional(isNumber), total: isOptional(isNumber), healthy: isOptional(isNumber), unavailable: isOptional(isNumber),
  uncertain: isOptional(isNumber), skipped: isOptional(isNumber), results: isOptional(isArrayOf(inspectionItemValidator)),
  code: isOptional(isString), message: isOptional(isString),
});

// getInspectionOverview 获取 Build 账号的最新巡检快照和分类统计。
// 无参数；返回巡检工作台数据。
export function getInspectionOverview(): Promise<InspectionOverviewDTO> {
  return apiRequest("/api/admin/v1/accounts/inspection", {}, decodeInspectionOverview);
}

// runInspection 通过 SSE 执行完整或增量 Build 账号巡检，并在完成后返回本次报告。
// 参数 input 为巡检范围，onProgress 接收实时进度，signal 可中止请求；返回巡检报告。
export async function runInspection(input: InspectionInput, onProgress?: (value: InspectionProgressDTO) => void, signal?: AbortSignal): Promise<void> {
  await apiEventStream("/api/admin/v1/accounts/inspect", {
    method: "POST",
    headers: { Accept: "text/event-stream" },
    body: input,
    signal,
  }, decodeInspectionStreamPayload, ({ event, data }) => {
    if (event === "progress" && typeof data.completed === "number" && typeof data.total === "number") {
      onProgress?.({ completed: data.completed, total: data.total });
      return;
    }
    if (event === "error") {
      const code = data.code ?? "accountInspectionFailed";
      throw new ApiError(502, code, i18n.exists(`apiErrors.${code}`) ? i18n.t(`apiErrors.${code}`) : (data.message ?? i18n.t("apiErrors.requestFailed")));
    }
  });
}
