import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Pencil, RefreshCw, Search } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Spinner } from "@/components/ui/spinner";
import { Table, TableActionCell, TableActionHead, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { EmptyState, ErrorState, TableLoadingRow } from "@/shared/components/data-state";
import { DataTableShell } from "@/shared/components/data-table-shell";
import { Pagination } from "@/shared/components/pagination";
import { useDebouncedValue } from "@/shared/hooks/use-debounced-value";
import { formatDateTime } from "@/shared/lib/format";
import { listProxyOptions } from "@/features/proxies/proxies-api";
import { listAccountFamilies, updateAccountFamilyProxy, type AccountFamilyDTO, type AccountFamilyMemberDTO } from "@/features/accounts/account-families-api";

// AccountFamiliesPanel 展示逻辑账号组，并提供组级固定代理绑定入口。
export function AccountFamiliesPanel() {
  const { t, i18n } = useTranslation();
  const queryClient = useQueryClient();
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [search, setSearch] = useState("");
  const [editing, setEditing] = useState<AccountFamilyDTO | null>(null);
  const [proxyID, setProxyID] = useState("none");
  const debouncedSearch = useDebouncedValue(search);

  const familiesQuery = useQuery({
    queryKey: ["account-families", page, pageSize, debouncedSearch],
    queryFn: () => listAccountFamilies({ page, pageSize, search: debouncedSearch }),
  });
  const proxyOptionsQuery = useQuery({
    queryKey: ["proxies", "options"],
    queryFn: listProxyOptions,
    enabled: editing !== null,
  });
  const bindingMutation = useMutation({
    mutationFn: () => {
      if (!editing) throw new Error(t("errors.generic"));
      return updateAccountFamilyProxy(editing.id, proxyID === "none" ? { clearProxy: true } : { proxyId: proxyID });
    },
    onSuccess: () => {
      setEditing(null);
      void queryClient.invalidateQueries({ queryKey: ["account-families"] });
      void queryClient.invalidateQueries({ queryKey: ["accounts"] });
      void queryClient.invalidateQueries({ queryKey: ["proxies"] });
      toast.success(t("accountFamilies.proxyUpdated"));
    },
    onError: showAccountFamilyError,
  });
  const result = familiesQuery.data;

  // beginProxyEdit 打开账号组代理编辑器并回填当前绑定。
  function beginProxyEdit(value: AccountFamilyDTO): void {
    setEditing(value);
    setProxyID(value.proxyId ?? "none");
  }

  return (
    <DataTableShell
      toolbar={<>
        <div className="relative min-w-56 flex-1 sm:w-72 sm:flex-none">
          <Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input className="h-8 pl-9 text-xs" value={search} onChange={(event) => { setSearch(event.target.value); setPage(1); }} placeholder={t("accountFamilies.search")} />
        </div>
        <Button variant="secondary" size="sm" disabled={familiesQuery.isFetching} onClick={() => void familiesQuery.refetch()}>{familiesQuery.isFetching ? <Spinner /> : <RefreshCw />}{t("common.refresh")}</Button>
      </>}
      footer={result && result.total > 0 ? <Pagination page={page} pageSize={pageSize} total={result.total} onPageChange={setPage} onPageSizeChange={(value) => { setPageSize(value); setPage(1); }} /> : undefined}
    >
      {familiesQuery.isError ? <ErrorState message={t("accountFamilies.loadFailed")} onRetry={() => void familiesQuery.refetch()} /> : !familiesQuery.isPending && result?.items.length === 0 ? <EmptyState message={t("accountFamilies.empty")} /> : (
        <Table className="min-w-[860px] table-fixed">
          <colgroup><col className="w-[24%]" /><col className="w-[34%]" /><col className="w-[18%]" /><col className="w-[10%]" /><col className="w-[10%]" /><col className="w-[4%]" /></colgroup>
          <TableHeader><TableRow>
            <TableHead>{t("accountFamilies.logicalAccount")}</TableHead><TableHead>{t("accountFamilies.members")}</TableHead><TableHead>{t("accountFamilies.proxy")}</TableHead><TableHead>{t("accountFamilies.status")}</TableHead><TableHead>{t("accountFamilies.createdAt")}</TableHead><TableActionHead />
          </TableRow></TableHeader>
          <TableBody>
            {familiesQuery.isPending ? <TableLoadingRow colSpan={6} /> : result?.items.map((value) => {
              const primary = primaryMember(value.members);
              const healthy = value.members.every((member) => member.enabled && member.authStatus === "active");
              return <TableRow key={value.id}>
                <TableCell className="min-w-0"><div className="truncate text-xs font-medium" title={primary?.name}>{primary?.name || t("accountFamilies.unnamed")}</div><div className="mt-0.5 truncate text-xs text-muted-foreground" title={primary?.email}>{primary?.email || t("accountFamilies.familyID", { id: value.id })}</div></TableCell>
                <TableCell><div className="flex flex-wrap gap-1.5">{value.members.map((member) => <Badge key={member.id} variant={member.enabled && member.authStatus === "active" ? "secondary" : "destructive"} title={member.name}>{providerLabel(member.provider)} · {member.name}</Badge>)}</div></TableCell>
                <TableCell className="text-xs"><div className="truncate font-medium" title={value.proxyName}>{value.proxyName || t("accountFamilies.unbound")}</div>{value.proxyName ? <div className="mt-0.5 text-muted-foreground">{value.proxyEnabled ? t("common.enabled") : t("common.disabled")}</div> : <div className="mt-0.5 text-muted-foreground">{t("accountFamilies.sharedEgress")}</div>}</TableCell>
                <TableCell><Badge variant={healthy ? "secondary" : "destructive"}>{healthy ? t("accountFamilies.normal") : t("accountFamilies.abnormal")}</Badge></TableCell>
                <TableCell className="whitespace-nowrap text-xs text-muted-foreground">{formatDateTime(value.createdAt, i18n.language)}</TableCell>
                <TableActionCell><Button variant="ghost" size="icon" className="size-8" aria-label={t("accountFamilies.bindProxy")} onClick={() => beginProxyEdit(value)}><Pencil /></Button></TableActionCell>
              </TableRow>;
            })}
          </TableBody>
        </Table>
      )}

      <Dialog open={editing !== null} onOpenChange={(open) => !open && setEditing(null)}>
        <DialogContent>
          <DialogHeader><DialogTitle>{t("accountFamilies.bindProxy")}</DialogTitle><DialogDescription>{t("accountFamilies.bindDescription", { name: primaryMember(editing?.members ?? [])?.name || editing?.id })}</DialogDescription></DialogHeader>
          <div className="space-y-2">
            <Label htmlFor="family-proxy">{t("accountFamilies.proxy")}</Label>
            <Select value={proxyID} onValueChange={setProxyID}>
              <SelectTrigger id="family-proxy"><SelectValue placeholder={t("accounts.selectProxy")} /></SelectTrigger>
              <SelectContent>
                <SelectItem value="none">{t("accountFamilies.unboundOption")}</SelectItem>
                {editing?.proxyId && !proxyOptionsQuery.data?.some((value) => value.id === editing.proxyId) ? <SelectItem value={editing.proxyId}>{editing.proxyName ?? editing.proxyId} ({t("common.disabled")})</SelectItem> : null}
                {proxyOptionsQuery.data?.map((value) => <SelectItem key={value.id} value={value.id}>{value.name} · {value.protocol.toUpperCase()} · {value.address}</SelectItem>)}
              </SelectContent>
            </Select>
            <p className="text-xs text-muted-foreground">{t("accountFamilies.proxyHint")}</p>
          </div>
          <DialogFooter><Button variant="secondary" size="sm" onClick={() => setEditing(null)}>{t("common.cancel")}</Button><Button size="sm" disabled={bindingMutation.isPending} onClick={() => bindingMutation.mutate()}>{bindingMutation.isPending ? <Spinner /> : null}{t("common.save")}</Button></DialogFooter>
        </DialogContent>
      </Dialog>
    </DataTableShell>
  );
}

// primaryMember 选择最适合作为逻辑账号标题的成员，优先 Web，其次 Console、Build。
function primaryMember(members: AccountFamilyMemberDTO[]): AccountFamilyMemberDTO | undefined {
  return members.find((member) => member.provider === "grok_web") ?? members.find((member) => member.provider === "grok_console") ?? members[0];
}

// providerLabel 返回逻辑账号成员的紧凑 Provider 标签。
function providerLabel(provider: AccountFamilyMemberDTO["provider"]): string {
  if (provider === "grok_web") return "Web";
  if (provider === "grok_console") return "Console";
  return "Build";
}

// showAccountFamilyError 将逻辑账号组请求错误显示为通知。
function showAccountFamilyError(error: unknown): void {
  toast.error(error instanceof Error ? error.message : "逻辑账号操作失败");
}
