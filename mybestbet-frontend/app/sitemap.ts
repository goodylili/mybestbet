import type { MetadataRoute } from "next"
import { SIGNALS, SPORTS } from "@/lib/odds/markets"

const BASE_URL = "https://mybestbet.app"

export default function sitemap(): MetadataRoute.Sitemap {
  const lastModified = new Date()

  return [
    {
      url: `${BASE_URL}/`,
      lastModified,
      changeFrequency: "weekly",
      priority: 1,
    },
    ...SPORTS.map((sport) => ({
      url: `${BASE_URL}/sports/${sport.id}`,
      lastModified,
      changeFrequency: "hourly" as const,
      priority: 0.9,
    })),
    ...Object.values(SIGNALS).map((signal) => ({
      url: `${BASE_URL}/signals/${signal.slug}`,
      lastModified,
      changeFrequency: "hourly" as const,
      priority: 0.8,
    })),
    {
      url: `${BASE_URL}/sources`,
      lastModified,
      changeFrequency: "daily",
      priority: 0.5,
    },
  ]
}
