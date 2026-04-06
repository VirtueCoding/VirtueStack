# UI Modernization Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Transform both admin and customer UIs from plain default shadcn/ui to modern, elegant, polished interfaces with smooth transitions, refined theming, and better mobile experience.

**Architecture:** CSS variable-based design token refinement + Motion (Framer Motion) for declarative animations. No component library replacement — additive changes on top of existing shadcn/ui + Tailwind stack.

**Tech Stack:** Next.js 16, React 19, Tailwind CSS 3.4, motion (Framer Motion 12+), next-themes, Geist font, shadcn/ui (Radix primitives)

**Spec:** `docs/superpowers/specs/2026-04-01-ui-modernization-design.md`

---

## File Structure

### New Files
```
webui/admin/lib/animations.ts              — Shared animation variants
webui/admin/components/page-transition.tsx  — Page enter animation wrapper
webui/admin/components/animated-card.tsx    — Card with motion hover/enter
webui/admin/components/skeleton.tsx         — Shimmer skeleton loader
webui/customer/lib/animations.ts           — Shared animation variants (same)
webui/customer/components/page-transition.tsx
webui/customer/components/animated-card.tsx
webui/customer/components/skeleton.tsx
```

### Modified Files
```
webui/packages/ui/package.json             — No changes needed (motion used in app layer)
webui/admin/package.json                   — Add motion dependency
webui/customer/package.json                — Add motion dependency
webui/admin/app/globals.css                — New color palette + transition rules
webui/customer/app/globals.css             — New color palette + transition rules
webui/admin/tailwind.config.ts             — New keyframes, animations, sidebar colors
webui/customer/tailwind.config.ts          — New keyframes, animations, sidebar colors
webui/admin/app/layout.tsx                 — Geist font loading
webui/customer/app/layout.tsx              — Geist font loading
webui/admin/app/providers.tsx              — Remove disableTransitionOnChange
webui/customer/app/providers.tsx           — Remove disableTransitionOnChange
webui/admin/components/theme-toggle.tsx    — Animated icon toggle
webui/customer/components/theme-toggle.tsx — Animated icon toggle
webui/admin/components/sidebar.tsx         — Nav groups, new active state, tooltips
webui/customer/components/sidebar.tsx      — New active state, tooltips
webui/admin/components/mobile-nav.tsx      — Updated active state styling
webui/customer/components/mobile-nav.tsx   — Updated active state styling
webui/admin/lib/navigation.ts             — Add group labels to nav items
webui/admin/app/login/page.tsx             — Split layout with brand panel
webui/customer/app/login/page.tsx          — Enhanced with animations
webui/admin/app/dashboard/page.tsx         — Animated stat cards, better activity feed
webui/admin/app/dashboard/layout.tsx       — Add notification bell to header
webui/customer/app/vms/layout.tsx          — Add notification bell to header
```

---

## Phase 1: Foundation

### Task 1: Install Motion dependency

**Files:**
- Modify: `webui/admin/package.json`
- Modify: `webui/customer/package.json`

- [ ] **Step 1: Install motion in admin UI**

```bash
cd /home/hiron/VirtueStack/webui/admin && npm install motion
```

- [ ] **Step 2: Install motion in customer UI**

```bash
cd /home/hiron/VirtueStack/webui/customer && npm install motion
```

- [ ] **Step 3: Verify both installs succeeded**

```bash
cd /home/hiron/VirtueStack/webui/admin && node -e "require('motion/react')" && echo 'admin OK'
cd /home/hiron/VirtueStack/webui/customer && node -e "require('motion/react')" && echo 'customer OK'
```

Expected: Both print OK without errors.

- [ ] **Step 4: Commit**

```bash
cd /home/hiron/VirtueStack
git add webui/admin/package.json webui/admin/package-lock.json webui/customer/package.json webui/customer/package-lock.json
git commit -m "feat(ui): install motion (framer-motion) in both UIs

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```

---

### Task 2: Update admin color palette and global CSS

**Files:**
- Modify: `webui/admin/app/globals.css`

Replace the entire file with the refined slate + sidebar tokens + smooth theme transition CSS:

- [ ] **Step 1: Replace globals.css**

Replace `webui/admin/app/globals.css` with:

```css
@tailwind base;
@tailwind components;
@tailwind utilities;

@layer base {
  :root {
    --background: 0 0% 100%;
    --foreground: 224 71.4% 4.1%;
    --card: 0 0% 100%;
    --card-foreground: 224 71.4% 4.1%;
    --popover: 0 0% 100%;
    --popover-foreground: 224 71.4% 4.1%;
    --primary: 220.9 39.3% 11%;
    --primary-foreground: 210 20% 98%;
    --secondary: 220 14.3% 95.9%;
    --secondary-foreground: 220.9 39.3% 11%;
    --muted: 220 14.3% 95.9%;
    --muted-foreground: 220 8.9% 46.1%;
    --accent: 220 14.3% 95.9%;
    --accent-foreground: 220.9 39.3% 11%;
    --destructive: 0 84.2% 60.2%;
    --destructive-foreground: 210 20% 98%;
    --border: 220 13% 91%;
    --input: 220 13% 91%;
    --ring: 224 71.4% 4.1%;
    --radius: 0.625rem;
    --chart-1: 220 70% 50%;
    --chart-2: 160 60% 45%;
    --chart-3: 30 80% 55%;
    --chart-4: 280 65% 60%;
    --chart-5: 340 75% 55%;
    --sidebar: 0 0% 98%;
    --sidebar-foreground: 224 71.4% 4.1%;
    --sidebar-primary: 224 71.4% 4.1%;
    --sidebar-primary-foreground: 210 20% 98%;
    --sidebar-accent: 220 14.3% 95.9%;
    --sidebar-accent-foreground: 220.9 39.3% 11%;
    --sidebar-border: 220 13% 91%;
  }

  .dark {
    --background: 224 71.4% 4.1%;
    --foreground: 210 20% 98%;
    --card: 222 47% 6.5%;
    --card-foreground: 210 20% 98%;
    --popover: 224 71.4% 4.1%;
    --popover-foreground: 210 20% 98%;
    --primary: 210 20% 98%;
    --primary-foreground: 220.9 39.3% 11%;
    --secondary: 215 27.9% 16.9%;
    --secondary-foreground: 210 20% 98%;
    --muted: 215 27.9% 16.9%;
    --muted-foreground: 217.9 10.6% 64.9%;
    --accent: 215 27.9% 16.9%;
    --accent-foreground: 210 20% 98%;
    --destructive: 0 62.8% 30.6%;
    --destructive-foreground: 210 20% 98%;
    --border: 215 27.9% 16.9%;
    --input: 215 27.9% 16.9%;
    --ring: 216 12.2% 83.9%;
    --chart-1: 220 70% 50%;
    --chart-2: 160 60% 45%;
    --chart-3: 30 80% 55%;
    --chart-4: 280 65% 60%;
    --chart-5: 340 75% 55%;
    --sidebar: 222 47% 6.5%;
    --sidebar-foreground: 210 20% 98%;
    --sidebar-primary: 210 20% 98%;
    --sidebar-primary-foreground: 220.9 39.3% 11%;
    --sidebar-accent: 215 27.9% 16.9%;
    --sidebar-accent-foreground: 210 20% 98%;
    --sidebar-border: 215 27.9% 16.9%;
  }
}

@layer base {
  * {
    @apply border-border;
  }
  body {
    @apply bg-background text-foreground;
  }
}
```

- [ ] **Step 2: Verify no CSS syntax errors**

```bash
cd /home/hiron/VirtueStack/webui/admin && npx tailwindcss --content './app/globals.css' --output /dev/null 2>&1 || echo "Check manually"
```

- [ ] **Step 3: Commit**

```bash
cd /home/hiron/VirtueStack
git add webui/admin/app/globals.css
git commit -m "feat(admin): refine color palette with slate tones and sidebar tokens

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```

---

### Task 3: Update customer color palette and global CSS

**Files:**
- Modify: `webui/customer/app/globals.css`

- [ ] **Step 1: Replace globals.css**

Replace `webui/customer/app/globals.css` with:

