import { NextResponse, type NextRequest } from "next/server"
import { authHref, safeNext } from "@/lib/auth/paths"
import { GUEST_COOKIE } from "@/lib/auth/guest"

const SESSION_COOKIES = ["__Host-mbb_at", "mbb_at", "__Host-mbb_rt", "mbb_rt"]
const GUEST_ONLY = ["/login", "/signup"]
const OPEN = process.env.NEXT_PUBLIC_MBB_SNAPSHOT === "1"

export function proxy(request: NextRequest) {
  const { pathname, search, searchParams } = request.nextUrl
  const signedIn = SESSION_COOKIES.some((name) => request.cookies.has(name))
  if (GUEST_ONLY.includes(pathname)) {
    return signedIn ? NextResponse.redirect(new URL(safeNext(searchParams.get("next")), request.url)) : NextResponse.next()
  }
  if (signedIn || OPEN || request.cookies.get(GUEST_COOKIE)?.value === "1") return NextResponse.next()
  return NextResponse.redirect(new URL(authHref("/login", { next: pathname + search }), request.url))
}

export const config = {
  matcher: [
    "/login",
    "/signup",
    "/sports/:path*",
    "/leagues/:path*",
    "/favourites",
    "/events/:path*",
    "/players/:path*",
    "/signals/:path*",
    "/sources",
    "/baysed",
  ],
}
