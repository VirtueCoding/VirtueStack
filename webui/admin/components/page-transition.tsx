"use client";

import { motion } from "motion/react";
import { easeTransition } from "@/lib/animations";

export function PageTransition({ children }: { children: React.ReactNode }) {
  return (
    <motion.div
      initial={{ opacity: 0, y: 8 }}
      animate={{ opacity: 1, y: 0 }}
      transition={easeTransition}
    >
      {children}
    </motion.div>
  );
}