```css
@tailwind base;
@tailwind components;
@tailwind utilities;

@layer base {
  :root {
    --background: 0 0% 100%;
    --foreground: 222.2 84% 4.9%;
    --card: 0 0% 100%;
    --card-foreground: 222.2 84% 4.9%;
    --popover: 0 0% 100%;
    --popover-foreground: 222.2 84% 4.9%;
    --primary: 221.2 83.2% 53.3%;
    --primary-foreground: 210 40% 98%;
    --secondary: 210 40% 96.1%;
    --secondary-foreground: 222.2 47.4% 11.2%;
    --muted: 210 40% 96.1%;
    --muted-foreground: 215.4 16.3% 46.9%;
    --accent: 210 40% 96.1%;
    --accent-foreground: 222.2 47.4% 11.2%;
    --destructive: 0 84.2% 60.2%;
    --destructive-foreground: 210 40% 98%;
    --border: 214.3 31.8% 91.4%;
    --input: 214.3 31.8% 91.4%;
    --ring: 221.2 83.2% 53.3%;
    --radius: 0.625rem;
    --chart-1: 221 83% 53%;
    --chart-2: 212 95% 68%;
    --chart-3: 216 92% 60%;
    --chart-4: 210 98% 78%;
    --chart-5: 183 74% 44%;
    --sidebar: 0 0% 98%;
    --sidebar-foreground: 222.2 84% 4.9%;
    --sidebar-primary: 221.2 83.2% 53.3%;
    --sidebar-primary-foreground: 210 40% 98%;
    --sidebar-accent: 210 40% 96.1%;
    --sidebar-accent-foreground: 222.2 47.4% 11.2%;
    --sidebar-border: 214.3 31.8% 91.4%;
  }

  .dark {
    --background: 222.2 84% 4.9%;
    --foreground: 210 40% 98%;
    --card: 217.2 32.6% 11%;
    --card-foreground: 210 40% 98%;
    --popover: 222.2 84% 4.9%;
    --popover-foreground: 210 40% 98%;
    --primary: 217.2 91.2% 59.8%;
    --primary-foreground: 222.2 47.4% 11.2%;
    --secondary: 217.2 32.6% 17.5%;
    --secondary-foreground: 210 40% 98%;
    --muted: 217.2 32.6% 17.5%;
    --muted-foreground: 215 20.2% 65.1%;
    --accent: 217.2 32.6% 17.5%;
    --accent-foreground: 210 40% 98%;
    --destructive: 0 62.8% 30.6%;
    --destructive-foreground: 210 40% 98%;
    --border: 217.2 32.6% 17.5%;
    --input: 217.2 32.6% 17.5%;
    --ring: 224.3 76.3% 48%;
    --chart-1: 221 83% 53%;
    --chart-2: 212 95% 68%;
    --chart-3: 216 92% 60%;
    --chart-4: 210 98% 78%;
    --chart-5: 183 74% 44%;
    --sidebar: 222.2 84% 4.9%;
    --sidebar-foreground: 210 40% 98%;
    --sidebar-primary: 217.2 91.2% 59.8%;
    --sidebar-primary-foreground: 222.2 47.4% 11.2%;
    --sidebar-accent: 217.2 32.6% 17.5%;
    --sidebar-accent-foreground: 210 40% 98%;
    --sidebar-border: 217.2 32.6% 17.5%;
  }
}

@layer base {
  * {
    @apply border-border;
  }
  body {
    @apply bg-background text-foreground;
  }
}
```

- [ ] **Step 2: Commit**

```bash
cd /home/hiron/VirtueStack
git add webui/customer/app/globals.css
git commit -m "feat(customer): refine color palette with elevated dark cards and sidebar tokens

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```

---

### Task 4: Update Tailwind configs with new animations and sidebar colors

**Files:**
- Modify: `webui/admin/tailwind.config.ts`
- Modify: `webui/customer/tailwind.config.ts`

- [ ] **Step 1: Replace admin tailwind.config.ts**

Replace `webui/admin/tailwind.config.ts` with:

```ts
import type { Config } from 'tailwindcss';

const config: Config = {
  darkMode: ['class'],
  content: [
    './app/**/*.{js,ts,jsx,tsx,mdx}',
    './components/**/*.{js,ts,jsx,tsx,mdx}',
  ],
  theme: {
    container: {
      center: true,
      padding: '2rem',
      screens: {
        '2xl': '1400px',
      },
    },
    extend: {
      colors: {
        border: 'hsl(var(--border))',
        input: 'hsl(var(--input))',
        ring: 'hsl(var(--ring))',
        background: 'hsl(var(--background))',
        foreground: 'hsl(var(--foreground))',
        primary: {
          DEFAULT: 'hsl(var(--primary))',
          foreground: 'hsl(var(--primary-foreground))',
        },
        secondary: {
          DEFAULT: 'hsl(var(--secondary))',
          foreground: 'hsl(var(--secondary-foreground))',
        },
        destructive: {
          DEFAULT: 'hsl(var(--destructive))',
          foreground: 'hsl(var(--destructive-foreground))',
        },
        muted: {
          DEFAULT: 'hsl(var(--muted))',
          foreground: 'hsl(var(--muted-foreground))',
        },
        accent: {
          DEFAULT: 'hsl(var(--accent))',
          foreground: 'hsl(var(--accent-foreground))',
        },
        popover: {
          DEFAULT: 'hsl(var(--popover))',
          foreground: 'hsl(var(--popover-foreground))',
        },
        card: {
          DEFAULT: 'hsl(var(--card))',
          foreground: 'hsl(var(--card-foreground))',
        },
        sidebar: {
          DEFAULT: 'hsl(var(--sidebar))',
          foreground: 'hsl(var(--sidebar-foreground))',
          primary: 'hsl(var(--sidebar-primary))',
          'primary-foreground': 'hsl(var(--sidebar-primary-foreground))',
          accent: 'hsl(var(--sidebar-accent))',
          'accent-foreground': 'hsl(var(--sidebar-accent-foreground))',
          border: 'hsl(var(--sidebar-border))',
        },
        chart: {
          '1': 'hsl(var(--chart-1))',
          '2': 'hsl(var(--chart-2))',
          '3': 'hsl(var(--chart-3))',
          '4': 'hsl(var(--chart-4))',
          '5': 'hsl(var(--chart-5))',
        },
      },
      borderRadius: {
        lg: 'var(--radius)',
        md: 'calc(var(--radius) - 2px)',
        sm: 'calc(var(--radius) - 4px)',
      },
      keyframes: {
        'accordion-down': {
          from: { height: '0' },
          to: { height: 'var(--radix-accordion-content-height)' },
        },
        'accordion-up': {
          from: { height: 'var(--radix-accordion-content-height)' },
          to: { height: '0' },
        },
        'fade-up': {
          from: { opacity: '0', transform: 'translateY(8px)' },
          to: { opacity: '1', transform: 'translateY(0)' },
        },
        'fade-in': {
          from: { opacity: '0' },
          to: { opacity: '1' },
        },
        'scale-in': {
          from: { opacity: '0', transform: 'scale(0.95)' },
          to: { opacity: '1', transform: 'scale(1)' },
        },
        'slide-in-right': {
          from: { transform: 'translateX(100%)' },
          to: { transform: 'translateX(0)' },
        },
        'shimmer': {
          from: { backgroundPosition: '200% 0' },
          to: { backgroundPosition: '-200% 0' },
        },
      },
      animation: {
        'accordion-down': 'accordion-down 0.2s ease-out',
        'accordion-up': 'accordion-up 0.2s ease-out',
        'fade-up': 'fade-up 0.4s ease-out',
        'fade-in': 'fade-in 0.3s ease-out',
        'scale-in': 'scale-in 0.2s ease-out',
        'slide-in-right': 'slide-in-right 0.3s ease-out',
        'shimmer': 'shimmer 2s linear infinite',
      },
    },
  },
  plugins: [require('tailwindcss-animate')],
};

export default config;
```

- [ ] **Step 2: Replace customer tailwind.config.ts**

Replace `webui/customer/tailwind.config.ts` with the exact same content as above (identical config).

- [ ] **Step 3: Commit**

```bash
cd /home/hiron/VirtueStack
git add webui/admin/tailwind.config.ts webui/customer/tailwind.config.ts
git commit -m "feat(ui): add sidebar colors, chart tokens, and animation keyframes to Tailwind configs

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```

---

### Task 5: Add Geist font to both layouts

**Files:**
- Modify: `webui/admin/app/layout.tsx`
- Modify: `webui/customer/app/layout.tsx`

- [ ] **Step 1: Update admin layout.tsx**

Replace `webui/admin/app/layout.tsx` with:

```tsx
import type { Metadata } from 'next';
import { Geist, Geist_Mono } from 'next/font/google';
import './globals.css';
import { Providers } from './providers';
import { Toaster } from '@virtuestack/ui';

const geistSans = Geist({
  variable: '--font-sans',
  subsets: ['latin'],
});

const geistMono = Geist_Mono({
  variable: '--font-mono',
  subsets: ['latin'],
});

export const metadata: Metadata = {
  title: 'VirtueStack Admin',
  description: 'VirtueStack Administration Panel',
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="en" suppressHydrationWarning>
      <body className={`${geistSans.variable} ${geistMono.variable} font-sans antialiased`}>
        <Providers>{children}</Providers>
        <Toaster />
      </body>
    </html>
  );
}
```

- [ ] **Step 2: Update customer layout.tsx**

Replace `webui/customer/app/layout.tsx` with:

```tsx
import type { Metadata } from 'next';
import { Geist, Geist_Mono } from 'next/font/google';
import './globals.css';
import { Providers } from './providers';
import { Toaster } from '@virtuestack/ui';

const geistSans = Geist({
  variable: '--font-sans',
  subsets: ['latin'],
});

const geistMono = Geist_Mono({
  variable: '--font-mono',
  subsets: ['latin'],
});

export const metadata: Metadata = {
  title: 'VirtueStack Customer Portal',
  description: 'VirtueStack Customer Self-Service Portal',
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="en" suppressHydrationWarning>
      <body className={`${geistSans.variable} ${geistMono.variable} font-sans antialiased`}>
        <Providers>{children}</Providers>
        <Toaster />
      </body>
    </html>
  );
}
```

- [ ] **Step 3: Commit**

```bash
cd /home/hiron/VirtueStack
git add webui/admin/app/layout.tsx webui/customer/app/layout.tsx
git commit -m "feat(ui): add Geist font family to both UIs

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```

---

### Task 6: Enable smooth theme transitions

**Files:**
- Modify: `webui/admin/app/providers.tsx`
- Modify: `webui/customer/app/providers.tsx`

- [ ] **Step 1: Update admin providers.tsx — remove disableTransitionOnChange**

In `webui/admin/app/providers.tsx`, remove the `disableTransitionOnChange` prop from `<ThemeProvider>`. The full file becomes:

