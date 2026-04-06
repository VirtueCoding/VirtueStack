# UI Modernization Design Spec

**Date:** 2026-04-01
**Scope:** Admin portal (`webui/admin/`) and Customer portal (`webui/customer/`)
**Goal:** Transform both UIs from functional-but-plain to modern, elegant, and polished — with smooth transitions, better mobile experience, and refined dark/light theming.

---

## Problem Statement

Both VirtueStack portals use default shadcn/ui styling with minimal customization. Specific issues:

1. **No custom typography** — relies on system font stack (`font-sans`), looks generic
2. **Flat, lifeless interactions** — no hover effects, no page transitions, no enter animations
3. **Jarring theme switching** — `disableTransitionOnChange` causes instant color snap
4. **Bland color palette** — admin uses default zinc (gray), customer uses basic blue
5. **Weak mobile experience** — no bottom navigation, small touch targets, tables don't adapt
6. **No visual hierarchy** — stat cards, lists, and detail pages all look the same weight
7. **No loading polish** — raw spinners instead of skeleton shimmer effects
8. **Plain login pages** — centered card on white/black background, no branding presence

---

## Approach: Motion + Design Token Refinement

Add the `motion` library (Framer Motion) for declarative animations and refine the design system at the CSS variable level. This approach:

- Preserves all existing component structure and functionality
- Works within the existing shadcn/ui + Tailwind architecture
- Adds ~40KB gzipped (motion library) — acceptable for the quality improvement
- Requires no component library replacement

**Why not CSS-only?** CSS transitions cover hover/focus states but cannot do: page transitions with AnimatePresence, staggered list animations, spring physics, layout animations, or scroll-triggered reveals. Motion provides all of these with a declarative API.

**Why not a full redesign?** The existing component structure (sidebar + header + content) is sound. The problem is styling and polish, not architecture. A full redesign would be high-risk for low marginal benefit.

---

## 1. Design System Foundation

### 1.1 Typography — Geist Font Family

Add the Geist Sans font (designed by Vercel for Next.js). It's modern, highly legible, and purpose-built for UI work.

**Implementation:**
- Use `next/font/google` to load `Geist` and `Geist Mono`
- Apply via CSS variable `--font-sans` and `--font-mono`
- Both UIs share the same font configuration

**Type scale refinements:**
- Page titles: `text-3xl font-bold tracking-tight` (already used, keep)
- Section headers: `text-xl font-semibold`
- Card titles: `text-base font-semibold`
- Body: `text-sm` (default)
- Captions/labels: `text-xs text-muted-foreground`

### 1.2 Color Palette — Refined Tokens

**Admin Portal** — Slate + Indigo accent:
```css
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
  --card: 224 71.4% 4.1%;
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
  --sidebar: 224 71.4% 4.1%;
  --sidebar-foreground: 210 20% 98%;
  --sidebar-primary: 210 20% 98%;
  --sidebar-primary-foreground: 220.9 39.3% 11%;
  --sidebar-accent: 215 27.9% 16.9%;
  --sidebar-accent-foreground: 210 20% 98%;
  --sidebar-border: 215 27.9% 16.9%;
}
```

**Customer Portal** — Refined Blue:
```css
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
  --success: 142 76% 36%;
  --success-foreground: 0 0% 100%;
  --warning: 38 92% 50%;
  --warning-foreground: 0 0% 0%;
  --info: 221 83% 53%;
  --info-foreground: 210 40% 98%;
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
  --success: 142 71% 45%;
  --success-foreground: 0 0% 100%;
  --warning: 38 92% 50%;
  --warning-foreground: 0 0% 0%;
  --info: 217 91% 60%;
  --info-foreground: 210 40% 98%;
  --sidebar: 222.2 84% 4.9%;
  --sidebar-foreground: 210 40% 98%;
  --sidebar-primary: 217.2 91.2% 59.8%;
  --sidebar-primary-foreground: 222.2 47.4% 11.2%;
  --sidebar-accent: 217.2 32.6% 17.5%;
  --sidebar-accent-foreground: 210 40% 98%;
  --sidebar-border: 217.2 32.6% 17.5%;
}
```

