import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Pencil, RefreshCw, Search, Trash2 } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";

import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from "@/components/ui/alert-dialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Spinner } from "@/components/ui/spinner";
import { Table, TableActionCell, TableActionHead, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { AccountFamilyProxyDialog } from "@/features/accounts/account-family-proxy-dialog";
import { batchUpdateAccountFamilyProxy, deleteAccountFamily, listAccountFamilies, updateAccountFamilyProxy, type AccountFamilyDTO, type AccountFamilyMemberDTO, type AccountFamilyProxyInput } from "@/features/accounts/account-families-api";
import { EmptyState, ErrorState, TableLoadingRow } from "@/shared/components/data-state";
import { DataTableShell } from "@/shared/components/data-table-shell";
import { Pagination } from "@/shared/components/pagination";
import { useDebouncedValue } from "@/shared/hooks/use-debounced-value";
import { formatDateTime } from "@/shared/lib/format";

type ProxyBindingFilter = "all" | "bound" | "unbound";
type BindingTarget = { kind: "single"; family: AccountFamilyDTO } | { kind: "batch"; ids: string[] };

// AccountFamiliesPanel 展示逻辑账号组；无参数；返回带搜索、筛选和批量绑定的面板。
export function AccountFamiliesPanel() {
  const { t, i18n } = useTranslation();
  const queryClient = useQueryClient();
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [search, setSearch] = useState("");
  const [proxyBinding, setProxyBinding] = useState<ProxyBindingFilter>("all");
  const [selected, setSelected] = useState<Set<string>>(() => new Set());
  const [bindingTarget, setBindingTarget] = useState<BindingTarget | null>(null);
  const [deleting, setDeleting] = useState<AccountFamilyDTO | null>(null);
  const debouncedSearch = useDebouncedValue(search);

  const familiesQuery = useQuery({
    queryKey: ["account-families", page, pageSize, debouncedSearch, proxyBinding],
    queryFn: () => listAccountFamilies({ page, pageSize, search: debouncedSearch, proxyBinding: proxyBinding === "all" ? undefined : proxyBinding }),
  });
  const bindingMutation = useMutation({
    mutationFn: (request: { target: BindingTarget; input: AccountFamilyProxyInput }) => updateBindingTarget(request.target, request.input),
    onSuccess: (updated, request) => {
      setBindingTarget(null);
      setSelected(new Set());
      refreshAccountFamilyCaches();
      toast.success(request.target.kind === "batch" ? t("accountFamilies.batchUpdated", { count: updated }) : t("accountFamilies.proxyUpdated"));
    },
    onError: showAccountFamilyError,
  });
  const deleteMutation = useMutation({
    mutationFn: (id: string) => deleteAccountFamily(id),
    onSuccess: (result) => {
      setDeleting(null);
      setSelected(new Set());
      refreshAccountFamilyCaches();
      toast.success(t("accountFamilies.deleted", { count: result.deleted }));
    },
    onError: showAccountFamilyError,
  });
  const result = familiesQuery.data;
  const pageIDs = result?.items.map((value) => value.id) ?? [];
  const selectedOnPage = pageIDs.filter((id) => selected.has(id));
  const allPageSelected = pageIDs.length > 0 && selectedOnPage.length === pageIDs.length;

  // refreshAccountFamilyCaches 刷新逻辑账号、三个 Provider 账号和代理列表缓存；无参数；无返回值。
  function refreshAccountFamilyCaches(): void {
    void queryClient.invalidateQueries({ queryKey: ["account-families"] });
    void queryClient.invalidateQueries({ queryKey: ["accounts"] });
    void queryClient.invalidateQueries({ queryKey: ["proxies"] });
  }

  // beginProxyEdit 打开单个账号组代理编辑器；value 为目标账号组；无返回值。
  function beginProxyEdit(value: AccountFamilyDTO): void {
    setBindingTarget({ kind: "single", family: value });
  }

  // beginBatchProxyEdit 打开当前页勾选账号组的批量编辑器；无参数；无返回值。
  function beginBatchProxyEdit(): void {
    if (selectedOnPage.length === 0) return;
    setBindingTarget({ kind: "batch", ids: selectedOnPage });
  }

  // submitProxyBinding 提交代理绑定；input 为绑定或解绑指令；无返回值。
  function submitProxyBinding(input: AccountFamilyProxyInput): void {
    if (!bindingTarget) return;
    bindingMutation.mutate({ target: bindingTarget, input });
  }

  // togglePage 切换当前页全选；checked 表示是否选中；无返回值。
  function togglePage(checked: boolean): void {
    setSelected(checked ? new Set(pageIDs) : new Set());
  }

  // toggleFamily 切换单个账号组选中状态；id 为组标识，checked 为选中状态；无返回值。
  function toggleFamily(id: string, checked: boolean): void {
    setSelected((current) => {
      const next = new Set(current);
      if (checked) next.add(id);
      else next.delete(id);
      return next;
    });
  }

  // changeSearch 修改搜索词；value 为新搜索词；无返回值。
  function changeSearch(value: string): void {
    setSearch(value);
    setPage(1);
    setSelected(new Set());
  }

  // changeProxyBinding 修改代理绑定筛选；value 为新筛选值；无返回值。
  function changeProxyBinding(value: ProxyBindingFilter): void {
    setProxyBinding(value);
    setPage(1);
    setSelected(new Set());
  }

  // changePage 修改页码；value 为新页码；无返回值。
  function changePage(value: number): void {
    setPage(value);
    setSelected(new Set());
  }

  // changePageSize 修改每页数量；value 为新每页数；无返回值。
  function changePageSize(value: number): void {
    setPageSize(value);
    setPage(1);
    setSelected(new Set());
  }

  return (
    <DataTableShell
      toolbar={<>
        <div className="relative min-w-56 flex-1 sm:w-72 sm:flex-none">
          <Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input className="h-8 pl-9 text-xs" value={search} onChange={(event) => changeSearch(event.target.value)} placeholder={t("accountFamilies.search")} />
        </div>
        <Select value={proxyBinding} onValueChange={(value) => changeProxyBinding(value as ProxyBindingFilter)}>
          <SelectTrigger className="h-8 w-36 text-xs"><SelectValue /></SelectTrigger>
          <SelectContent><SelectItem value="all">{t("accountFamilies.allBindings")}</SelectItem><SelectItem value="bound">{t("accountFamilies.boundOnly")}</SelectItem><SelectItem value="unbound">{t("accountFamilies.unboundOnly")}</SelectItem></SelectContent>
        </Select>
        <div className="ml-auto flex flex-wrap items-center gap-1.5">
          {selectedOnPage.length > 0 ? <><span className="mr-1 text-xs text-muted-foreground">{t("common.selectedCount", { count: selectedOnPage.length })}</span><Button variant="secondary" size="sm" onClick={beginBatchProxyEdit}>{t("accountFamilies.batchBindProxy")}</Button></> : null}
          <Button variant="secondary" size="sm" disabled={familiesQuery.isFetching} onClick={() => { setSelected(new Set()); void familiesQuery.refetch(); }}>{familiesQuery.isFetching ? <Spinner /> : <RefreshCw />}{t("common.refresh")}</Button>
        </div>
      </>}
      footer={result && result.total > 0 ? <Pagination page={page} pageSize={pageSize} total={result.total} onPageChange={changePage} onPageSizeChange={changePageSize} /> : undefined}
    >
      {familiesQuery.isError ? <ErrorState message={t("accountFamilies.loadFailed")} onRetry={() => void familiesQuery.refetch()} /> : !familiesQuery.isPending && result?.items.length === 0 ? <EmptyState message={t("accountFamilies.empty")} /> : (
        <Table className="min-w-[920px] table-fixed">
          <colgroup><col className="w-[4%]" /><col className="w-[20%]" /><col className="w-[30%]" /><col className="w-[17%]" /><col className="w-[10%]" /><col className="w-[12%]" /><col className="w-[7%]" /></colgroup>
          <TableHeader><TableRow>
            <TableHead className="px-2 text-center"><Checkbox checked={allPageSelected ? true : selectedOnPage.length > 0 ? "indeterminate" : false} onCheckedChange={(checked) => togglePage(checked === true)} aria-label={t("common.selectPage")} /></TableHead><TableHead>{t("accountFamilies.logicalAccount")}</TableHead><TableHead>{t("accountFamilies.members")}</TableHead><TableHead>{t("accountFamilies.proxy")}</TableHead><TableHead>{t("accountFamilies.status")}</TableHead><TableHead>{t("accountFamilies.createdAt")}</TableHead><TableActionHead className="w-20 min-w-20" />
          </TableRow></TableHeader>
          <TableBody>
            {familiesQuery.isPending ? <TableLoadingRow colSpan={7} /> : result?.items.map((value) => {
              const primary = primaryMember(value.members);
              const healthy = value.members.every((member) => member.enabled && member.authStatus === "active");
              return <TableRow key={value.id} data-state={selected.has(value.id) ? "selected" : undefined}>
                <TableCell className="px-2 text-center"><Checkbox checked={selected.has(value.id)} onCheckedChange={(checked) => toggleFamily(value.id, checked === true)} aria-label={t("common.selectItem", { name: primary?.name || value.id })} /></TableCell>
                <TableCell className="min-w-0"><div className="truncate text-xs font-medium" title={primary?.name}>{primary?.name || t("accountFamilies.unnamed")}</div><div className="mt-0.5 truncate text-xs text-muted-foreground" title={primary?.email}>{primary?.email || t("accountFamilies.familyID", { id: value.id })}</div></TableCell>
                <TableCell><div className="flex flex-wrap gap-1.5">{value.members.map((member) => <Badge key={member.id} variant={member.enabled && member.authStatus === "active" ? "secondary" : "destructive"} title={member.name}>{providerLabel(member.provider)} · {member.name}</Badge>)}</div></TableCell>
                <TableCell className="text-xs"><div className="truncate font-medium" title={value.proxyName}>{value.proxyName || t("accountFamilies.unbound")}</div>{value.proxyName ? <div className="mt-0.5 text-muted-foreground">{value.proxyEnabled ? t("common.enabled") : t("common.disabled")}</div> : <div className="mt-0.5 text-muted-foreground">{t("accountFamilies.sharedEgress")}</div>}</TableCell>
                <TableCell><Badge variant={healthy ? "secondary" : "destructive"}>{healthy ? t("accountFamilies.normal") : t("accountFamilies.abnormal")}</Badge></TableCell>
                <TableCell className="whitespace-nowrap text-xs text-muted-foreground">{formatDateTime(value.createdAt, i18n.language)}</TableCell>
                <TableActionCell className="w-20 min-w-20"><div className="flex items-center"><Button variant="ghost" size="icon" className="size-8" aria-label={t("accountFamilies.bindProxy")} onClick={() => beginProxyEdit(value)}><Pencil /></Button><Button variant="ghost" size="icon" className="size-8 text-destructive hover:text-destructive" aria-label={t("accountFamilies.delete")} onClick={() => setDeleting(value)}><Trash2 /></Button></div></TableActionCell>
              </TableRow>;
            })}
          </TableBody>
        </Table>
      )}

      {bindingTarget ? <AccountFamilyProxyDialog
        key={bindingTarget.kind === "single" ? bindingTarget.family.id : bindingTarget.ids.join("-")}
        title={t(bindingTarget.kind === "single" ? "accountFamilies.bindProxy" : "accountFamilies.batchBindProxy")}
        description={bindingTarget.kind === "single" ? t("accountFamilies.bindDescription", { name: primaryMember(bindingTarget.family.members)?.name || bindingTarget.family.id }) : t("accountFamilies.batchBindDescription", { count: bindingTarget.ids.length })}
        initialProxyID={bindingTarget.kind === "single" ? bindingTarget.family.proxyId : undefined}
        initialProxyName={bindingTarget.kind === "single" ? bindingTarget.family.proxyName : undefined}
        pending={bindingMutation.isPending}
        onClose={() => setBindingTarget(null)}
        onSubmit={submitProxyBinding}
      /> : null}

      <AlertDialog open={Boolean(deleting)} onOpenChange={(open) => !open && setDeleting(null)}>
        <AlertDialogContent>
          <AlertDialogHeader><AlertDialogTitle>{t("accountFamilies.deleteTitle", { name: deleting ? primaryMember(deleting.members)?.name || deleting.id : "" })}</AlertDialogTitle><AlertDialogDescription>{t("accountFamilies.deleteDescription")}</AlertDialogDescription></AlertDialogHeader>
          <AlertDialogFooter><AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel><AlertDialogAction className="bg-destructive text-white hover:bg-destructive/90" disabled={deleteMutation.isPending} onClick={() => deleting && deleteMutation.mutate(deleting.id)}>{deleteMutation.isPending ? <Spinner /> : null}{t("common.delete")}</AlertDialogAction></AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </DataTableShell>
  );
}

// updateBindingTarget 提交代理绑定；target 为单条或批量目标，input 为绑定指令；返回更新数。
async function updateBindingTarget(target: BindingTarget, input: AccountFamilyProxyInput): Promise<number> {
  if (target.kind === "batch") {
    const result = await batchUpdateAccountFamilyProxy(target.ids, input);
    return result.updated;
  }
  await updateAccountFamilyProxy(target.family.id, input);
  return 1;
}

// primaryMember 选择逻辑账号标题；members 为组成员；返回优先 Web、其次 Console/Build 的成员。
function primaryMember(members: AccountFamilyMemberDTO[]): AccountFamilyMemberDTO | undefined {
  return members.find((member) => member.provider === "grok_web") ?? members.find((member) => member.provider === "grok_console") ?? members[0];
}

// providerLabel 生成紧凑标签；provider 为成员 Provider；返回 Web、Console 或 Build。
function providerLabel(provider: AccountFamilyMemberDTO["provider"]): string {
  if (provider === "grok_web") return "Web";
  if (provider === "grok_console") return "Console";
  return "Build";
}

// showAccountFamilyError 显示请求错误；error 为异常值；无返回值。
function showAccountFamilyError(error: unknown): void {
  toast.error(error instanceof Error ? error.message : "逻辑账号操作失败");
}
