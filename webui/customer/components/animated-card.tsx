"use client";

import { motion } from "motion/react";
import { Card } from "@virtuestack/ui";
import { cn } from "@/lib/utils";
import { forwardRef } from "react";
import { easeTransition } from "@/lib/animations";

const MotionCard = motion.create(Card);

interface AnimatedCardProps extends React.ComponentProps<typeof Card> {
  hoverLift?: boolean;
  delay?: number;
}

export const AnimatedCard = forwardRef<HTMLDivElement, AnimatedCardProps>(
  ({ className, hoverLift = true, delay = 0, children, ...props }, ref) => {
    return (
      <MotionCard
        ref={ref}
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
        className={cn(
          "transition-shadow duration-200 hover:shadow-md",
          className
        )}
        {...props}
      >
        {children}
      </MotionCard>
    );
  }
);
AnimatedCard.displayName = "AnimatedCard";