```tsx
'use client';

import { ThemeProvider } from 'next-themes';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { useState } from 'react';
import { AuthProvider } from '@/lib/auth-context';
import { PermissionProvider } from '@/contexts/PermissionContext';

export function Providers({ children }: { children: React.ReactNode }) {
  const [queryClient] = useState(
    () =>
      new QueryClient({
        defaultOptions: {
          queries: {
            staleTime: 60 * 1000,
            refetchOnWindowFocus: false,
          },
        },
      })
  );

  return (
    <QueryClientProvider client={queryClient}>
      <ThemeProvider
        attribute="class"
        defaultTheme="system"
        enableSystem
      >
        <AuthProvider>
          <PermissionProvider>
            {children}
          </PermissionProvider>
        </AuthProvider>
      </ThemeProvider>
    </QueryClientProvider>
  );
}
```

- [ ] **Step 2: Update customer providers.tsx — remove disableTransitionOnChange**

In `webui/customer/app/providers.tsx`, remove the `disableTransitionOnChange` prop. The full file becomes:

```tsx
'use client';

import { ThemeProvider } from 'next-themes';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { useState } from 'react';
import { AuthProvider } from '@/lib/auth-context';

export function Providers({ children }: { children: React.ReactNode }) {
  const [queryClient] = useState(
    () =>
      new QueryClient({
        defaultOptions: {
          queries: {
            staleTime: 60 * 1000,
            refetchOnWindowFocus: false,
          },
        },
      })
  );

  return (
    <QueryClientProvider client={queryClient}>
      <AuthProvider>
        <ThemeProvider
          attribute="class"
          defaultTheme="system"
          enableSystem
        >
          {children}
        </ThemeProvider>
      </AuthProvider>
    </QueryClientProvider>
  );
}
```

- [ ] **Step 3: Commit**

```bash
cd /home/hiron/VirtueStack
git add webui/admin/app/providers.tsx webui/customer/app/providers.tsx
git commit -m "feat(ui): enable smooth theme transitions by removing disableTransitionOnChange

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```

---

### Task 7: Create shared animation utilities and components

**Files:**
- Create: `webui/admin/lib/animations.ts`
- Create: `webui/customer/lib/animations.ts`
- Create: `webui/admin/components/page-transition.tsx`
- Create: `webui/customer/components/page-transition.tsx`
- Create: `webui/admin/components/animated-card.tsx`
- Create: `webui/customer/components/animated-card.tsx`
- Create: `webui/admin/components/skeleton.tsx`
- Create: `webui/customer/components/skeleton.tsx`

- [ ] **Step 1: Create admin lib/animations.ts**

Create `webui/admin/lib/animations.ts`:

```ts
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
```

- [ ] **Step 2: Create customer lib/animations.ts**

Create `webui/customer/lib/animations.ts` with the exact same content as admin's `lib/animations.ts` above.

- [ ] **Step 3: Create admin components/page-transition.tsx**

Create `webui/admin/components/page-transition.tsx`:

```tsx
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
```

- [ ] **Step 4: Create customer components/page-transition.tsx**

Create `webui/customer/components/page-transition.tsx` with the exact same content as admin's version above.

- [ ] **Step 5: Create admin components/animated-card.tsx**

Create `webui/admin/components/animated-card.tsx`:

```tsx
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
```

- [ ] **Step 6: Create customer components/animated-card.tsx**

Create `webui/customer/components/animated-card.tsx` with the exact same content as admin's version above.

- [ ] **Step 7: Create admin components/skeleton.tsx**

Create `webui/admin/components/skeleton.tsx`:

```tsx
import { cn } from "@/lib/utils";

export function Skeleton({
  className,
  ...props
}: React.HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      className={cn(
        "rounded-md bg-muted animate-pulse",
        className
      )}
      {...props}
    />
  );
}
```

- [ ] **Step 8: Create customer components/skeleton.tsx**

Create `webui/customer/components/skeleton.tsx` with the exact same content as admin's version above.

- [ ] **Step 9: Commit**

```bash
cd /home/hiron/VirtueStack
git add webui/admin/lib/animations.ts webui/customer/lib/animations.ts \
  webui/admin/components/page-transition.tsx webui/customer/components/page-transition.tsx \
  webui/admin/components/animated-card.tsx webui/customer/components/animated-card.tsx \
  webui/admin/components/skeleton.tsx webui/customer/components/skeleton.tsx
git commit -m "feat(ui): add animation utilities, PageTransition, AnimatedCard, and Skeleton components

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```

---

### Task 8: Verify Phase 1 — build both UIs

- [ ] **Step 1: Build admin UI**

```bash
cd /home/hiron/VirtueStack/webui/admin && npm run build
```

Expected: Build succeeds with no errors.

- [ ] **Step 2: Build customer UI**

```bash
cd /home/hiron/VirtueStack/webui/customer && npm run build
```

Expected: Build succeeds with no errors.

- [ ] **Step 3: Fix any build errors if present**

If either build fails, fix the errors in the relevant files and re-run. Common issues: import path typos, missing `"use client"` directives, TypeScript type errors.

---

## Phase 2: Shared Layout Components

### Task 9: Update admin navigation with groups

**Files:**
- Modify: `webui/admin/lib/navigation.ts`

- [ ] **Step 1: Update navigation.ts with group structure**

Replace `webui/admin/lib/navigation.ts` with:

```ts
import {
  LayoutDashboard,
  Monitor,
  Server,
  HardDrive,
  GitBranch,
  FileText,
  Network,
  Users,
  CreditCard,
  Receipt,
  Database,
  Calendar,
  KeyRound,
  ShieldCheck,
  Layers,
} from "lucide-react";
import type { LucideIcon } from "lucide-react";

export interface NavItem {
  href: string;
  label: string;
  icon: LucideIcon;
}

export interface NavGroup {
  label: string;
  items: NavItem[];
}

export const adminNavGroups: NavGroup[] = [
  {
    label: "Overview",
    items: [
      { href: "/dashboard", label: "Dashboard", icon: LayoutDashboard },
    ],
  },
  {
    label: "Infrastructure",
    items: [
      { href: "/nodes", label: "Nodes", icon: Server },
      { href: "/storage-backends", label: "Storage", icon: HardDrive },
      { href: "/failover-requests", label: "Failover", icon: GitBranch },
      { href: "/templates", label: "Templates", icon: Layers },
    ],
  },
  {
    label: "Virtual Machines",
    items: [
      { href: "/vms", label: "VMs", icon: Monitor },
      { href: "/plans", label: "Plans", icon: FileText },
      { href: "/ip-sets", label: "IP Sets", icon: Network },
    ],
  },
  {
    label: "Customers",
    items: [
      { href: "/customers", label: "Customers", icon: Users },
      { href: "/billing", label: "Billing", icon: CreditCard },
      { href: "/invoices", label: "Invoices", icon: Receipt },
    ],
  },
  {
    label: "System",
    items: [
      { href: "/backups", label: "Backups", icon: Database },
      { href: "/backup-schedules", label: "Schedules", icon: Calendar },
      { href: "/provisioning-keys", label: "Provisioning Keys", icon: KeyRound },
      { href: "/audit-logs", label: "Audit Logs", icon: ShieldCheck },
    ],
  },
];

export const adminNavItems: NavItem[] = adminNavGroups.flatMap(
  (group) => group.items
);
```

- [ ] **Step 2: Commit**

```bash
cd /home/hiron/VirtueStack
git add webui/admin/lib/navigation.ts
git commit -m "feat(admin): organize nav items into labeled groups

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```

---

### Task 10: Redesign admin sidebar

**Files:**
- Modify: `webui/admin/components/sidebar.tsx`

- [ ] **Step 1: Replace sidebar.tsx**

Replace `webui/admin/components/sidebar.tsx` with:

