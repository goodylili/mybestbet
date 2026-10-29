export const GUEST_COOKIE = "mbb_guest"

const GUEST_DAYS = 30

export const isGuest = () => typeof document !== "undefined" && document.cookie.split("; ").some((c) => c === GUEST_COOKIE + "=1")

export function startGuest() {
  document.cookie = `${GUEST_COOKIE}=1; Path=/; Max-Age=${GUEST_DAYS * 86400}; SameSite=Lax` + (location.protocol === "https:" ? "; Secure" : "")
}
