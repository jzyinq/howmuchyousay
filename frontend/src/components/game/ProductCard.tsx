import { Card, CardContent } from "@/components/ui/card"
import { cn } from "@/lib/utils"
import type { ProductDTO } from "@/types"

interface ProductCardProps {
  product: ProductDTO
  label: string
  selected: boolean
  disabled: boolean
  isCorrectAnswer?: boolean
  isWrongPick?: boolean
  onClick: () => void
}

export default function ProductCard({
  product,
  label,
  selected,
  disabled,
  isCorrectAnswer,
  isWrongPick,
  onClick,
}: ProductCardProps) {
  return (
    <Card
      className={cn(
        "w-full max-w-xs cursor-pointer transition-all",
        selected && !isCorrectAnswer && !isWrongPick && "ring-4 ring-main translate-x-boxShadowX translate-y-boxShadowY shadow-none",
        isCorrectAnswer && "ring-4 ring-green-500 border-green-500",
        isWrongPick && "ring-4 ring-red-500 border-red-500 opacity-75",
        disabled && "cursor-default",
        !disabled && !selected && "hover:translate-x-boxShadowX hover:translate-y-boxShadowY hover:shadow-none",
      )}
      onClick={() => {
        if (!disabled) onClick()
      }}
    >
      <CardContent className="flex flex-col items-center gap-3 p-4">
        <span className="text-xs font-heading text-foreground/50 uppercase">
          {label}
        </span>
        <div className="w-full aspect-square overflow-hidden rounded-base border-2 border-border bg-background">
          <img
            src={product.image_url}
            alt={product.name}
            className="w-full h-full object-contain"
            onError={(e) => {
              e.currentTarget.src = "data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='200' height='200'%3E%3Crect width='200' height='200' fill='%23e5e5e5'/%3E%3Ctext x='50%25' y='50%25' text-anchor='middle' dy='.3em' fill='%23999' font-size='14'%3ENo Image%3C/text%3E%3C/svg%3E"
            }}
          />
        </div>
        <p className="text-center text-sm font-heading leading-tight">
          {product.name}
        </p>
        {isCorrectAnswer && (
          <p className="text-sm font-heading text-green-600">
            ✓ More expensive
          </p>
        )}
        {isWrongPick && (
          <p className="text-sm font-heading text-red-600">
            ✗ Not this one
          </p>
        )}
      </CardContent>
    </Card>
  )
}