```tsx
"use client";

import { LogOut, Settings, ChevronLeft } from "lucide-react";
import Link from "next/link";
import { usePathname } from "next/navigation";

import { cn } from "@/lib/utils";
import { useAuth } from "@/lib/auth-context";
import { adminNavGroups } from "@/lib/navigation";
import { Button, ScrollArea } from "@virtuestack/ui";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@virtuestack/ui";
import { Avatar, AvatarFallback, AvatarImage } from "@virtuestack/ui";
import { NotificationBell } from "@/components/notification-bell";

interface SidebarProps {
  collapsed?: boolean;
  onToggle?: () => void;
}

export function Sidebar({ collapsed = false, onToggle }: SidebarProps) {
  const pathname = usePathname();
  const { user, logout } = useAuth();

  const userEmail = user?.email || "Admin";
  const localPart = userEmail.split("@")[0];
  const initials = localPart?.trim()
    ? localPart.slice(0, 2).toUpperCase()
    : "??";

  return (
    <div
      className={cn(
        "relative flex h-screen flex-col border-r bg-sidebar transition-all duration-300",
        collapsed ? "w-16" : "w-64"
      )}
    >
      <div className="flex h-14 items-center border-b border-sidebar-border px-4">
        {!collapsed && (
          <span className="text-lg font-semibold tracking-tight">
            VirtueStack
          </span>
        )}
        <div
          className={cn(
            "ml-auto flex items-center gap-1",
            collapsed && "mx-auto"
          )}
        >
          {!collapsed && <NotificationBell />}
          <Button variant="ghost" size="icon" onClick={onToggle}>
            <ChevronLeft
              className={cn(
                "h-4 w-4 transition-transform duration-200",
                collapsed && "rotate-180"
              )}
            />
            <span className="sr-only">Toggle sidebar</span>
          </Button>
        </div>
      </div>

      <ScrollArea className="flex-1 py-2">
        <nav className="flex flex-col gap-1 px-2">
          {adminNavGroups.map((group) => (
            <div key={group.label} className="mt-2 first:mt-0">
              {!collapsed && (
                <p className="mb-1 px-3 text-[11px] font-medium uppercase tracking-wider text-muted-foreground/70">
                  {group.label}
                </p>
              )}
              {collapsed && <div className="mx-auto my-1 h-px w-6 bg-sidebar-border" />}
              {group.items.map((item) => {
                const Icon = item.icon;
                const isActive = pathname === item.href;
                return (
                  <Link
                    key={item.href}
                    href={item.href}
                    title={collapsed ? item.label : undefined}
                    className={cn(
                      "group relative flex items-center gap-3 rounded-lg px-3 py-2 text-sm font-medium transition-all duration-150",
                      isActive
                        ? "bg-primary/10 text-primary dark:text-primary"
                        : "text-muted-foreground hover:bg-accent hover:text-accent-foreground",
                      collapsed && "justify-center px-2"
                    )}
                  >
                    {isActive && (
                      <span className="absolute left-0 top-1/2 h-5 w-[3px] -translate-y-1/2 rounded-r-full bg-primary" />
                    )}
                    <Icon
                      className={cn(
                        "h-4 w-4 shrink-0 transition-colors",
                        isActive && "text-primary"
                      )}
                    />
                    {!collapsed && <span>{item.label}</span>}
                  </Link>
                );
              })}
            </div>
          ))}
        </nav>
      </ScrollArea>

      <div className="border-t border-sidebar-border p-3">
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button
              variant="ghost"
              className={cn(
                "w-full justify-start gap-3 rounded-lg",
                collapsed && "justify-center px-2"
              )}
            >
              <Avatar className="h-8 w-8">
                <AvatarImage src="/avatars/admin.png" alt={userEmail} />
                <AvatarFallback className="bg-primary/10 text-primary text-xs">
                  {initials}
                </AvatarFallback>
              </Avatar>
              {!collapsed && (
                <div className="flex flex-col items-start text-xs">
                  <span className="font-medium">{localPart}</span>
                  <span className="text-muted-foreground">
                    {user?.role || "Admin"}
                  </span>
                </div>
              )}
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="w-56">
            <DropdownMenuLabel>My Account</DropdownMenuLabel>
            <DropdownMenuSeparator />
            <DropdownMenuItem asChild>
              <Link href="/settings">
                <Settings className="mr-2 h-4 w-4" />
                Settings
              </Link>
            </DropdownMenuItem>
            <DropdownMenuItem onClick={() => logout()}>
              <LogOut className="mr-2 h-4 w-4" />
              Log out
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
    </div>
  );
}
```

Key changes:
- Uses `bg-sidebar` / `border-sidebar-border` instead of `bg-card`
- Nav items organized into groups with uppercase labels
- Active state: left pill indicator + `bg-primary/10 text-primary` instead of solid `bg-primary`
- Collapsed mode shows dividers between groups and uses `title` attribute for tooltip
- `rounded-lg` for softer nav items

- [ ] **Step 2: Commit**

```bash
cd /home/hiron/VirtueStack
git add webui/admin/components/sidebar.tsx
git commit -m "feat(admin): redesign sidebar with nav groups, pill indicator, and sidebar tokens

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```

---

### Task 11: Redesign customer sidebar

**Files:**
- Modify: `webui/customer/components/sidebar.tsx`

- [ ] **Step 1: Replace customer sidebar.tsx**

Replace `webui/customer/components/sidebar.tsx` with:

```tsx
"use client";

import { Settings, LogOut, ChevronLeft } from "lucide-react";
import Link from "next/link";
import { usePathname } from "next/navigation";

import { cn } from "@/lib/utils";
import { useAuth } from "@/lib/auth-context";
import { navItems } from "@/lib/nav-items";
import { Button, ScrollArea } from "@virtuestack/ui";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@virtuestack/ui";
import { Avatar, AvatarFallback, AvatarImage } from "@virtuestack/ui";
import { NotificationBell } from "@/components/notification-bell";

interface SidebarProps {
  collapsed?: boolean;
  onToggle?: () => void;
}

export function Sidebar({ collapsed = false, onToggle }: SidebarProps) {
  const pathname = usePathname();
  const { user, logout } = useAuth();

  const userEmail = user?.email || "Customer";
  const localPart = userEmail.split("@")[0];
  const initials = localPart?.trim()
    ? localPart.slice(0, 2).toUpperCase()
    : "??";

  return (
    <div
      className={cn(
        "relative flex h-screen flex-col border-r bg-sidebar transition-all duration-300",
        collapsed ? "w-16" : "w-64"
      )}
    >
      <div className="flex h-14 items-center border-b border-sidebar-border px-4">
        {!collapsed && (
          <span className="text-lg font-semibold tracking-tight">
            VirtueStack
          </span>
        )}
        <div
          className={cn(
            "ml-auto flex items-center gap-1",
            collapsed && "mx-auto"
          )}
        >
          {!collapsed && <NotificationBell />}
          <Button variant="ghost" size="icon" onClick={onToggle}>
            <ChevronLeft
              className={cn(
                "h-4 w-4 transition-transform duration-200",
                collapsed && "rotate-180"
              )}
            />
            <span className="sr-only">Toggle sidebar</span>
          </Button>
        </div>
      </div>

      <ScrollArea className="flex-1 py-4">
        <nav className="flex flex-col gap-1 px-2">
          {navItems.map((item) => {
            const Icon = item.icon;
            const isActive =
              pathname === item.href ||
              pathname?.startsWith(item.href + "/");
            return (
              <Link
                key={item.href}
                href={item.href}
                title={collapsed ? item.label : undefined}
                className={cn(
                  "group relative flex items-center gap-3 rounded-lg px-3 py-2 text-sm font-medium transition-all duration-150",
                  isActive
                    ? "bg-primary/10 text-primary dark:text-primary"
                    : "text-muted-foreground hover:bg-accent hover:text-accent-foreground",
                  collapsed && "justify-center px-2"
                )}
              >
                {isActive && (
                  <span className="absolute left-0 top-1/2 h-5 w-[3px] -translate-y-1/2 rounded-r-full bg-primary" />
                )}
                <Icon
                  className={cn(
                    "h-4 w-4 shrink-0 transition-colors",
                    isActive && "text-primary"
                  )}
                />
                {!collapsed && <span>{item.label}</span>}
              </Link>
            );
          })}
        </nav>
      </ScrollArea>

      <div className="border-t border-sidebar-border p-3">
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button
              variant="ghost"
              className={cn(
                "w-full justify-start gap-3 rounded-lg",
                collapsed && "justify-center px-2"
              )}
            >
              <Avatar className="h-8 w-8">
                <AvatarImage src="/avatars/customer.png" alt={userEmail} />
                <AvatarFallback className="bg-primary/10 text-primary text-xs">
                  {initials}
                </AvatarFallback>
              </Avatar>
              {!collapsed && (
                <div className="flex flex-col items-start text-xs">
                  <span className="font-medium">{localPart}</span>
                  <span className="text-muted-foreground">{userEmail}</span>
                </div>
              )}
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="w-56">
            <DropdownMenuLabel>My Account</DropdownMenuLabel>
            <DropdownMenuSeparator />
            <DropdownMenuItem asChild>
              <Link href="/settings">
                <Settings className="mr-2 h-4 w-4" />
                Account Settings
              </Link>
            </DropdownMenuItem>
            <DropdownMenuItem onClick={() => logout()}>
              <LogOut className="mr-2 h-4 w-4" />
              Log out
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
    </div>
  );
}
```

- [ ] **Step 2: Commit**

```bash
cd /home/hiron/VirtueStack
git add webui/customer/components/sidebar.tsx
git commit -m "feat(customer): redesign sidebar with pill indicator and sidebar tokens

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```

---

### Task 12: Update mobile nav active states in both UIs

**Files:**
- Modify: `webui/admin/components/mobile-nav.tsx`
- Modify: `webui/customer/components/mobile-nav.tsx`

- [ ] **Step 1: Replace admin mobile-nav.tsx**

Replace `webui/admin/components/mobile-nav.tsx` with:

```tsx
"use client";

import { Menu } from "lucide-react";
import Link from "next/link";
import { usePathname } from "next/navigation";

import { cn } from "@/lib/utils";
import { Button } from "@virtuestack/ui";
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
  SheetClose,
} from "@virtuestack/ui";
import { adminNavGroups } from "@/lib/navigation";

export function MobileNav() {
  const pathname = usePathname();

  return (
    <Sheet>
      <SheetTrigger asChild>
        <Button variant="ghost" size="icon" className="md:hidden">
          <Menu className="h-5 w-5" />
          <span className="sr-only">Toggle navigation menu</span>
        </Button>
      </SheetTrigger>
      <SheetContent side="left" className="w-72">
        <SheetHeader>
          <SheetTitle className="text-left tracking-tight">
            VirtueStack Admin
          </SheetTitle>
        </SheetHeader>
        <nav className="mt-4 flex flex-col gap-1">
          {adminNavGroups.map((group) => (
            <div key={group.label} className="mt-3 first:mt-0">
              <p className="mb-1 px-3 text-[11px] font-medium uppercase tracking-wider text-muted-foreground/70">
                {group.label}
              </p>
              {group.items.map((item) => {
                const Icon = item.icon;
                const isActive = pathname === item.href;
                return (
                  <SheetClose asChild key={item.href}>
                    <Link
                      href={item.href}
                      className={cn(
                        "relative flex items-center gap-3 rounded-lg px-3 py-2.5 text-sm font-medium transition-all duration-150",
                        isActive
                          ? "bg-primary/10 text-primary"
                          : "text-muted-foreground hover:bg-accent hover:text-accent-foreground"
                      )}
                    >
                      {isActive && (
                        <span className="absolute left-0 top-1/2 h-5 w-[3px] -translate-y-1/2 rounded-r-full bg-primary" />
                      )}
                      <Icon className="h-4 w-4" />
                      {item.label}
                    </Link>
                  </SheetClose>
                );
              })}
            </div>
          ))}
        </nav>
      </SheetContent>
    </Sheet>
  );
}
```

