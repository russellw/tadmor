import { formatAmount, isZeroAmount } from "@/lib/amount"
import { TableCell } from "@/components/ui/table"
import { cn } from "@/lib/utils"

// Right-aligned numeric table cell for the exact amount strings the reporting
// endpoints return. Zero amounts render muted so the nonzero figures stand out.
export function AmountCell({
  value,
  className,
}: {
  value: string
  className?: string
}) {
  return (
    <TableCell
      className={cn(
        "text-right font-mono tabular-nums",
        isZeroAmount(value) && "text-muted-foreground",
        className,
      )}
    >
      {formatAmount(value)}
    </TableCell>
  )
}
