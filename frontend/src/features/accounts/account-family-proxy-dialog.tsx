import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { useTranslation } from "react-i18next";

import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Spinner } from "@/components/ui/spinner";
import type { AccountFamilyProxyInput } from "@/features/accounts/account-families-api";
import { listProxyOptions } from "@/features/proxies/proxies-api";

type AccountFamilyProxyDialogProps = {
  title: string;
  description: string;
  initialProxyID?: string;
  initialProxyName?: string;
  pending: boolean;
  onClose: () => void;
  onSubmit: (input: AccountFamilyProxyInput) => void;
};

// AccountFamilyProxyDialog 提供代理选择表单；props 包含初始代理和提交回调；返回单条/批量共用对话框。
export function AccountFamilyProxyDialog(props: AccountFamilyProxyDialogProps) {
  const { t } = useTranslation();
  const [proxyID, setProxyID] = useState(props.initialProxyID ?? "none");
  const proxyOptionsQuery = useQuery({
    queryKey: ["proxies", "options"],
    queryFn: listProxyOptions,
  });

  // submitBinding 将当前 proxyID 转换为绑定或解绑请求；无参数；无返回值。
  function submitBinding(): void {
    props.onSubmit(proxyID === "none" ? { clearProxy: true } : { proxyId: proxyID });
  }

  return (
    <Dialog open onOpenChange={(open) => !open && props.onClose()}>
      <DialogContent>
        <DialogHeader><DialogTitle>{props.title}</DialogTitle><DialogDescription>{props.description}</DialogDescription></DialogHeader>
        <div className="space-y-2">
          <Label htmlFor="family-proxy">{t("accountFamilies.proxy")}</Label>
          <Select value={proxyID} onValueChange={setProxyID}>
            <SelectTrigger id="family-proxy"><SelectValue placeholder={t("accounts.selectProxy")} /></SelectTrigger>
            <SelectContent>
              <SelectItem value="none">{t("accountFamilies.unboundOption")}</SelectItem>
              {props.initialProxyID && !proxyOptionsQuery.data?.some((value) => value.id === props.initialProxyID) ? <SelectItem value={props.initialProxyID}>{props.initialProxyName ?? props.initialProxyID} ({t("common.disabled")})</SelectItem> : null}
              {proxyOptionsQuery.data?.map((value) => <SelectItem key={value.id} value={value.id}>{value.name} · {value.protocol.toUpperCase()} · {value.address}</SelectItem>)}
            </SelectContent>
          </Select>
          <p className="text-xs text-muted-foreground">{t("accountFamilies.proxyHint")}</p>
        </div>
        <DialogFooter><Button variant="secondary" size="sm" onClick={props.onClose}>{t("common.cancel")}</Button><Button size="sm" disabled={props.pending} onClick={submitBinding}>{props.pending ? <Spinner /> : null}{t("common.save")}</Button></DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
