import { Ban, Trash2 } from "lucide-react";
import { useTranslation } from "react-i18next";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";

import { InspectionResultBadge, InspectionSuggestionBadge } from "@/features/account-inspection/inspection-status";
import type { InspectionItemDTO } from "@/features/account-inspection/inspection-api";

type InspectionTableProps = {
  rows: InspectionItemDTO[];
  selected: Set<string>;
  onSelectedChange: (value: Set<string>) => void;
  onDisable: (id: string) => void;
  onDelete: (id: string) => void;
  pending: boolean;
};

// InspectionTable 按截图列结构展示 Build 账号状态、结果、建议与逐项操作。
// 参数 props 为列表数据、选择状态和操作回调；返回巡检结果表格。
export function InspectionTable({ rows, selected, onSelectedChange, onDisable, onDelete, pending }: InspectionTableProps) {
  const { t } = useTranslation();
  const selectableIDs = rows.map((item) => item.accountId);
  const allSelected = selectableIDs.length > 0 && selectableIDs.every((id) => selected.has(id));
  const toggleAll = (checked: boolean) => onSelectedChange(checked ? new Set(selectableIDs) : new Set());
  const toggleOne = (id: string, checked: boolean) => {
    const next = new Set(selected);
    if (checked) next.add(id);
    else next.delete(id);
    onSelectedChange(next);
  };

  return (
    <div className="overflow-hidden rounded-2xl border bg-card">
      <Table className="min-w-[1030px]">
        <TableHeader className="bg-muted/45">
          <TableRow className="hover:bg-transparent">
            <TableHead className="w-11"><Checkbox checked={allSelected} onCheckedChange={(checked) => toggleAll(checked === true)} aria-label={t("accountInspection.selectAll")} /></TableHead>
            <TableHead>{t("accountInspection.table.account")}</TableHead>
            <TableHead>{t("accountInspection.table.currentStatus")}</TableHead>
            <TableHead>{t("accountInspection.table.result")}</TableHead>
            <TableHead>{t("accountInspection.table.http")}</TableHead>
            <TableHead>{t("accountInspection.table.model")}</TableHead>
            <TableHead>{t("accountInspection.table.suggestion")}</TableHead>
            <TableHead>{t("accountInspection.table.reason")}</TableHead>
            <TableHead className="text-right">{t("common.actions")}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {rows.map((item) => (
            <TableRow key={item.accountId}>
              <TableCell><Checkbox checked={selected.has(item.accountId)} onCheckedChange={(checked) => toggleOne(item.accountId, checked === true)} aria-label={item.name} /></TableCell>
              <TableCell className="font-medium">{item.name}<span className="ml-2 font-mono text-xs text-muted-foreground">#{item.accountId}</span></TableCell>
              <TableCell><Badge variant={item.enabled ? "secondary" : "outline"} className={item.enabled ? "bg-emerald-500/10 text-emerald-700 dark:text-emerald-300" : undefined}>{item.enabled ? t("accountInspection.enabled") : t("accountInspection.disabled")}</Badge></TableCell>
              <TableCell><InspectionResultBadge state={item.state} classification={item.classification} /></TableCell>
              <TableCell className="font-mono text-sm">{item.httpStatus || "—"}</TableCell>
              <TableCell className="font-mono text-sm">{item.model ?? "—"}</TableCell>
              <TableCell><InspectionSuggestionBadge suggestion={item.suggestion} /></TableCell>
              <TableCell className="max-w-64 text-xs text-muted-foreground">{item.reason}</TableCell>
              <TableCell>
                <div className="flex justify-end gap-2">
                  <Button variant="outline" size="sm" disabled={!item.enabled || pending} onClick={() => onDisable(item.accountId)}><Ban />{t("accountInspection.disable")}</Button>
                  <Button variant="outline" size="sm" className="border-destructive/30 text-destructive hover:bg-destructive/10 hover:text-destructive" disabled={pending} onClick={() => onDelete(item.accountId)}><Trash2 />{t("common.delete")}</Button>
                </div>
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  );
}
