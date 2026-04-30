import type { Variants } from "motion/react";

export const fadeUp: Variants = {
  hidden: { opacity: 0, y: 8 },
  visible: { opacity: 1, y: 0 },
};

export const fadeIn: Variants = {
  hidden: { opacity: 0 },
  visible: { opacity: 1 },
};

export const scaleIn: Variants = {
  hidden: { opacity: 0, scale: 0.95 },
  visible: { opacity: 1, scale: 1 },
};

export const slideInLeft: Variants = {
  hidden: { opacity: 0, x: -16 },
  visible: { opacity: 1, x: 0 },
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

export const tableRow: Variants = {
  hidden: { opacity: 0, x: -4 },
  visible: { opacity: 1, x: 0 },
};

export const springTransition = {
  type: "spring" as const,
  stiffness: 350,
  damping: 30,
};

export const easeTransition = {
  duration: 0.3,
  ease: [0.25, 0.1, 0.25, 1] as const,
};
