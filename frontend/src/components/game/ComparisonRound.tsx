import { useState } from "react"
import { Button } from "@/components/ui/button"
import ProductCard from "./ProductCard"
import type { ProductDTO, AnswerResponse } from "@/types"

interface ComparisonRoundProps {
  productA: ProductDTO
  productB: ProductDTO
  onSubmit: (answer: "a" | "b") => void
  isSubmitting: boolean
  result: AnswerResponse | null
}

export default function ComparisonRound({
  productA,
  productB,
  onSubmit,
  isSubmitting,
  result,
}: ComparisonRoundProps) {
  const [selected, setSelected] = useState<"a" | "b" | null>(null)
  const answered = result !== null

  return (
    <div className="flex flex-col items-center gap-6">
      <h2 className="text-2xl font-heading text-center">
        Which product is more expensive?
      </h2>
      <div className="flex flex-col sm:flex-row gap-6 items-center sm:items-start">
        <ProductCard
          product={productA}
          label="Product A"
          selected={selected === "a"}
          disabled={answered}
          isCorrectAnswer={answered && result.correct_answer === "a"}
          isWrongPick={answered && selected === "a" && result.correct_answer !== "a"}
          revealedPrice={answered ? result.price_a : undefined}
          onClick={() => setSelected("a")}
        />
        <div className="flex items-center text-2xl font-heading text-foreground/30 self-center">
          VS
        </div>
        <ProductCard
          product={productB}
          label="Product B"
          selected={selected === "b"}
          disabled={answered}
          isCorrectAnswer={answered && result.correct_answer === "b"}
          isWrongPick={answered && selected === "b" && result.correct_answer !== "b"}
          revealedPrice={answered ? result.price_b : undefined}
          onClick={() => setSelected("b")}
        />
      </div>
      {!answered && (
        <Button
          size="lg"
          disabled={selected === null || isSubmitting}
          onClick={() => {
            if (selected) onSubmit(selected)
          }}
        >
          {isSubmitting ? "Submitting..." : "Lock In Answer"}
        </Button>
      )}
    </div>
  )
}