- [ ] **Step 2: Replace customer mobile-nav.tsx**

Replace `webui/customer/components/mobile-nav.tsx` with:

```tsx
"use client";

import { Menu } from "lucide-react";
import Link from "next/link";
import { usePathname } from "next/navigation";

import { cn } from "@/lib/utils";
import { navItems } from "@/lib/nav-items";
import { Button } from "@virtuestack/ui";
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
  SheetClose,
} from "@virtuestack/ui";

export function MobileNav() {
  const pathname = usePathname();

  return (
    <Sheet>
      <SheetTrigger asChild>
        <Button variant="ghost" size="icon" className="md:hidden">
          <Menu className="h-5 w-5" />
          <span className="sr-only">Toggle navigation menu</span>
        </Button>
      </SheetTrigger>
      <SheetContent side="left" className="w-72">
        <SheetHeader>
          <SheetTitle className="text-left tracking-tight">
            VirtueStack
          </SheetTitle>
        </SheetHeader>
        <nav className="mt-6 flex flex-col gap-1">
          {navItems.map((item) => {
            const Icon = item.icon;
            const isActive =
              pathname === item.href ||
              pathname?.startsWith(item.href + "/");
            return (
              <SheetClose asChild key={item.href}>
                <Link
                  href={item.href}
                  className={cn(
                    "relative flex items-center gap-3 rounded-lg px-3 py-2.5 text-sm font-medium transition-all duration-150",
                    isActive
                      ? "bg-primary/10 text-primary"
                      : "text-muted-foreground hover:bg-accent hover:text-accent-foreground"
                  )}
                >
                  {isActive && (
                    <span className="absolute left-0 top-1/2 h-5 w-[3px] -translate-y-1/2 rounded-r-full bg-primary" />
                  )}
                  <Icon className="h-4 w-4" />
                  {item.label}
                </Link>
              </SheetClose>
            );
          })}
        </nav>
      </SheetContent>
    </Sheet>
  );
}
```

- [ ] **Step 3: Commit**

```bash
cd /home/hiron/VirtueStack
git add webui/admin/components/mobile-nav.tsx webui/customer/components/mobile-nav.tsx
git commit -m "feat(ui): update mobile nav with grouped items and pill active indicator

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```

---

### Task 13: Redesign theme toggle with animated icon

**Files:**
- Modify: `webui/admin/components/theme-toggle.tsx`
- Modify: `webui/customer/components/theme-toggle.tsx`

- [ ] **Step 1: Replace admin theme-toggle.tsx**

Replace `webui/admin/components/theme-toggle.tsx` with:

```tsx
"use client";

import { useTheme } from "next-themes";
import { Button } from "@virtuestack/ui";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@virtuestack/ui";
import { Moon, Sun, Monitor } from "lucide-react";
import { motion, AnimatePresence } from "motion/react";

export function ThemeToggle() {
  const { setTheme, theme } = useTheme();

  const iconMap = {
    light: Sun,
    dark: Moon,
    system: Monitor,
  } as const;

  const CurrentIcon = iconMap[(theme as keyof typeof iconMap)] || Monitor;

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          variant="outline"
          size="icon"
          aria-label="Toggle theme"
          className="relative overflow-hidden"
        >
          <AnimatePresence mode="wait" initial={false}>
            <motion.div
              key={theme}
              initial={{ y: -20, opacity: 0, rotate: -90 }}
              animate={{ y: 0, opacity: 1, rotate: 0 }}
              exit={{ y: 20, opacity: 0, rotate: 90 }}
              transition={{ duration: 0.15 }}
            >
              <CurrentIcon className="h-[1.2rem] w-[1.2rem]" />
            </motion.div>
          </AnimatePresence>
          <span className="sr-only">Toggle theme</span>
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        <DropdownMenuItem onClick={() => setTheme("light")}>
          <Sun className="mr-2 h-4 w-4" />
          Light
        </DropdownMenuItem>
        <DropdownMenuItem onClick={() => setTheme("dark")}>
          <Moon className="mr-2 h-4 w-4" />
          Dark
        </DropdownMenuItem>
        <DropdownMenuItem onClick={() => setTheme("system")}>
          <Monitor className="mr-2 h-4 w-4" />
          System
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
```

- [ ] **Step 2: Replace customer theme-toggle.tsx**

Replace `webui/customer/components/theme-toggle.tsx` with the exact same content as admin's version above.

- [ ] **Step 3: Commit**

```bash
cd /home/hiron/VirtueStack
git add webui/admin/components/theme-toggle.tsx webui/customer/components/theme-toggle.tsx
git commit -m "feat(ui): animate theme toggle icon with rotation transition

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```

---

### Task 14: Verify Phase 2 — build both UIs

- [ ] **Step 1: Build admin UI**

```bash
cd /home/hiron/VirtueStack/webui/admin && npm run build
```

Expected: Build succeeds.

- [ ] **Step 2: Build customer UI**

```bash
cd /home/hiron/VirtueStack/webui/customer && npm run build
```

Expected: Build succeeds.

- [ ] **Step 3: Fix any build errors**

---

## Phase 3: Login Pages

### Task 15: Redesign admin login page with split layout

**Files:**
- Modify: `webui/admin/app/login/page.tsx`

- [ ] **Step 1: Replace admin login page**

Replace `webui/admin/app/login/page.tsx` with:

```tsx
"use client";

import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { Loader2, Shield, Server } from "lucide-react";
import { motion, AnimatePresence } from "motion/react";
import {
  Button,
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
  Input,
  Label,
} from "@virtuestack/ui";
import { useAuth } from "@/lib/auth-context";

const loginSchema = z.object({
  email: z.string().email("Invalid email address"),
  password: z.string().min(12, "Password must be at least 12 characters"),
});

const totpSchema = z.object({
  totp_code: z
    .string()
    .length(6, "2FA code must be 6 digits")
    .regex(/^\d+$/, "Code must contain only numbers"),
});

type LoginFormData = z.infer<typeof loginSchema>;
type TotpFormData = z.infer<typeof totpSchema>;

export default function LoginPage() {
  const {
    login,
    verify2FA,
    requires2FA,
    tempToken,
    isLoading,
    error,
    clearError,
    reset2FA,
  } = useAuth();

  const loginForm = useForm<LoginFormData>({
    resolver: zodResolver(loginSchema),
  });

  const totpForm = useForm<TotpFormData>({
    resolver: zodResolver(totpSchema),
  });

  const onLoginSubmit = async (data: LoginFormData) => {
    clearError();
    await login(data);
  };

  const onTotpSubmit = async (data: TotpFormData) => {
    clearError();
    if (!tempToken) return;
    await verify2FA({
      temp_token: tempToken,
      totp_code: data.totp_code,
    });
  };

  return (
    <div className="flex min-h-screen">
      {/* Brand Panel — hidden on mobile */}
      <div className="hidden lg:flex lg:w-1/2 items-center justify-center bg-gradient-to-br from-slate-900 via-slate-800 to-slate-900 p-12">
        <motion.div
          initial={{ opacity: 0, y: 20 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.6, ease: [0.25, 0.1, 0.25, 1] }}
          className="max-w-md text-center"
        >
          <div className="mx-auto mb-8 flex h-16 w-16 items-center justify-center rounded-2xl bg-white/10 backdrop-blur-sm">
            <Server className="h-8 w-8 text-white" />
          </div>
          <h1 className="text-4xl font-bold tracking-tight text-white">
            VirtueStack
          </h1>
          <p className="mt-3 text-lg text-slate-300">
            Infrastructure Management
          </p>
          <div className="mt-8 grid grid-cols-3 gap-4">
            {["Nodes", "VMs", "Storage"].map((label) => (
              <div
                key={label}
                className="rounded-xl bg-white/5 px-4 py-3 text-center backdrop-blur-sm"
              >
                <p className="text-sm font-medium text-slate-300">{label}</p>
              </div>
            ))}
          </div>
        </motion.div>
      </div>

      {/* Login Form */}
      <div className="flex flex-1 items-center justify-center p-6 sm:p-12">
        <motion.div
          initial={{ opacity: 0, y: 12 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.4, ease: [0.25, 0.1, 0.25, 1] }}
          className="w-full max-w-md"
        >
          {/* Mobile logo */}
          <div className="mb-8 lg:hidden">
            <h1 className="text-2xl font-bold tracking-tight">VirtueStack</h1>
            <p className="text-sm text-muted-foreground">
              Administration Panel
            </p>
          </div>

          <Card className="border-0 shadow-xl sm:border">
            <CardHeader className="space-y-1">
              <CardTitle className="text-2xl font-bold tracking-tight">
                {requires2FA ? "Two-Factor Authentication" : "Admin Login"}
              </CardTitle>
              <CardDescription>
                {requires2FA
                  ? "Enter your 6-digit authentication code"
                  : "Sign in to your admin account"}
              </CardDescription>
            </CardHeader>

            <AnimatePresence mode="wait" initial={false}>
              {error && (
                <motion.div
                  key="error"
                  initial={{ opacity: 0, height: 0 }}
                  animate={{ opacity: 1, height: "auto" }}
                  exit={{ opacity: 0, height: 0 }}
                  className="mx-6 mb-4 overflow-hidden"
                >
                  <div className="rounded-lg bg-destructive/10 p-3 text-sm text-destructive">
                    {error}
                  </div>
                </motion.div>
              )}
            </AnimatePresence>

            <AnimatePresence mode="wait" initial={false}>
              {requires2FA ? (
                <motion.form
                  key="totp"
                  initial={{ opacity: 0, x: 20 }}
                  animate={{ opacity: 1, x: 0 }}
                  exit={{ opacity: 0, x: -20 }}
                  transition={{ duration: 0.2 }}
                  onSubmit={totpForm.handleSubmit(onTotpSubmit)}
                >
                  <CardContent className="space-y-4">
                    <div className="flex items-center justify-center py-4">
                      <div className="rounded-full bg-primary/10 p-4">
                        <Shield className="h-8 w-8 text-primary" />
                      </div>
                    </div>
                    <div className="space-y-2">
                      <Label htmlFor="totp_code">Authentication Code</Label>
                      <Input
                        id="totp_code"
                        type="text"
                        maxLength={6}
                        placeholder="000000"
                        disabled={isLoading}
                        {...totpForm.register("totp_code")}
                        className="text-center text-lg tracking-widest"
                      />
                      {totpForm.formState.errors.totp_code && (
                        <p className="text-sm text-destructive">
                          {totpForm.formState.errors.totp_code.message}
                        </p>
                      )}
                    </div>
                  </CardContent>
                  <CardFooter className="flex flex-col space-y-4">
                    <Button type="submit" className="w-full" disabled={isLoading}>
                      {isLoading && (
                        <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                      )}
                      {isLoading ? "Verifying..." : "Verify"}
                    </Button>
                    <Button
                      type="button"
                      variant="ghost"
                      className="text-sm text-muted-foreground"
                      onClick={() => reset2FA()}
                    >
                      Back to login
                    </Button>
                  </CardFooter>
                </motion.form>
              ) : (
                <motion.form
                  key="login"
                  initial={{ opacity: 0, x: -20 }}
                  animate={{ opacity: 1, x: 0 }}
                  exit={{ opacity: 0, x: 20 }}
                  transition={{ duration: 0.2 }}
                  onSubmit={loginForm.handleSubmit(onLoginSubmit)}
                >
                  <CardContent className="space-y-4">
                    <div className="space-y-2">
                      <Label htmlFor="email">Email</Label>
                      <Input
                        id="email"
                        type="email"
                        placeholder="admin@example.com"
                        disabled={isLoading}
                        {...loginForm.register("email")}
                      />
                      {loginForm.formState.errors.email && (
                        <p className="text-sm text-destructive">
                          {loginForm.formState.errors.email.message}
                        </p>
                      )}
                    </div>
                    <div className="space-y-2">
                      <Label htmlFor="password">Password</Label>
                      <Input
                        id="password"
                        type="password"
                        placeholder="Enter your password"
                        disabled={isLoading}
                        {...loginForm.register("password")}
                      />
                      {loginForm.formState.errors.password && (
                        <p className="text-sm text-destructive">
                          {loginForm.formState.errors.password.message}
                        </p>
                      )}
                    </div>
                  </CardContent>
                  <CardFooter>
                    <Button type="submit" className="w-full" disabled={isLoading}>
                      {isLoading && (
                        <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                      )}
                      {isLoading ? "Signing in..." : "Sign In"}
                    </Button>
                  </CardFooter>
                </motion.form>
              )}
            </AnimatePresence>
          </Card>
        </motion.div>
      </div>
    </div>
  );
}
```

