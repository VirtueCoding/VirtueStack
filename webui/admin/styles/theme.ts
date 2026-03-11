/**
 * Theme Configuration for Admin WebUI
 * 
 * This file centralizes all theme-related configuration including:
 * - CSS variable definitions for light/dark themes
 * - Color palette (zinc-based for professional admin interface)
 * - Theme utilities and types
 * 
 * @module styles/theme
 */

// ============================================================================
// Theme Types
// ============================================================================

export type ThemeMode = 'light' | 'dark' | 'system';

export interface ThemeColors {
  background: string;
  foreground: string;
  card: string;
  cardForeground: string;
  popover: string;
  popoverForeground: string;
  primary: string;
  primaryForeground: string;
  secondary: string;
  secondaryForeground: string;
  muted: string;
  mutedForeground: string;
  accent: string;
  accentForeground: string;
  destructive: string;
  destructiveForeground: string;
  border: string;
  input: string;
  ring: string;
}

export interface ThemeConfig {
  name: string;
  baseColor: 'zinc' | 'blue' | 'slate' | 'gray';
  radius: number;
  colors: ThemeColors;
}

// ============================================================================
// Color Palette (Zinc - Professional Admin Theme)
// ============================================================================

export const colors = {
  // Base colors
  zinc: {
    50: '240 20% 98%',
    100: '240 10% 96%',
    200: '240 6% 90%',
    300: '240 5% 84%',
    400: '240 5% 65%',
    500: '240 4% 46%',
    600: '240 5% 34%',
    700: '240 6% 25%',
    800: '240 6% 10%',
    900: '240 10% 4%',
    950: '240 10% 4%',
  },
  // Semantic colors
  light: {
    background: '0 0% 100%',
    foreground: '240 10% 3.9%',
    card: '0 0% 100%',
    cardForeground: '240 10% 3.9%',
    popover: '0 0% 100%',
    popoverForeground: '240 10% 3.9%',
    primary: '240 5.9% 10%',
    primaryForeground: '0 0% 98%',
    secondary: '240 4.8% 95.9%',
    secondaryForeground: '240 5.9% 10%',
    muted: '240 4.8% 95.9%',
    mutedForeground: '240 3.8% 46.1%',
    accent: '240 4.8% 95.9%',
    accentForeground: '240 5.9% 10%',
    destructive: '0 84.2% 60.2%',
    destructiveForeground: '0 0% 98%',
    border: '240 5.9% 90%',
    input: '240 5.9% 90%',
    ring: '240 5.9% 10%',
  } as ThemeColors,
  dark: {
    background: '240 10% 3.9%',
    foreground: '0 0% 98%',
    card: '240 10% 3.9%',
    cardForeground: '0 0% 98%',
    popover: '240 10% 3.9%',
    popoverForeground: '0 0% 98%',
    primary: '0 0% 98%',
    primaryForeground: '240 5.9% 10%',
    secondary: '240 3.7% 15.9%',
    secondaryForeground: '0 0% 98%',
    muted: '240 3.7% 15.9%',
    mutedForeground: '240 5% 64.9%',
    accent: '240 3.7% 15.9%',
    accentForeground: '0 0% 98%',
    destructive: '0 62.8% 30.6%',
    destructiveForeground: '0 0% 98%',
    border: '240 3.7% 15.9%',
    input: '240 3.7% 15.9%',
    ring: '240 4.9% 83.9%',
  } as ThemeColors,
};

// ============================================================================
// Theme Configuration
// ============================================================================

export const themeConfig: ThemeConfig = {
  name: 'admin',
  baseColor: 'zinc',
  radius: 0.5,
  colors: colors.light,
};

// ============================================================================
// CSS Variable Generator
// ============================================================================

/**
 * Generates CSS variable definitions for the theme
 * Use this to create CSS custom properties
 */
export function generateCSSVariables(colors: ThemeColors): string {
  return `
    --background: ${colors.background};
    --foreground: ${colors.foreground};
    --card: ${colors.card};
    --card-foreground: ${colors.cardForeground};
    --popover: ${colors.popover};
    --popover-foreground: ${colors.popoverForeground};
    --primary: ${colors.primary};
    --primary-foreground: ${colors.primaryForeground};
    --secondary: ${colors.secondary};
    --secondary-foreground: ${colors.secondaryForeground};
    --muted: ${colors.muted};
    --muted-foreground: ${colors.mutedForeground};
    --accent: ${colors.accent};
    --accent-foreground: ${colors.accentForeground};
    --destructive: ${colors.destructive};
    --destructive-foreground: ${colors.destructiveForeground};
    --border: ${colors.border};
    --input: ${colors.input};
    --ring: ${colors.ring};
    --radius: ${themeConfig.radius}rem;
  `.trim();
}

// ============================================================================
// Theme Utilities
// ============================================================================

/**
 * Converts HSL string to CSS hsl() function
 */
export function hsl(hslString: string): string {
  return `hsl(${hslString})`;
}

/**
 * Converts HSL string to CSS hsla() function with alpha
 */
export function hsla(hslString: string, alpha: number): string {
  return `hsl(${hslString} / ${alpha})`;
}

/**
 * Get theme colors for a specific mode
 */
export function getThemeColors(mode: 'light' | 'dark'): ThemeColors {
  return mode === 'dark' ? colors.dark : colors.light;
}

/**
 * Check if user prefers dark mode (for system preference detection)
 */
export function prefersDarkMode(): boolean {
  if (typeof window === 'undefined') return false;
  return window.matchMedia('(prefers-color-scheme: dark)').matches;
}

// ============================================================================
// shadcn/ui Theme Configuration
// ============================================================================

/**
 * Configuration object for shadcn/ui components
 * Matches the components.json configuration
 */
export const shadcnConfig = {
  $schema: 'https://ui.shadcn.com/schema.json',
  style: 'default',
  rsc: true,
  tsx: true,
  tailwind: {
    config: 'tailwind.config.ts',
    css: 'app/globals.css',
    baseColor: 'zinc',
    cssVariables: true,
  },
  aliases: {
    components: '@/components',
    utils: '@/lib/utils',
  },
};

// ============================================================================
// Default Export
// ============================================================================

export default themeConfig;