Key changes from current:
- Admin gets a cooler slate tone (hue 220) instead of pure neutral (hue 240) — more sophisticated
- Customer dark mode cards get a slightly elevated surface (`217.2 32.6% 11%`) instead of matching background — creates depth
- Both get `--chart-1` through `--chart-5` tokens for chart theming
- Customer gets semantic `--success`, `--warning`, `--info` tokens for status indicators
- Both get `--sidebar-*` tokens for independent sidebar theming
- Radius bumped from `0.5rem` to `0.625rem` for softer, more modern feel

### 1.3 Global CSS Additions

Add to both `globals.css`:

```css
@layer base {
  * {
    @apply border-border;
  }

  body {
    @apply bg-background text-foreground;
    transition: background-color 0.3s ease, color 0.3s ease;
  }

  /* Smooth theme transitions for all elements */
  *,
  *::before,
  *::after {
    transition-property: background-color, border-color, color, fill, stroke, box-shadow;
    transition-duration: 0.2s;
    transition-timing-function: ease;
  }

  /* Disable transitions for animations that manage their own timing */
  [data-motion],
  [data-radix-popper-content-wrapper] *,
  [data-state="open"],
  [data-state="closed"] {
    transition: none !important;
  }
}
```

### 1.4 Tailwind Config Additions

Add to both `tailwind.config.ts`:

```typescript
extend: {
  // ... existing colors ...
  keyframes: {
    // ... existing accordion keyframes ...
    'fade-in': {
      from: { opacity: '0' },
      to: { opacity: '1' },
    },
    'fade-up': {
      from: { opacity: '0', transform: 'translateY(8px)' },
      to: { opacity: '1', transform: 'translateY(0)' },
    },
    'slide-in-right': {
      from: { transform: 'translateX(100%)' },
      to: { transform: 'translateX(0)' },
    },
    'shimmer': {
      from: { backgroundPosition: '200% 0' },
      to: { backgroundPosition: '-200% 0' },
    },
    'pulse-soft': {
      '0%, 100%': { opacity: '1' },
      '50%': { opacity: '0.7' },
    },
    'scale-in': {
      from: { opacity: '0', transform: 'scale(0.95)' },
      to: { opacity: '1', transform: 'scale(1)' },
    },
  },
  animation: {
    // ... existing accordion animations ...
    'fade-in': 'fade-in 0.3s ease-out',
    'fade-up': 'fade-up 0.4s ease-out',
    'slide-in-right': 'slide-in-right 0.3s ease-out',
    'shimmer': 'shimmer 2s linear infinite',
    'pulse-soft': 'pulse-soft 2s ease-in-out infinite',
    'scale-in': 'scale-in 0.2s ease-out',
  },
}
```

---

## 2. Animation System

### 2.1 New Dependency: `motion`

Install `motion` (Framer Motion v12+) in both UIs and the shared `@virtuestack/ui` package:
```bash
cd webui/admin && npm install motion
cd webui/customer && npm install motion
cd webui/packages/ui && npm install motion  # For animated shared components
```

### 2.2 Shared Animation Utilities

Create `lib/animations.ts` in both UIs (identical content):

```typescript
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
  ease: [0.25, 0.1, 0.25, 1],
};
```

### 2.3 Page Transition Wrapper

Create `components/page-transition.tsx`:

```typescript
"use client";

import { motion } from "motion/react";

export function PageTransition({ children }: { children: React.ReactNode }) {
  return (
    <motion.div
      initial={{ opacity: 0, y: 8 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.3, ease: [0.25, 0.1, 0.25, 1] }}
    >
      {children}
    </motion.div>
  );
}
```

Every page wraps its content in `<PageTransition>` for consistent enter animations.

### 2.4 Animated Card Component

Create `components/animated-card.tsx`:

