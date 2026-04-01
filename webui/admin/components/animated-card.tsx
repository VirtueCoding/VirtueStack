"use client";

import { motion } from "motion/react";
import { Card } from "@virtuestack/ui";
import { cn } from "@/lib/utils";
import { forwardRef } from "react";
import { easeTransition } from "@/lib/animations";

interface AnimatedCardProps extends React.ComponentProps<typeof Card> {
  hoverLift?: boolean;
  delay?: number;
}

export const AnimatedCard = forwardRef<HTMLDivElement, AnimatedCardProps>(
  ({ className, hoverLift = true, delay = 0, children, ...props }, ref) => {
    return (
      <motion.div
        initial={{ opacity: 0, y: 8 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ ...easeTransition, delay }}
        whileHover={
          hoverLift
            ? {
                y: -2,
                transition: { duration: 0.2 },
              }
            : undefined
        }
      >
        <Card
          ref={ref}
          className={cn(
            "transition-shadow duration-200 hover:shadow-md",
            className
          )}
          {...props}
        >
          {children}
        </Card>
      </motion.div>
    );
  }
);
AnimatedCard.displayName = "AnimatedCard";
