export const ADMIN_BASE_PATH = "/admin";

export function stripAdminBasePath(pathname: string | null | undefined): string {
  if (!pathname) {
    return "/";
  }

  if (pathname === ADMIN_BASE_PATH) {
    return "/";
  }

  if (pathname.startsWith(`${ADMIN_BASE_PATH}/`)) {
    return pathname.slice(ADMIN_BASE_PATH.length);
  }

  return pathname;
}

export function isAdminLoginPath(pathname: string | null | undefined): boolean {
  return stripAdminBasePath(pathname) === "/login";
}

export function isAdminNavItemActive(
  pathname: string | null | undefined,
  href: string,
): boolean {
  return stripAdminBasePath(pathname) === href;
}