```typescript
"use client";

import { motion } from "motion/react";
import { Card } from "@virtuestack/ui";
import { cn } from "@/lib/utils";
import { forwardRef } from "react";

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
        transition={{ duration: 0.3, delay, ease: [0.25, 0.1, 0.25, 1] }}
        whileHover={hoverLift ? { y: -2, boxShadow: "0 8px 30px rgba(0,0,0,0.08)" } : undefined}
        className={cn(
          "transition-shadow duration-200",
          hoverLift && "cursor-pointer",
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

### 2.5 Skeleton Shimmer Component

Create `components/skeleton.tsx`:

```typescript
import { cn } from "@/lib/utils";

export function Skeleton({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      className={cn(
        "rounded-md bg-muted",
        "bg-gradient-to-r from-muted via-muted/60 via-muted bg-[length:400%_100%] animate-shimmer",
        className
      )}
      {...props}
    />
  );
}
```

---

## 3. Component-Level Improvements

### 3.1 Sidebar Redesign

**Changes from current:**
- Add nav group sections with labels (e.g., "Infrastructure", "Customers", "System")
- Active item indicator: left border pill + tinted background instead of solid primary fill
- Hover: subtle background tint with 150ms transition
- Collapsed mode: tooltip on hover showing item label
- Smooth width transition already exists (duration-300) — keep it
- Add subtle top gradient in dark mode for depth

**Active state (before):**
```
bg-primary text-primary-foreground
```

**Active state (after):**
```
bg-primary/10 text-primary border-l-2 border-primary font-semibold
```

This is lighter, more modern, and doesn't obscure the icon color.

**Nav group structure (admin):**
```
Overview
  └ Dashboard

Infrastructure
  ├ Nodes
  ├ Storage
  ├ Failover
  └ Templates

Virtual Machines
  ├ VMs
  ├ Plans
  └ IP Sets

Customers
  ├ Customers
  ├ Billing
  └ Invoices

System
  ├ Backups
  ├ Schedules
  ├ Provisioning Keys
  └ Audit Logs