- [ ] **Step 2: Commit**

```bash
cd /home/hiron/VirtueStack
git add webui/admin/app/login/page.tsx
git commit -m "feat(admin): redesign login with split layout, brand panel, and form animations

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```

---

### Task 16: Enhance customer login page with animations

**Files:**
- Modify: `webui/customer/app/login/page.tsx`

- [ ] **Step 1: Replace customer login page**

Replace `webui/customer/app/login/page.tsx` with:

```tsx
"use client";

import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import Link from "next/link";
import { Loader2, Shield, Cloud } from "lucide-react";
import { motion, AnimatePresence } from "motion/react";

import {
  Button,
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
  Input,
  Label,
} from "@virtuestack/ui";
import { useAuth } from "@/lib/auth-context";
import { OAuthButtons } from "@/components/oauth-buttons";

const OAUTH_GOOGLE_ENABLED =
  process.env.NEXT_PUBLIC_OAUTH_GOOGLE_ENABLED === "true";
const OAUTH_GITHUB_ENABLED =
  process.env.NEXT_PUBLIC_OAUTH_GITHUB_ENABLED === "true";

const loginSchema = z.object({
  email: z.string().email("Invalid email address"),
  password: z.string().min(12, "Password must be at least 12 characters"),
});

const totpSchema = z.object({
  totp_code: z
    .string()
    .length(6, "2FA code must be 6 digits")
    .regex(/^\d+$/, "Code must contain only numbers"),
});

type LoginFormData = z.infer<typeof loginSchema>;
type TotpFormData = z.infer<typeof totpSchema>;

export default function LoginPage() {
  const {
    login,
    verify2FA,
    requires2FA,
    tempToken,
    isLoading,
    error,
    clearError,
    reset2FA,
  } = useAuth();

  const loginForm = useForm<LoginFormData>({
    resolver: zodResolver(loginSchema),
  });

  const totpForm = useForm<TotpFormData>({
    resolver: zodResolver(totpSchema),
  });

  const onLoginSubmit = async (data: LoginFormData) => {
    clearError();
    await login(data);
  };

  const onTotpSubmit = async (data: TotpFormData) => {
    clearError();
    if (!tempToken) return;
    await verify2FA({
      temp_token: tempToken,
      totp_code: data.totp_code,
    });
  };

  return (
    <div className="flex min-h-screen">
      {/* Brand Panel */}
      <div className="hidden lg:flex lg:w-1/2 items-center justify-center bg-gradient-to-br from-blue-600 via-blue-700 to-indigo-800 p-12">
        <motion.div
          initial={{ opacity: 0, y: 20 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.6, ease: [0.25, 0.1, 0.25, 1] }}
          className="max-w-md text-center"
        >
          <div className="mx-auto mb-8 flex h-16 w-16 items-center justify-center rounded-2xl bg-white/10 backdrop-blur-sm">
            <Cloud className="h-8 w-8 text-white" />
          </div>
          <h1 className="text-4xl font-bold tracking-tight text-white">
            VirtueStack
          </h1>
          <p className="mt-3 text-lg text-blue-200">Customer Portal</p>
          <p className="mt-6 text-sm leading-relaxed text-blue-200/80">
            Manage your virtual machines, backups, and account settings from one
            place.
          </p>
        </motion.div>
      </div>

      {/* Form */}
      <div className="flex flex-1 items-center justify-center p-6 sm:p-12">
        <motion.div
          initial={{ opacity: 0, y: 12 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.4, ease: [0.25, 0.1, 0.25, 1] }}
          className="w-full max-w-md"
        >
          <div className="mb-8 lg:hidden">
            <h1 className="text-2xl font-bold tracking-tight">VirtueStack</h1>
            <p className="text-sm text-muted-foreground">Customer Portal</p>
          </div>

          <Card className="border-0 shadow-xl sm:border">
            <CardHeader className="space-y-1">
              <CardTitle className="text-2xl font-bold tracking-tight">
                {requires2FA ? "Two-Factor Authentication" : "Welcome back"}
              </CardTitle>
              <CardDescription>
                {requires2FA
                  ? "Enter your 6-digit authentication code"
                  : "Sign in to your account"}
              </CardDescription>
            </CardHeader>

            <AnimatePresence mode="wait" initial={false}>
              {error && (
                <motion.div
                  key="error"
                  initial={{ opacity: 0, height: 0 }}
                  animate={{ opacity: 1, height: "auto" }}
                  exit={{ opacity: 0, height: 0 }}
                  className="mx-6 mb-4 overflow-hidden"
                >
                  <div className="rounded-lg bg-destructive/10 p-3 text-sm text-destructive">
                    {error}
                  </div>
                </motion.div>
              )}
            </AnimatePresence>

            <AnimatePresence mode="wait" initial={false}>
              {requires2FA ? (
                <motion.form
                  key="totp"
                  initial={{ opacity: 0, x: 20 }}
                  animate={{ opacity: 1, x: 0 }}
                  exit={{ opacity: 0, x: -20 }}
                  transition={{ duration: 0.2 }}
                  onSubmit={totpForm.handleSubmit(onTotpSubmit)}
                >
                  <CardContent className="space-y-4">
                    <div className="flex items-center justify-center py-4">
                      <div className="rounded-full bg-primary/10 p-4">
                        <Shield className="h-8 w-8 text-primary" />
                      </div>
                    </div>
                    <div className="space-y-2">
                      <Label htmlFor="totp_code">Authentication Code</Label>
                      <Input
                        id="totp_code"
                        type="text"
                        maxLength={6}
                        placeholder="000000"
                        disabled={isLoading}
                        {...totpForm.register("totp_code")}
                        className="text-center text-lg tracking-widest"
                      />
                      {totpForm.formState.errors.totp_code && (
                        <p className="text-sm text-destructive">
                          {totpForm.formState.errors.totp_code.message}
                        </p>
                      )}
                    </div>
                  </CardContent>
                  <CardFooter className="flex flex-col space-y-4">
                    <Button type="submit" className="w-full" disabled={isLoading}>
                      {isLoading && (
                        <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                      )}
                      {isLoading ? "Verifying..." : "Verify"}
                    </Button>
                    <Button
                      type="button"
                      variant="ghost"
                      className="text-sm text-muted-foreground"
                      onClick={() => reset2FA()}
                    >
                      Back to login
                    </Button>
                  </CardFooter>
                </motion.form>
              ) : (
                <motion.form
                  key="login"
                  initial={{ opacity: 0, x: -20 }}
                  animate={{ opacity: 1, x: 0 }}
                  exit={{ opacity: 0, x: 20 }}
                  transition={{ duration: 0.2 }}
                  onSubmit={loginForm.handleSubmit(onLoginSubmit)}
                >
                  <CardContent className="space-y-4">
                    <div className="space-y-2">
                      <Label htmlFor="email">Email</Label>
                      <Input
                        id="email"
                        type="email"
                        placeholder="you@example.com"
                        disabled={isLoading}
                        {...loginForm.register("email")}
                      />
                      {loginForm.formState.errors.email && (
                        <p className="text-sm text-destructive">
                          {loginForm.formState.errors.email.message}
                        </p>
                      )}
                    </div>
                    <div className="space-y-2">
                      <Label htmlFor="password">Password</Label>
                      <Input
                        id="password"
                        type="password"
                        placeholder="Enter your password"
                        disabled={isLoading}
                        {...loginForm.register("password")}
                      />
                      {loginForm.formState.errors.password && (
                        <p className="text-sm text-destructive">
                          {loginForm.formState.errors.password.message}
                        </p>
                      )}
                    </div>
                  </CardContent>
                  <CardFooter className="flex flex-col space-y-4">
                    <Button type="submit" className="w-full" disabled={isLoading}>
                      {isLoading && (
                        <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                      )}
                      {isLoading ? "Signing in..." : "Sign In"}
                    </Button>
                    <OAuthButtons
                      googleEnabled={OAUTH_GOOGLE_ENABLED}
                      githubEnabled={OAUTH_GITHUB_ENABLED}
                      disabled={isLoading}
                    />
                    <Link
                      href="/forgot-password"
                      className="text-sm text-muted-foreground hover:text-primary transition-colors"
                    >
                      Forgot your password?
                    </Link>
                  </CardFooter>
                </motion.form>
              )}
            </AnimatePresence>
          </Card>
        </motion.div>
      </div>
    </div>
  );
}
```

