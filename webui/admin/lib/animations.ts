import type { Variants } from "motion/react";

export const fadeUp: Variants = {
  hidden: { opacity: 0, y: 8 },
  visible: { opacity: 1, y: 0 },
};

export const staggerContainer: Variants = {
  hidden: { opacity: 0 },
  visible: {
    opacity: 1,
    transition: {
      staggerChildren: 0.05,
      delayChildren: 0.1,
    },
  },
};

export const easeTransition = {
  duration: 0.3,
  ease: [0.25, 0.1, 0.25, 1] as const,
};
