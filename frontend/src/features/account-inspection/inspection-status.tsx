import { ShieldAlert, ShieldCheck, TriangleAlert, UserRoundCheck, WalletCards } from "lucide-react";
import { useTranslation } from "react-i18next";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { cn } from "@/shared/lib/cn";

import type { InspectionClassification, InspectionOverviewDTO, InspectionState, InspectionSuggestion } from "@/features/account-inspection/inspection-api";

export type InspectionCategory = "all" | "healthy" | "permissionDenied" | "quotaExhausted" | "reauthRequired" | "exception";

type CategoryCardProps = {
  category: InspectionCategory;
  label: string;
  value: number;
  active: boolean;
  onClick: () => void;
};

const categoryIcons = {
  all: ShieldCheck,
  healthy: ShieldCheck,
  permissionDenied: ShieldAlert,
  quotaExhausted: WalletCards,
  reauthRequired: UserRoundCheck,
  exception: TriangleAlert,
} as const;

// InspectionCategoryCards 展示并切换巡检结果分类，与截图中的统计卡行为保持一致。
// 参数 overview 为巡检统计，active 为当前分类，onChange 接收分类切换；返回统计卡区域。
export function InspectionCategoryCards({ overview, active, onChange }: { overview: InspectionOverviewDTO; active: InspectionCategory; onChange: (value: InspectionCategory) => void }) {
  const { t } = useTranslation();
  const cards: Array<Omit<CategoryCardProps, "active" | "onClick">> = [
    { category: "all", label: t("accountInspection.categories.all"), value: overview.total },
    { category: "healthy", label: t("accountInspection.categories.healthy"), value: overview.healthy },
    { category: "permissionDenied", label: t("accountInspection.categories.permissionDenied"), value: overview.permissionDenied },
    { category: "quotaExhausted", label: t("accountInspection.categories.quotaExhausted"), value: overview.quotaExhausted },
    { category: "reauthRequired", label: t("accountInspection.categories.reauthRequired"), value: overview.reauthRequired },
    { category: "exception", label: t("accountInspection.categories.exception"), value: overview.exception },
  ];
  return (
    <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-6">
      {cards.map((card) => <CategoryCard key={card.category} {...card} active={active === card.category} onClick={() => onChange(card.category)} />)}
    </div>
  );
}

// CategoryCard 显示一个可点击的巡检分类统计值。
// 参数 props 为分类名称、数量和选择状态；返回统计按钮。
function CategoryCard({ category, label, value, active, onClick }: CategoryCardProps) {
  const Icon = categoryIcons[category];
  return (
    <Button type="button" variant="outline" className={cn("h-auto min-h-28 items-start justify-start rounded-2xl border px-4 py-4 text-left shadow-sm", active && "border-primary ring-2 ring-primary/20")} onClick={onClick}>
      <span className="flex w-full items-start justify-between gap-3"><span><span className="block text-xs text-muted-foreground">{label}</span><span className="mt-3 block text-3xl font-semibold tabular-nums">{value}</span></span><Icon className="mt-0.5 size-4 text-muted-foreground" /></span>
    </Button>
  );
}

// InspectionResultBadge 展示单条巡检结论的颜色和文字。
// 参数 state 为巡检状态，classification 为细分类；返回结果徽标。
export function InspectionResultBadge({ state, classification }: { state: InspectionState; classification: InspectionClassification }) {
  const { t } = useTranslation();
  const variant = state === "unavailable" ? "destructive" : state === "healthy" ? "secondary" : "outline";
  const className = state === "healthy" ? "bg-emerald-500/10 text-emerald-700 dark:text-emerald-300" : state === "uncertain" ? "border-amber-500/30 bg-amber-500/10 text-amber-700 dark:text-amber-300" : undefined;
  return <Badge variant={variant} className={className}>{t(`accountInspection.classifications.${classification}`)}</Badge>;
}

// InspectionSuggestionBadge 展示巡检后的人工操作建议。
// 参数 suggestion 为建议操作；返回建议徽标。
export function InspectionSuggestionBadge({ suggestion }: { suggestion: InspectionSuggestion }) {
  const { t } = useTranslation();
  const variant = suggestion === "disable" ? "destructive" : suggestion === "keep" ? "secondary" : "outline";
  return <Badge variant={variant} className={suggestion === "keep" ? "bg-emerald-500/10 text-emerald-700 dark:text-emerald-300" : undefined}>{t(`accountInspection.suggestions.${suggestion}`)}</Badge>;
}