- [ ] **Step 2: Commit**

```bash
cd /home/hiron/VirtueStack
git add webui/customer/app/login/page.tsx
git commit -m "feat(customer): redesign login with split layout, brand panel, and form animations

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```

---

## Phase 4: Dashboard

### Task 17: Redesign admin dashboard with animated cards

**Files:**
- Modify: `webui/admin/app/dashboard/page.tsx`

- [ ] **Step 1: Replace dashboard page.tsx**

Replace `webui/admin/app/dashboard/page.tsx` with:

```tsx
"use client";

import { useState, useEffect, useCallback } from "react";
import { motion } from "motion/react";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
  Button,
} from "@virtuestack/ui";
import {
  Server,
  Users,
  Monitor,
  Plus,
  FileSpreadsheet,
  HardDrive,
  Activity,
  Loader2,
  AlertTriangle,
  Network,
} from "lucide-react";
import { useRouter } from "next/navigation";
import {
  adminVMsApi,
  adminNodesApi,
  adminCustomersApi,
  adminAuditLogsApi,
  type AuditLog,
} from "@/lib/api-client";
import { PageTransition } from "@/components/page-transition";
import { AnimatedCard } from "@/components/animated-card";
import { Skeleton } from "@/components/skeleton";
import {
  fadeUp,
  staggerContainer,
  easeTransition,
} from "@/lib/animations";

interface DashboardStats {
  totalVMs: number;
  totalNodes: number;
  totalCustomers: number;
}

interface ActivityItem {
  id: string;
  action: string;
  resource: string;
  timestamp: string;
  type: "info" | "warning" | "success" | "error";
}

function formatRelativeTime(dateStr: string): string {
  const now = Date.now();
  const then = new Date(dateStr).getTime();
  const diff = Math.floor((now - then) / 1000);
  if (diff < 60) return "just now";
  if (diff < 3600) return `${Math.floor(diff / 60)}m ago`;
  if (diff < 86400) return `${Math.floor(diff / 3600)}h ago`;
  return `${Math.floor(diff / 86400)}d ago`;
}

const statCardConfig = [
  {
    key: "totalVMs",
    title: "Virtual Machines",
    icon: Monitor,
    description: "Total VMs",
    color: "text-blue-500",
    bg: "bg-blue-500/10",
  },
  {
    key: "totalNodes",
    title: "Nodes",
    icon: HardDrive,
    description: "Hypervisor nodes",
    color: "text-emerald-500",
    bg: "bg-emerald-500/10",
  },
  {
    key: "totalCustomers",
    title: "Customers",
    icon: Users,
    description: "Active accounts",
    color: "text-violet-500",
    bg: "bg-violet-500/10",
  },
] as const;

function DashboardSkeleton() {
  return (
    <div className="mx-auto max-w-7xl space-y-8">
      <div>
        <Skeleton className="h-9 w-48" />
        <Skeleton className="mt-2 h-5 w-72" />
      </div>
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
        {[1, 2, 3].map((i) => (
          <Skeleton key={i} className="h-32 rounded-xl" />
        ))}
      </div>
      <div className="grid gap-6 lg:grid-cols-2">
        <Skeleton className="h-80 rounded-xl" />
        <Skeleton className="h-80 rounded-xl" />
      </div>
    </div>
  );
}

export default function DashboardPage() {
  const router = useRouter();
  const [stats, setStats] = useState<DashboardStats>({
    totalVMs: 0,
    totalNodes: 0,
    totalCustomers: 0,
  });
  const [activities, setActivities] = useState<ActivityItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const loadData = useCallback(async function loadData() {
    try {
      const results = await Promise.allSettled([
        adminVMsApi.getVMs(),
        adminNodesApi.getNodes(),
        adminCustomersApi.getCustomers(),
        adminAuditLogsApi.getAuditLogs(),
      ]);

      const vmsResult =
        results[0].status === "fulfilled" ? results[0].value : { data: [] };
      const nodesResult =
        results[1].status === "fulfilled" ? results[1].value : { data: [] };
      const customersResult =
        results[2].status === "fulfilled" ? results[2].value : { data: [] };
      const logsResult =
        results[3].status === "fulfilled" ? results[3].value : { data: [] };
      const logs = logsResult.data || [];

      setStats({
        totalVMs: (vmsResult.data || []).length,
        totalNodes: (nodesResult.data || []).length,
        totalCustomers: (customersResult.data || []).length,
      });

      const mappedActivities: ActivityItem[] = (logs as AuditLog[])
        .slice(0, 8)
        .map((log) => {
          let type: "info" | "warning" | "success" | "error" = "info";
          if (!log.success) type = "error";
          else if (log.action.includes("create") || log.action.includes("start"))
            type = "success";
          else if (log.action.includes("delete") || log.action.includes("stop"))
            type = "warning";

          return {
            id: log.id,
            action: log.action,
            resource: log.resource_id || log.resource_type,
            timestamp: log.timestamp,
            type,
          };
        });
      setActivities(mappedActivities);

      const failedCount = results.filter((r) => r.status === "rejected").length;
      if (failedCount > 0) {
        setError(`Failed to load ${failedCount} dashboard data source(s)`);
      }
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    loadData();
  }, [loadData]);

  useEffect(() => {
    const interval = setInterval(() => {
      loadData();
    }, 60000);
    return () => clearInterval(interval);
  }, [loadData]);

  if (loading) {
    return (
      <div className="min-h-screen p-6 md:p-8">
        <DashboardSkeleton />
      </div>
    );
  }

  return (
    <PageTransition>
      <div className="min-h-screen p-6 md:p-8">
        {error && (
          <div className="mx-auto max-w-7xl mb-6 flex items-center gap-2 rounded-lg border border-yellow-500/30 bg-yellow-500/10 p-3 text-sm text-yellow-700 dark:text-yellow-400">
            <AlertTriangle className="h-4 w-4 shrink-0" />
            {error}
          </div>
        )}

        <div className="mx-auto max-w-7xl space-y-8">
          {/* Header */}
          <div className="flex flex-col gap-4 md:flex-row md:items-center md:justify-between">
            <div>
              <h1 className="text-3xl font-bold tracking-tight">Dashboard</h1>
              <p className="text-muted-foreground">
                System overview and recent activity
              </p>
            </div>
            <div className="flex gap-2">
              <Button
                variant="outline"
                size="default"
                onClick={() => router.push("/audit-logs")}
              >
                <Activity className="mr-2 h-4 w-4" />
                View Logs
              </Button>
            </div>
          </div>

          {/* Stat Cards */}
          <motion.div
            variants={staggerContainer}
            initial="hidden"
            animate="visible"
            className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3"
          >
            {statCardConfig.map((stat, i) => {
              const Icon = stat.icon;
              const value = stats[stat.key as keyof DashboardStats];
              return (
                <motion.div key={stat.key} variants={fadeUp}>
                  <AnimatedCard delay={i * 0.05}>
                    <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
                      <CardTitle className="text-sm font-medium">
                        {stat.title}
                      </CardTitle>
                      <div className={`rounded-lg p-2 ${stat.bg}`}>
                        <Icon className={`h-4 w-4 ${stat.color}`} />
                      </div>
                    </CardHeader>
                    <CardContent>
                      <div className="text-3xl font-bold tabular-nums">
                        {value}
                      </div>
                      <p className="mt-1 text-xs text-muted-foreground">
                        {stat.description}
                      </p>
                    </CardContent>
                  </AnimatedCard>
                </motion.div>
              );
            })}
          </motion.div>

          {/* Activity + Quick Actions */}
          <div className="grid gap-6 lg:grid-cols-2">
            {/* Activity Feed */}
            <AnimatedCard hoverLift={false} delay={0.2}>
              <CardHeader>
                <CardTitle className="flex items-center gap-2">
                  <Activity className="h-5 w-5" />
                  Recent Activity
                </CardTitle>
                <CardDescription>Latest events across the system</CardDescription>
              </CardHeader>
              <CardContent>
                {activities.length === 0 ? (
                  <div className="py-8 text-center text-sm text-muted-foreground">
                    No recent activity found.
                  </div>
                ) : (
                  <motion.div
                    variants={staggerContainer}
                    initial="hidden"
                    animate="visible"
                    className="space-y-1"
                  >
                    {activities.map((activity) => (
                      <motion.div
                        key={activity.id}
                        variants={fadeUp}
                        className="flex items-start justify-between gap-4 rounded-lg px-2 py-2.5 transition-colors hover:bg-muted/50"
                      >
                        <div className="flex items-start gap-3">
                          <div
                            className={`mt-1.5 h-2 w-2 shrink-0 rounded-full ${
                              activity.type === "success"
                                ? "bg-emerald-500"
                                : activity.type === "error"
                                  ? "bg-red-500"
                                  : activity.type === "warning"
                                    ? "bg-yellow-500"
                                    : "bg-blue-500"
                            }`}
                          />
                          <div>
                            <p className="text-sm font-medium">
                              {activity.action}
                            </p>
                            <p className="text-xs text-muted-foreground">
                              {activity.resource}
                            </p>
                          </div>
                        </div>
                        <span className="shrink-0 text-xs text-muted-foreground">
                          {formatRelativeTime(activity.timestamp)}
                        </span>
                      </motion.div>
                    ))}
                  </motion.div>
                )}
              </CardContent>
            </AnimatedCard>

            {/* Quick Actions */}
            <AnimatedCard hoverLift={false} delay={0.25}>
              <CardHeader>
                <CardTitle className="flex items-center gap-2">
                  <Monitor className="h-5 w-5" />
                  Quick Actions
                </CardTitle>
                <CardDescription>Common administrative tasks</CardDescription>
              </CardHeader>
              <CardContent>
                <div className="grid gap-2 sm:grid-cols-2">
                  {[
                    {
                      icon: Plus,
                      label: "Add Node",
                      href: "/nodes",
                      color: "text-emerald-500",
                      bg: "bg-emerald-500/10",
                    },
                    {
                      icon: FileSpreadsheet,
                      label: "Create Plan",
                      href: "/plans",
                      color: "text-blue-500",
                      bg: "bg-blue-500/10",
                    },
                    {
                      icon: Server,
                      label: "Provision VM",
                      href: "/vms",
                      color: "text-violet-500",
                      bg: "bg-violet-500/10",
                    },
                    {
                      icon: Users,
                      label: "Add Customer",
                      href: "/customers",
                      color: "text-amber-500",
                      bg: "bg-amber-500/10",
                    },
                    {
                      icon: Network,
                      label: "Manage IPs",
                      href: "/ip-sets",
                      color: "text-cyan-500",
                      bg: "bg-cyan-500/10",
                    },
                    {
                      icon: HardDrive,
                      label: "Storage",
                      href: "/storage-backends",
                      color: "text-rose-500",
                      bg: "bg-rose-500/10",
                    },
                  ].map((action) => {
                    const Icon = action.icon;
                    return (
                      <Button
                        key={action.label}
                        variant="outline"
                        className="h-auto justify-start gap-3 px-4 py-3 transition-all duration-150 hover:shadow-sm"
                        onClick={() => router.push(action.href)}
                      >
                        <div className={`rounded-md p-1.5 ${action.bg}`}>
                          <Icon className={`h-4 w-4 ${action.color}`} />
                        </div>
                        <span className="text-sm font-medium">
                          {action.label}
                        </span>
                      </Button>
                    );
                  })}
                </div>
              </CardContent>
            </AnimatedCard>
          </div>
        </div>
      </div>
    </PageTransition>
  );
}
```

