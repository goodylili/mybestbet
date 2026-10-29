const ZONES: Record<string, string> = {
  "Africa/Lagos": "NG", "Africa/Accra": "GH", "Africa/Nairobi": "KE", "Africa/Kampala": "UG", "Africa/Dar_es_Salaam": "TZ",
  "Africa/Lusaka": "ZM", "Africa/Johannesburg": "ZA", "Africa/Kigali": "RW", "Africa/Blantyre": "MW", "Africa/Maputo": "MZ",
  "Africa/Douala": "CM", "Africa/Kinshasa": "CD", "Africa/Lubumbashi": "CD", "Africa/Addis_Ababa": "ET", "Africa/Cairo": "EG",
  "Africa/Casablanca": "MA", "Africa/Abidjan": "CI", "Africa/Dakar": "SN", "Africa/Freetown": "SL", "Africa/Monrovia": "LR",
  "Africa/Porto-Novo": "BJ", "Africa/Gaborone": "BW", "Africa/Harare": "ZW", "Africa/Windhoek": "NA", "Africa/Maseru": "LS",
  "Africa/Juba": "SS", "Africa/Brazzaville": "CG", "Africa/Bamako": "ML",
  "Europe/London": "GB", "Europe/Dublin": "IE", "Europe/Paris": "FR", "Europe/Berlin": "DE", "Europe/Madrid": "ES",
  "Europe/Rome": "IT", "Europe/Lisbon": "PT", "Europe/Amsterdam": "NL", "Europe/Brussels": "BE", "Europe/Athens": "GR",
  "Europe/Bucharest": "RO", "Europe/Sofia": "BG", "Europe/Prague": "CZ", "Europe/Warsaw": "PL", "Europe/Copenhagen": "DK",
  "Europe/Stockholm": "SE", "Europe/Oslo": "NO", "Europe/Helsinki": "FI", "Europe/Vienna": "AT", "Europe/Zurich": "CH",
  "Europe/Kyiv": "UA", "Europe/Kiev": "UA", "Europe/Istanbul": "TR", "Europe/Moscow": "RU", "Europe/Budapest": "HU",
  "Asia/Nicosia": "CY", "Europe/Malta": "MT",
  "America/New_York": "US", "America/Chicago": "US", "America/Denver": "US", "America/Phoenix": "US", "America/Los_Angeles": "US",
  "America/Anchorage": "US", "Pacific/Honolulu": "US", "America/Detroit": "US", "America/Toronto": "CA", "America/Vancouver": "CA",
  "America/Edmonton": "CA", "America/Winnipeg": "CA", "America/Halifax": "CA", "America/Mexico_City": "MX", "America/Sao_Paulo": "BR",
  "America/Argentina/Buenos_Aires": "AR", "America/Buenos_Aires": "AR", "America/Bogota": "CO", "America/Lima": "PE",
  "America/Santiago": "CL", "America/Guayaquil": "EC", "America/Caracas": "VE",
  "Asia/Jakarta": "ID", "Asia/Makassar": "ID", "Asia/Jayapura": "ID", "Asia/Singapore": "SG", "Asia/Kuala_Lumpur": "MY",
  "Asia/Bangkok": "TH", "Asia/Manila": "PH", "Asia/Ho_Chi_Minh": "VN", "Asia/Saigon": "VN", "Asia/Kolkata": "IN", "Asia/Calcutta": "IN",
  "Asia/Karachi": "PK", "Asia/Dhaka": "BD", "Asia/Tokyo": "JP", "Asia/Seoul": "KR", "Asia/Shanghai": "CN", "Asia/Hong_Kong": "HK",
  "Asia/Taipei": "TW", "Asia/Dubai": "AE", "Asia/Riyadh": "SA", "Asia/Qatar": "QA", "Asia/Jerusalem": "IL", "Asia/Tel_Aviv": "IL",
  "Australia/Sydney": "AU", "Australia/Melbourne": "AU", "Australia/Brisbane": "AU", "Australia/Perth": "AU", "Australia/Adelaide": "AU",
  "Pacific/Auckland": "NZ",
}

const NOT_COUNTRIES = new Set(["EU", "EZ", "UN", "QO", "ZZ", "XA", "XB", "AC", "CP", "DG", "EA", "IC", "TA"])

export const ALPHA2 = /^[A-Z]{2}$/

/** The location value that turns location filtering off, so every book is available. */
export const ALL_LOCATIONS = "ALL"

function languageCountry() {
  for (const tag of navigator.languages || [navigator.language]) {
    try {
      const region = new Intl.Locale(tag).maximize().region
      if (region && ALPHA2.test(region) && !NOT_COUNTRIES.has(region)) return region
    } catch {}
  }
  return ""
}

/** Best guess at the visitor's country from the browser: the time zone first, then the language region. */
export function zoneCountry() {
  try {
    return ZONES[Intl.DateTimeFormat().resolvedOptions().timeZone] || languageCountry()
  } catch {
    return ""
  }
}

let names: Intl.DisplayNames | null = null

export function countryName(code: string) {
  if (code === ALL_LOCATIONS) return "all locations"
  try {
    names ??= new Intl.DisplayNames(["en"], { type: "region", fallback: "none" })
    return names.of(code) || code
  } catch {
    return code
  }
}

const isCanonical = (code: string) => {
  try {
    return Intl.getCanonicalLocales("und-" + code)[0] === "und-" + code
  } catch {
    return false
  }
}

let list: { code: string; name: string }[] | null = null

export function countryList() {
  if (list) return list
  const all: { code: string; name: string }[] = []
  for (let a = 65; a <= 90; a++) {
    for (let b = 65; b <= 90; b++) {
      const code = String.fromCharCode(a, b)
      const name = NOT_COUNTRIES.has(code) || !isCanonical(code) ? "" : countryName(code)
      if (name && name !== code) all.push({ code, name })
    }
  }
  list = all.sort((x, y) => x.name.localeCompare(y.name))
  return list
}
