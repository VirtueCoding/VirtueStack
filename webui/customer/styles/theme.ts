/**
 * Theme Configuration for Customer WebUI
 * 
 * This file centralizes all theme-related configuration including:
 * - CSS variable definitions for light/dark themes
 * - Color palette (blue-based for customer-facing interface)
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
// Color Palette (Blue - Customer-Facing Theme)
// ============================================================================

export const colors = {
  // Base colors
  blue: {
    50: '213 100% 96%',
    100: '214 94% 93%',
    200: '214 94% 87%',
    300: '213 97% 79%',
    400: '214 95% 67%',
    500: '217 91% 60%',
    600: '221 83% 53%',
    700: '224 76% 48%',
    800: '226 71% 40%',
    900: '224 64% 33%',
    950: '226 57% 21%',
  },
  // Semantic colors
  light: {
    background: '0 0% 100%',
    foreground: '222.2 84% 4.9%',
    card: '0 0% 100%',
    cardForeground: '222.2 84% 4.9%',
    popover: '0 0% 100%',
    popoverForeground: '222.2 84% 4.9%',
    primary: '221.2 83.2% 53.3%',
    primaryForeground: '210 40% 98%',
    secondary: '210 40% 96.1%',
    secondaryForeground: '222.2 47.4% 11.2%',
    muted: '210 40% 96.1%',
    mutedForeground: '215.4 16.3% 46.9%',
    accent: '210 40% 96.1%',
    accentForeground: '222.2 47.4% 11.2%',
    destructive: '0 84.2% 60.2%',
    destructiveForeground: '210 40% 98%',
    border: '214.3 31.8% 91.4%',
    input: '214.3 31.8% 91.4%',
    ring: '221.2 83.2% 53.3%',
  } as ThemeColors,
  dark: {
    background: '222.2 84% 4.9%',
    foreground: '210 40% 98%',
    card: '222.2 84% 4.9%',
    cardForeground: '210 40% 98%',
    popover: '222.2 84% 4.9%',
    popoverForeground: '210 40% 98%',
    primary: '217.2 91.2% 59.8%',
    primaryForeground: '222.2 47.4% 11.2%',
    secondary: '217.2 32.6% 17.5%',
    secondaryForeground: '210 40% 98%',
    muted: '217.2 32.6% 17.5%',
    mutedForeground: '215 20.2% 65.1%',
    accent: '217.2 32.6% 17.5%',
    accentForeground: '210 40% 98%',
    destructive: '0 62.8% 30.6%',
    destructiveForeground: '210 40% 98%',
    border: '217.2 32.6% 17.5%',
    input: '217.2 32.6% 17.5%',
    ring: '224.3 76.3% 48%',
  } as ThemeColors,
};

// ============================================================================
// Theme Configuration
// ============================================================================

export const themeConfig: ThemeConfig = {
  name: 'customer',
  baseColor: 'blue',
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
    baseColor: 'blue',
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