- [ ] **Step 2: Commit**

```bash
cd /home/hiron/VirtueStack
git add webui/admin/app/dashboard/page.tsx
git commit -m "feat(admin): redesign dashboard with animated cards, colored icons, skeleton loading, and relative timestamps

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```

---

### Task 18: Add notification bell to admin header and update layout

**Files:**
- Modify: `webui/admin/app/dashboard/layout.tsx`

- [ ] **Step 1: Replace dashboard layout.tsx**

Replace `webui/admin/app/dashboard/layout.tsx` with:

```tsx
"use client";

import { useState } from "react";
import { Sidebar } from "@/components/sidebar";
import { MobileNav } from "@/components/mobile-nav";
import { ThemeToggle } from "@/components/theme-toggle";
import { NotificationBell } from "@/components/notification-bell";
import { RequireAuth } from "@/lib/require-auth";

export default function DashboardLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false);

  return (
    <RequireAuth>
      <div className="flex min-h-screen">
        <div className="hidden md:block">
          <Sidebar
            collapsed={sidebarCollapsed}
            onToggle={() => setSidebarCollapsed(!sidebarCollapsed)}
          />
        </div>

        <div className="flex flex-1 flex-col">
          <header className="sticky top-0 z-10 flex h-14 items-center gap-4 border-b bg-background/80 px-6 backdrop-blur-md supports-[backdrop-filter]:bg-background/60">
            <MobileNav />
            <div className="flex-1" />
            <div className="ml-auto flex items-center gap-2">
              <NotificationBell />
              <ThemeToggle />
            </div>
          </header>

          <main className="flex-1 overflow-auto">{children}</main>
        </div>
      </div>
    </RequireAuth>
  );
}
```

Key changes:
- Added `NotificationBell` to header (visible at all widths)
- Changed `bg-background/95` to `bg-background/80` with `backdrop-blur-md` for stronger glass effect
- Removed `p-6` from main (pages handle their own padding)

- [ ] **Step 2: Update customer layout similarly**

Replace `webui/customer/app/vms/layout.tsx` with:

```tsx
"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { Search } from "lucide-react";

import { Sidebar } from "@/components/sidebar";
import { MobileNav } from "@/components/mobile-nav";
import { ThemeToggle } from "@/components/theme-toggle";
import { NotificationBell } from "@/components/notification-bell";
import { RequireAuth } from "@/lib/require-auth";

export default function VMSLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  const router = useRouter();
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false);
  const [searchQuery, setSearchQuery] = useState("");

  const handleSearch = (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    if (searchQuery.trim()) {
      router.push(`/vms?search=${encodeURIComponent(searchQuery.trim())}`);
    }
  };

  return (
    <RequireAuth>
      <div className="flex min-h-screen">
        <div className="hidden md:block">
          <Sidebar
            collapsed={sidebarCollapsed}
            onToggle={() => setSidebarCollapsed(!sidebarCollapsed)}
          />
        </div>

        <div className="flex flex-1 flex-col">
          <header className="sticky top-0 z-10 flex h-14 items-center gap-4 border-b bg-background/80 px-6 backdrop-blur-md supports-[backdrop-filter]:bg-background/60">
            <MobileNav />

            <div className="flex-1 md:flex-none">
              <form onSubmit={handleSearch} className="relative w-full max-w-sm">
                <Search className="absolute left-2.5 top-2.5 h-4 w-4 text-muted-foreground" />
                <input
                  type="search"
                  placeholder="Search VMs..."
                  className="h-9 w-full rounded-lg border border-input bg-background pl-8 pr-4 text-sm outline-none placeholder:text-muted-foreground focus:border-ring focus:ring-1 focus:ring-ring transition-all"
                  value={searchQuery}
                  onChange={(e) => setSearchQuery(e.target.value)}
                />
              </form>
            </div>

            <div className="ml-auto flex items-center gap-2">
              <NotificationBell />
              <ThemeToggle />
            </div>
          </header>

          <main className="flex-1 overflow-auto p-6">{children}</main>
        </div>
      </div>
    </RequireAuth>
  );
}
```

- [ ] **Step 3: Commit**

```bash
cd /home/hiron/VirtueStack
git add webui/admin/app/dashboard/layout.tsx webui/customer/app/vms/layout.tsx
git commit -m "feat(ui): add notification bell to headers and improve glass blur effect

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```

---

### Task 19: Final build verification

- [ ] **Step 1: Build admin UI**

```bash
cd /home/hiron/VirtueStack/webui/admin && npm run build
```

Expected: Build succeeds with no errors.

- [ ] **Step 2: Build customer UI**

```bash
cd /home/hiron/VirtueStack/webui/customer && npm run build
```

Expected: Build succeeds with no errors.

- [ ] **Step 3: Run lints**

```bash
cd /home/hiron/VirtueStack/webui/admin && npm run lint
cd /home/hiron/VirtueStack/webui/customer && npm run lint
```

Expected: No new lint errors.

- [ ] **Step 4: Run type checks**

```bash
cd /home/hiron/VirtueStack/webui/admin && npm run type-check
cd /home/hiron/VirtueStack/webui/customer && npm run type-check
```

Expected: No type errors.

- [ ] **Step 5: Fix any errors and commit fixes**

If any build/lint/type errors, fix them and commit with:

```bash
git commit -m "fix(ui): resolve build errors from UI modernization

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```