```

### 3.2 Header Improvements

- Keep existing backdrop blur (good)
- Add breadcrumb navigation below the header bar for pages deeper than root
- Improve search: add `Cmd+K` / `Ctrl+K` shortcut hint badge next to search input
- Add notification bell to header (move from sidebar for visibility at all widths)

### 3.3 Theme Toggle — Animated

Replace the dropdown-based toggle with an animated icon button that cycles through modes:

```
Light (Sun) → Dark (Moon) → System (Monitor) → Light ...
```

The icon morphs with a rotation + scale animation on each click. Remove `disableTransitionOnChange` from the `ThemeProvider` so the entire UI transitions smoothly over 200ms.

### 3.4 Card Hover Effects

All clickable cards (stat cards, VM cards, node cards) get:
- `hover:shadow-md` — shadow lifts on hover
- `hover:-translate-y-0.5` — subtle 2px lift
- `transition-all duration-200` — smooth transition
- In dark mode: `hover:border-primary/20` — subtle border glow

### 3.5 Table Improvements

- Sticky table header with backdrop blur
- Row hover: `hover:bg-muted/50` with transition
- Staggered row entry animation (fade-in from left, 30ms stagger)
- Empty state: illustration + message instead of plain text
- Better responsive: on mobile (`<md`), tables render as stacked card list

### 3.6 Dialog/Sheet Animations

- Entry: scale from 0.95 + fade in (200ms ease-out)
- Exit: scale to 0.97 + fade out (150ms ease-in)
- Overlay: fade in/out (200ms)
- These are already handled by Radix but can be enhanced with CSS

### 3.7 Toast Notifications

- Slide in from right with spring physics
- Exit: slide right + fade out
- Success: green left border accent
- Error: red left border accent
- Add progress bar showing auto-dismiss timer

### 3.8 Loading States

Replace all `<Loader2 className="animate-spin" />` full-page loading states with:
- Skeleton shimmer layouts that match the page structure
- Dashboard: 3 skeleton stat cards + skeleton activity list
- List pages: skeleton table rows
- Detail pages: skeleton content blocks

---

## 4. Page-Level Improvements

### 4.1 Login Pages

**Current:** Plain centered card on empty background.

**New design:**
- Split layout on desktop: left side has brand panel (gradient background + logo + tagline), right side has login form
- On mobile: full-width form with subtle gradient header
- Form fields: larger padding, smooth focus ring animation
- Submit button: loading state with animated spinner inside
- Error messages: slide-down animation with red-tinted background
- 2FA step: smooth crossfade transition between login and TOTP forms

**Admin login brand panel:**
```
Background: linear-gradient(135deg, hsl(var(--primary)) 0%, hsl(220 60% 20%) 100%)
Logo: "VirtueStack" text in white, large
Tagline: "Infrastructure Management" in white/70%
```

**Customer login brand panel:**
```
Background: linear-gradient(135deg, hsl(221 83% 53%) 0%, hsl(217 91% 40%) 100%)
Logo: "VirtueStack" text in white, large
Tagline: "Customer Portal" in white/70%
```

### 4.2 Dashboard

**Stat cards:**
- Add colored icon backgrounds (e.g., blue circle for VMs, green for nodes, purple for customers)
- Animated count-up on load (numbers animate from 0 to final value)
- Subtle gradient border on hover

**Activity feed:**
- Staggered entry animation for each activity item
- Better timeline-style layout with vertical line connector
- Colored dots match activity type (green=success, red=error, yellow=warning, blue=info)
- Relative timestamps ("2 minutes ago" instead of full datetime)

**Quick actions:**
- Card grid instead of stacked buttons
- Each action card has an icon, title, and brief description
- Hover: lift + shadow effect

### 4.3 List Pages (VMs, Nodes, Customers, etc.)

- Page header with title + description + action buttons (consistent across all list pages)
- Filter bar with animated expand/collapse
- Table with staggered row entry
- Pagination with current page indicator
- Empty state with relevant illustration
- Mobile: card list view instead of table

### 4.4 Detail Pages (VM Detail, Node Detail, etc.)

- Tabbed interface with animated tab indicator (sliding underline)
- Content fade transition when switching tabs
- Status badges with pulse animation for active/running states
- Action buttons grouped in a toolbar with icon + text

---

## 5. Mobile Experience

### 5.1 Navigation

**Bottom tab bar** on mobile (replaces hamburger menu as primary nav):
- Show 4-5 key nav items as bottom tabs with icons + labels
- "More" tab opens the full sheet menu
- Active tab has colored icon + subtle indicator dot
- Fixed at bottom with safe-area padding for notched phones

**Admin bottom tabs:** Dashboard, VMs, Nodes, Customers, More
**Customer bottom tabs:** VMs, Backups, Settings, More

The existing sheet-based mobile nav remains accessible from the "More" tab and the hamburger icon in the header.

### 5.2 Touch Targets

- Minimum 44px height for all interactive elements on mobile
- Increase button padding on mobile: `py-3` instead of `py-2`
- Larger icon sizes in navigation: `h-5 w-5` instead of `h-4 w-4`

### 5.3 Responsive Tables

On screens below `md` breakpoint, tables transform into a stacked card layout:
- Each row becomes a card
- Column headers become inline labels
- Action buttons stack or use a dropdown menu
- Maintains the same data, just different visual presentation

### 5.4 Swipe Gestures

- Swipe left on table cards to reveal action buttons (delete, edit)
- Pull-to-refresh on list pages (triggers TanStack Query refetch)

---

## 6. Dark Mode Refinements

### 6.1 Smooth Theme Transition

Remove `disableTransitionOnChange` from ThemeProvider in both `providers.tsx` files. The CSS transitions added in Section 1.3 handle smooth 200ms color transitions for all elements.

### 6.2 Dark Mode Surface Hierarchy

Use slightly different surface colors to create depth in dark mode:
- Background (deepest): `--background`
- Card surface (elevated): `--card` — slightly lighter than background
- Popover/dialog (highest): `--popover` — slightly lighter than card

### 6.3 Dark Mode Specific Enhancements

- Sidebar: subtle top-to-bottom gradient (from slightly lighter to background)
- Cards: `border-border/50` — softer borders in dark mode
- Active elements: glow effect using `shadow-primary/20` instead of flat color
- Charts: brighter, more saturated chart colors in dark mode

---

## 7. Shared Component Updates (`@virtuestack/ui`)

The shared package needs these additions:

1. **Skeleton component** — shimmer loading placeholder
2. **AnimatedCard** — Card with motion hover/enter effects
3. **Badge variants** — add `success`, `warning`, `info` variants alongside existing ones
4. **Tooltip** — ensure all icon-only buttons have tooltips

No existing components are removed or have breaking API changes.

---

## 8. Files Changed Summary

### New files (both UIs):
- `lib/animations.ts` — shared animation variants and transitions
- `components/page-transition.tsx` — page enter animation wrapper
- `components/animated-card.tsx` — card with motion effects
- `components/skeleton.tsx` — shimmer skeleton loader
- `components/bottom-nav.tsx` — mobile bottom tab bar
- `components/breadcrumbs.tsx` — breadcrumb navigation

### Modified files (both UIs):
- `app/globals.css` — color palette, transitions, new tokens
- `app/layout.tsx` — Geist font loading
- `app/providers.tsx` — remove `disableTransitionOnChange`
- `tailwind.config.ts` — new keyframes, animations, color tokens
- `components/sidebar.tsx` — nav groups, active state, tooltips
- `components/mobile-nav.tsx` — updates to work alongside bottom nav
- `components/theme-toggle.tsx` — animated icon cycle
- `app/login/page.tsx` — split layout, brand panel, transitions
- `app/dashboard/page.tsx` (admin) — animated stats, better activity feed
- `app/vms/page.tsx` (customer) — card hover effects, mobile cards
- All list page components — table hover effects, empty states, skeletons
- `package.json` — add `motion` dependency

### New files (shared package):
- `webui/packages/ui/src/skeleton.tsx`

### Modified files (shared package):
- `webui/packages/ui/src/index.ts` — export new components
- `webui/packages/ui/package.json` — add `motion` dependency

---

## 9. Performance Considerations

- **Motion tree-shaking:** Only import what's used. `motion/react` is fully tree-shakable.
- **CSS transitions over JS where possible:** Simple hover/focus effects use Tailwind CSS transitions, not Motion.
- **Skeleton over spinner:** Skeleton layouts prevent layout shift and feel faster perceptually.
- **No layout thrashing:** All motion animations use `transform` and `opacity` (GPU-accelerated properties only).
- **Font optimization:** `next/font` handles font loading with `display: swap` and preloading.
- **Bundle impact:** Motion adds ~40KB gzipped. Geist font adds ~20KB. Total: ~60KB — acceptable for both admin and customer portals.

---

## 10. What This Does NOT Change

- No backend changes
- No API changes
- No routing changes
- No authentication flow changes
- No data model changes
- No build tooling changes (stays on npm, Tailwind 3, Next.js 16)
- No shadcn/ui version upgrade (stays compatible with current v3-era Tailwind config)
- Existing component APIs remain the same — this is additive

---

## 11. Decomposition Into Phases

This is a large visual overhaul. Implementation should be phased:

**Phase 1 — Foundation** (design system, no page changes):
- Install motion in all packages
- Update globals.css with new color palette and transitions
- Update tailwind.config.ts with new animations
- Add Geist font to layouts
- Remove `disableTransitionOnChange`
- Create shared animation utilities
- Create PageTransition, AnimatedCard, Skeleton components

**Phase 2 — Shared Layout** (sidebar, header, mobile nav):
- Redesign sidebar with nav groups and new active states
- Add breadcrumbs component
- Animate theme toggle
- Create bottom navigation for mobile
- Update header with notification bell placement

**Phase 3 — Login & Dashboard**:
- Redesign login pages with split layout
- Animate dashboard stat cards with count-up
- Improve activity feed with timeline layout
- Add skeleton loading states for dashboard

**Phase 4 — List Pages & Tables**:
- Add staggered row animations to tables
- Responsive table-to-card on mobile
- Empty state improvements
- Filter bar animations
- Card hover effects on all list pages

**Phase 5 — Detail Pages & Polish**:
- Tab animation improvements
- Status badge pulse effects
- Dialog/sheet animation refinements
- Toast notification improvements
- Final cross-browser testing and polish
