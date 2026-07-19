import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { CircleStop, Play, Plus, ScanSearch, ShieldBan } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";

import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from "@/components/ui/alert-dialog";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Spinner } from "@/components/ui/spinner";
import { InspectionCategoryCards, type InspectionCategory } from "@/features/account-inspection/inspection-status";
import { InspectionTable } from "@/features/account-inspection/inspection-table";
import { getInspectionOverview, runInspection, type InspectionItemDTO, type InspectionMode, type InspectionProgressDTO } from "@/features/account-inspection/inspection-api";
import { deleteAccounts, updateAccountsEnabled } from "@/features/accounts/accounts-api";

type InspectionTarget = InspectionMode | string[];

// AccountInspectionPage 提供与截图一致的 Build 账号巡检工作台。
// 无参数；返回独立巡检页面。
export function AccountInspectionPage() {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const abortRef = useRef<AbortController | null>(null);
  const [category, setCategory] = useState<InspectionCategory>("all");
  const [concurrency, setConcurrency] = useState(6);
  const [includeDisabled, setIncludeDisabled] = useState(false);
  const [onlyDisabled, setOnlyDisabled] = useState(false);
  const [progress, setProgress] = useState<InspectionProgressDTO | null>(null);
  const [selected, setSelected] = useState<Set<string>>(() => new Set());
  const [deleteTargets, setDeleteTargets] = useState<string[] | null>(null);
  const overviewQuery = useQuery({ queryKey: ["account-inspection"], queryFn: getInspectionOverview });

  useEffect(() => () => abortRef.current?.abort(), []);

  const inspectMutation = useMutation({
    mutationFn: (target: InspectionTarget) => {
      const controller = new AbortController();
      abortRef.current = controller;
      setProgress(null);
      const input = Array.isArray(target)
        ? { provider: "grok_build" as const, ids: target, includeDisabled, concurrency }
        : { provider: "grok_build" as const, mode: target, includeDisabled, concurrency };
      return runInspection(input, setProgress, controller.signal);
    },
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["account-inspection"] });
      toast.success(t("accountInspection.completed"));
    },
    onError: (error) => {
      if (error instanceof DOMException && error.name === "AbortError") return;
      toast.error(error instanceof Error ? error.message : t("errors.generic"));
    },
    onSettled: () => {
      abortRef.current = null;
      setProgress(null);
    },
  });

  const disableMutation = useMutation({
    mutationFn: (ids: string[]) => updateAccountsEnabled(ids, false, "grok_build"),
    onSuccess: async (result) => {
      await queryClient.invalidateQueries({ queryKey: ["account-inspection"] });
      await queryClient.invalidateQueries({ queryKey: ["accounts"] });
      setSelected(new Set());
      toast.success(t("accountInspection.disabledCompleted", result));
    },
    onError: (error) => toast.error(error instanceof Error ? error.message : t("errors.generic")),
  });

  const deleteMutation = useMutation({
    mutationFn: (ids: string[]) => deleteAccounts(ids, "grok_build"),
    onSuccess: async (result) => {
      await queryClient.invalidateQueries({ queryKey: ["account-inspection"] });
      await queryClient.invalidateQueries({ queryKey: ["accounts"] });
      setSelected(new Set());
      setDeleteTargets(null);
      toast.success(t("accountInspection.deletedCompleted", result));
    },
    onError: (error) => toast.error(error instanceof Error ? error.message : t("errors.generic")),
  });

  const filteredRows = useMemo(() => (overviewQuery.data?.results ?? []).filter((item) => matchesInspectionCategory(item, category) && (!onlyDisabled || !item.enabled)), [category, onlyDisabled, overviewQuery.data]);
  const suggestedDisableIDs = filteredRows.filter((item) => item.enabled && item.suggestion === "disable").map((item) => item.accountId);
  const pending = inspectMutation.isPending || disableMutation.isPending || deleteMutation.isPending;
  const selectedIDs = [...selected];

  return (
    <div className="space-y-6">
      <header className="space-y-2">
        <span className="inline-flex rounded-full bg-primary/10 px-3 py-1 text-xs font-semibold text-primary">xAI / Grok · Build</span>
        <h1 className="text-3xl font-semibold tracking-tight">{t("accountInspection.title")}</h1>
        <p className="max-w-4xl text-sm text-muted-foreground">{t("accountInspection.description")}</p>
      </header>

      <section className="flex flex-wrap items-center gap-3 rounded-2xl border bg-card p-4 shadow-sm">
        <div className="flex items-center gap-2"><Label htmlFor="inspection-concurrency" className="text-sm">{t("accountInspection.concurrency")}</Label><Input id="inspection-concurrency" type="number" min={1} max={16} className="h-9 w-20" value={concurrency} disabled={inspectMutation.isPending} onChange={(event) => setConcurrency(clampConcurrency(event.target.value))} /></div>
        <Label className="flex h-9 items-center gap-2 rounded-full border px-3 text-xs"><Checkbox checked={includeDisabled} disabled={inspectMutation.isPending} onCheckedChange={(checked) => setIncludeDisabled(checked === true)} />{t("accountInspection.includeDisabled")}</Label>
        <Label className="flex h-9 items-center gap-2 rounded-full border px-3 text-xs"><Checkbox checked={onlyDisabled} disabled={inspectMutation.isPending} onCheckedChange={(checked) => setOnlyDisabled(checked === true)} />{t("accountInspection.onlyDisabled")}</Label>
        <div className="ml-auto flex flex-wrap items-center gap-2">
          {inspectMutation.isPending ? <Button variant="outline" size="sm" onClick={() => abortRef.current?.abort()}><CircleStop />{t("accountInspection.stop")}</Button> : null}
          <Button variant="secondary" size="sm" disabled={pending || suggestedDisableIDs.length === 0} onClick={() => disableMutation.mutate(suggestedDisableIDs)}><ShieldBan />{t("accountInspection.applySuggestions", { count: suggestedDisableIDs.length })}</Button>
          <Button variant="secondary" size="sm" disabled={pending} onClick={() => inspectMutation.mutate("incremental")}><Plus />{t("accountInspection.incremental")}</Button>
          <Button variant="outline" size="sm" disabled={pending || filteredRows.length === 0} onClick={() => inspectMutation.mutate(filteredRows.map((item) => item.accountId))}><ScanSearch />{t("accountInspection.inspectCurrentCategory")}</Button>
          <Button size="sm" disabled={pending} onClick={() => inspectMutation.mutate("full")}><Play />{inspectMutation.isPending ? <><Spinner />{progress ? `${progress.completed} / ${progress.total}` : t("accountInspection.running")}</> : <><ScanSearch />{t("accountInspection.start")}</>}</Button>
        </div>
      </section>

      {overviewQuery.data ? <InspectionCategoryCards overview={overviewQuery.data} active={category} onChange={(value) => { setCategory(value); setSelected(new Set()); }} /> : null}

      <section className="space-y-3">
        <div className="flex flex-wrap items-center gap-2">
          <Button variant="outline" size="sm" disabled={pending || selectedIDs.length === 0} onClick={() => disableMutation.mutate(selectedIDs)}>{t("accountInspection.batchDisable", { count: selectedIDs.length })}</Button>
          <Button variant="outline" size="sm" className="border-destructive/30 text-destructive hover:bg-destructive/10 hover:text-destructive" disabled={pending || selectedIDs.length === 0} onClick={() => setDeleteTargets(selectedIDs)}>{t("accountInspection.batchDelete", { count: selectedIDs.length })}</Button>
          <span className="ml-1 text-sm text-muted-foreground">{t("accountInspection.currentCategory", { category: t(`accountInspection.categories.${category}`), count: filteredRows.length })}</span>
          {overviewQuery.data?.uninspected ? <span className="text-xs text-muted-foreground">{t("accountInspection.uninspectedHint", { count: overviewQuery.data.uninspected })}</span> : null}
        </div>

        {overviewQuery.isPending ? <div className="flex min-h-56 items-center justify-center"><Spinner /></div> : null}
        {overviewQuery.isError ? <p className="rounded-xl border border-destructive/30 bg-destructive/5 p-4 text-sm text-destructive">{overviewQuery.error.message}</p> : null}
        {overviewQuery.data ? <InspectionTable rows={filteredRows} selected={selected} onSelectedChange={setSelected} onDisable={(id) => disableMutation.mutate([id])} onDelete={(id) => setDeleteTargets([id])} pending={pending} /> : null}
      </section>

      <AlertDialog open={deleteTargets !== null} onOpenChange={(open) => { if (!open) setDeleteTargets(null); }}>
        <AlertDialogContent>
          <AlertDialogHeader><AlertDialogTitle>{t("accountInspection.deleteTitle", { count: deleteTargets?.length ?? 0 })}</AlertDialogTitle><AlertDialogDescription>{t("accountInspection.deleteDescription")}</AlertDialogDescription></AlertDialogHeader>
          <AlertDialogFooter><AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel><AlertDialogAction className="bg-destructive text-white hover:bg-destructive/90" disabled={deleteMutation.isPending} onClick={(event) => { event.preventDefault(); if (deleteTargets) deleteMutation.mutate(deleteTargets); }}>{deleteMutation.isPending ? <Spinner /> : t("common.delete")}</AlertDialogAction></AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}

// matchesInspectionCategory 判断巡检项是否属于当前统计卡对应的分类。
// 参数 item 为巡检项，category 为分类卡标识；返回是否应显示。
function matchesInspectionCategory(item: InspectionItemDTO, category: InspectionCategory): boolean {
  if (category === "all") return true;
  if (category === "healthy") return item.state === "healthy";
  if (category === "permissionDenied") return item.classification === "permission_denied";
  if (category === "quotaExhausted") return item.classification === "quota_exhausted";
  if (category === "reauthRequired") return item.classification === "reauth_required";
  return item.state === "uncertain";
}

// clampConcurrency 将并发输入限制在后端允许的 1 到 16 范围内。
// 参数 value 为输入框原始字符串；返回规范化并发数。
function clampConcurrency(value: string): number {
  const parsed = Number.parseInt(value, 10);
  if (!Number.isFinite(parsed)) return 1;
  return Math.min(16, Math.max(1, parsed));
}
