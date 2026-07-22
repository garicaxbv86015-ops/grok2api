import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { MoreHorizontal, Pencil, Play, Plus, Search, Trash2, Zap } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";

import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from "@/components/ui/alert-dialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from "@/components/ui/dropdown-menu";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Spinner } from "@/components/ui/spinner";
import { Switch } from "@/components/ui/switch";
import { Table, TableActionCell, TableActionHead, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { EmptyState, ErrorState, TableLoadingRow } from "@/shared/components/data-state";
import { DataTableShell } from "@/shared/components/data-table-shell";
import { PageHeader } from "@/shared/components/page-header";
import { Pagination } from "@/shared/components/pagination";
import { SortableTableHead } from "@/shared/components/sortable-table-head";
import { useDebouncedValue } from "@/shared/hooks/use-debounced-value";
import { formatDateTime } from "@/shared/lib/format";
import { nextTableSort, type SortOrder, type TableSort } from "@/shared/lib/table-sort";
import { createProxy, deleteProxy, listProxies, testAllProxyConnections, testProxyConnection, updateProxy, type ProxyDTO, type ProxyInput } from "@/features/proxies/proxies-api";

type ProxyEditorState = {
  current: ProxyDTO | null;
  name: string;
  proxyURL: string;
  enabled: boolean;
};

// ProxiesPage 展示并维护可供逻辑账号组复用的固定代理。
export function ProxiesPage() {
  const { t, i18n } = useTranslation();
  const queryClient = useQueryClient();
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [search, setSearch] = useState("");
  const [protocol, setProtocol] = useState("all");
  const [status, setStatus] = useState("all");
  const [sort, setSort] = useState<TableSort>({ field: "createdAt", order: "desc" });
  const [editor, setEditor] = useState<ProxyEditorState | null>(null);
  const [deleting, setDeleting] = useState<ProxyDTO | null>(null);
  const debouncedSearch = useDebouncedValue(search);

  const proxiesQuery = useQuery({
    queryKey: ["proxies", page, pageSize, debouncedSearch, protocol, status, sort.field, sort.order],
    queryFn: () => listProxies({
      page, pageSize, search: debouncedSearch, protocol: protocol === "all" ? undefined : protocol,
      enabled: status === "all" ? undefined : status === "enabled", sortBy: sort.field, sortOrder: sort.order,
    }),
  });

  const saveMutation = useMutation({
    mutationFn: async (input: ProxyInput) => editor?.current ? updateProxy(editor.current.id, input) : createProxy(input),
    onSuccess: () => {
      setEditor(null);
      void queryClient.invalidateQueries({ queryKey: ["proxies"] });
      toast.success(t("proxies.saved"));
    },
    onError: showProxyError,
  });
  const deleteMutation = useMutation({
    mutationFn: deleteProxy,
    onSuccess: () => {
      setDeleting(null);
      void queryClient.invalidateQueries({ queryKey: ["proxies"] });
      toast.success(t("proxies.deleted"));
    },
    onError: showProxyError,
  });
  const testMutation = useMutation({
    mutationFn: testProxyConnection,
    onSuccess: (result) => {
      void queryClient.invalidateQueries({ queryKey: ["proxies"] });
      if (result.ok) toast.success(t("proxies.testSucceeded", { latency: result.latencyMS ?? 0 }));
      else toast.error(result.error || t("proxies.testFailed"));
    },
    onError: showProxyError,
  });
  const testAllMutation = useMutation({
    mutationFn: testAllProxyConnections,
    onSuccess: (result) => {
      void queryClient.invalidateQueries({ queryKey: ["proxies"] });
      if (result.total === 0) {
        toast.message(t("proxies.testAllEmpty"));
        return;
      }
      if (result.failed === 0) {
        toast.success(t("proxies.testAllAllSucceeded", { total: result.total }));
        return;
      }
      toast.warning(t("proxies.testAllCompleted", { succeeded: result.succeeded, failed: result.failed }));
    },
    onError: showProxyError,
  });

  const result = proxiesQuery.data;

  // handleSort 更新代理表格的稳定排序状态。
  function handleSort(field: string, initialOrder: SortOrder): void {
    setSort((current) => nextTableSort(current, field, initialOrder));
  }

  return (
    <div className="space-y-8">
      <PageHeader title={t("proxies.title")} description={t("proxies.description")} actions={<Button size="sm" onClick={() => setEditor({ current: null, name: "", proxyURL: "", enabled: true })}><Plus />{t("proxies.add")}</Button>} />
      <DataTableShell
        toolbar={<>
          <div className="flex w-full flex-wrap items-center gap-2 sm:w-auto">
            <div className="relative min-w-56 flex-1 sm:w-64 sm:flex-none">
              <Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
              <Input className="h-8 pl-9 text-xs" value={search} onChange={(event) => { setSearch(event.target.value); setPage(1); }} placeholder={t("proxies.search")} />
            </div>
            <Select value={protocol} onValueChange={(value) => { setProtocol(value); setPage(1); }}>
              <SelectTrigger className="w-32"><SelectValue /></SelectTrigger>
              <SelectContent><SelectItem value="all">{t("proxies.allProtocols")}</SelectItem><SelectItem value="http">HTTP</SelectItem><SelectItem value="https">HTTPS</SelectItem><SelectItem value="socks5">SOCKS5</SelectItem><SelectItem value="socks5h">SOCKS5H</SelectItem></SelectContent>
            </Select>
            <Select value={status} onValueChange={(value) => { setStatus(value); setPage(1); }}>
              <SelectTrigger className="w-28"><SelectValue /></SelectTrigger>
              <SelectContent><SelectItem value="all">{t("common.all")}</SelectItem><SelectItem value="enabled">{t("common.enabled")}</SelectItem><SelectItem value="disabled">{t("common.disabled")}</SelectItem></SelectContent>
            </Select>
          </div>
          <div className="flex items-center gap-2">
            <Button
              variant="secondary"
              size="sm"
              onClick={() => testAllMutation.mutate()}
              disabled={testAllMutation.isPending}
            >
              {testAllMutation.isPending ? <Spinner /> : <Zap />}
              {testAllMutation.isPending ? t("proxies.testingAll") : t("proxies.testAll")}
            </Button>
            <Button variant="secondary" size="sm" onClick={() => void proxiesQuery.refetch()} disabled={proxiesQuery.isFetching}>{proxiesQuery.isFetching ? <Spinner /> : null}{t("common.refresh")}</Button>
          </div>
        </>}
        footer={<Pagination page={page} pageSize={pageSize} total={result?.total ?? 0} onPageChange={setPage} onPageSizeChange={(value) => { setPageSize(value); setPage(1); }} />}
      >
        {proxiesQuery.isError ? <ErrorState message={t("proxies.loadFailed")} onRetry={() => void proxiesQuery.refetch()} /> : !proxiesQuery.isPending && result?.items.length === 0 ? <EmptyState message={t("proxies.empty")} /> : (
          <Table>
            <TableHeader><TableRow>
              <SortableTableHead field="name" sortBy={sort.field} sortOrder={sort.order} onSort={handleSort}>{t("proxies.name")}</SortableTableHead>
              <TableHead>{t("proxies.protocol")}</TableHead><TableHead>{t("proxies.address")}</TableHead><TableHead>{t("proxies.authentication")}</TableHead><TableHead>{t("proxies.status")}</TableHead><TableHead>{t("proxies.boundAccounts")}</TableHead><TableHead>{t("proxies.lastTest")}</TableHead>
              <SortableTableHead field="createdAt" sortBy={sort.field} sortOrder={sort.order} initialOrder="desc" onSort={handleSort}>{t("proxies.createdAt")}</SortableTableHead>
              <TableActionHead />
            </TableRow></TableHeader>
            <TableBody>
              {proxiesQuery.isPending ? <TableLoadingRow colSpan={9} /> : result?.items.map((value) => <TableRow key={value.id}>
                <TableCell className="text-xs font-medium">{value.name}</TableCell>
                <TableCell><Badge variant="secondary">{value.protocol.toUpperCase()}</Badge></TableCell>
                <TableCell className="font-mono text-xs text-muted-foreground">{value.address}</TableCell>
                <TableCell className="text-xs text-muted-foreground">{value.authConfigured ? t("proxies.configured") : t("proxies.none")}</TableCell>
                <TableCell><Badge variant={value.enabled ? "default" : "secondary"}>{value.enabled ? t("common.enabled") : t("common.disabled")}</Badge></TableCell>
                <TableCell className="text-xs">{value.boundFamilyCount}</TableCell>
                <TableCell className="text-xs text-muted-foreground">{value.lastTestAt ? (value.lastTestOK ? t("proxies.testOK", { latency: value.lastLatencyMS ?? 0 }) : t("proxies.testFailed")) : t("proxies.notTested")}</TableCell>
                <TableCell className="whitespace-nowrap text-xs text-muted-foreground">{formatDateTime(value.createdAt, i18n.language)}</TableCell>
                <TableActionCell><DropdownMenu><DropdownMenuTrigger asChild><Button variant="ghost" size="icon" className="size-8"><MoreHorizontal /></Button></DropdownMenuTrigger><DropdownMenuContent align="end">
                  <DropdownMenuItem onClick={() => testMutation.mutate(value.id)}><Play />{t("proxies.test")}</DropdownMenuItem>
                  <DropdownMenuItem onClick={() => setEditor({ current: value, name: value.name, proxyURL: "", enabled: value.enabled })}><Pencil />{t("common.edit")}</DropdownMenuItem>
                  <DropdownMenuSeparator /><DropdownMenuItem className="text-destructive focus:text-destructive" onClick={() => setDeleting(value)}><Trash2 />{t("common.delete")}</DropdownMenuItem>
                </DropdownMenuContent></DropdownMenu></TableActionCell>
              </TableRow>)}
            </TableBody>
          </Table>
        )}
      </DataTableShell>

      <Dialog open={editor !== null} onOpenChange={(open) => !open && setEditor(null)}>
        <DialogContent><DialogHeader><DialogTitle>{editor?.current ? t("proxies.edit") : t("proxies.add")}</DialogTitle><DialogDescription>{t(editor?.current ? "proxies.editDescription" : "proxies.createDescription")}</DialogDescription></DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2"><Label htmlFor="proxy-name">{t("proxies.name")}</Label><Input id="proxy-name" value={editor?.name ?? ""} onChange={(event) => setEditor((current) => current ? { ...current, name: event.target.value } : current)} /></div>
            <div className="space-y-2"><Label htmlFor="proxy-url">{t("proxies.proxyURL")}</Label><Input id="proxy-url" type="password" autoComplete="new-password" value={editor?.proxyURL ?? ""} onChange={(event) => setEditor((current) => current ? { ...current, proxyURL: event.target.value } : current)} placeholder={editor?.current ? t("proxies.keepConfigured") : "socks5://user:password@127.0.0.1:1080"} /><p className="text-xs text-muted-foreground">{t("proxies.proxyURLHint")}</p></div>
            <div className="flex items-center justify-between rounded-md border px-3 py-2"><Label htmlFor="proxy-enabled">{t("proxies.enabled")}</Label><Switch id="proxy-enabled" checked={editor?.enabled ?? false} onCheckedChange={(checked) => setEditor((current) => current ? { ...current, enabled: checked } : current)} /></div>
          </div>
          <DialogFooter><Button variant="secondary" size="sm" onClick={() => setEditor(null)}>{t("common.cancel")}</Button><Button size="sm" disabled={saveMutation.isPending || !editor?.name.trim() || (!editor?.current && !editor?.proxyURL.trim())} onClick={() => {
            if (!editor) return;
            const input: ProxyInput = { name: editor.name.trim(), enabled: editor.enabled };
            if (editor.proxyURL.trim()) input.proxyURL = editor.proxyURL.trim();
            saveMutation.mutate(input);
          }}>{saveMutation.isPending ? <Spinner /> : null}{t("common.save")}</Button></DialogFooter>
        </DialogContent>
      </Dialog>

      <AlertDialog open={deleting !== null} onOpenChange={(open) => !open && setDeleting(null)}><AlertDialogContent><AlertDialogHeader><AlertDialogTitle>{t("proxies.deleteTitle")}</AlertDialogTitle><AlertDialogDescription>{t("proxies.deleteDescription", { name: deleting?.name, count: deleting?.boundFamilyCount ?? 0 })}</AlertDialogDescription></AlertDialogHeader><AlertDialogFooter><AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel><AlertDialogAction disabled={deleteMutation.isPending || (deleting?.boundFamilyCount ?? 0) > 0} onClick={() => deleting && deleteMutation.mutate(deleting.id)}>{t("common.delete")}</AlertDialogAction></AlertDialogFooter></AlertDialogContent></AlertDialog>
    </div>
  );
}

// showProxyError 将代理管理请求错误显示为通知。
function showProxyError(error: unknown): void {
  toast.error(error instanceof Error ? error.message : "代理操作失败");
}
