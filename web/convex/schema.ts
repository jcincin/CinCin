import { defineSchema, defineTable } from "convex/server";
import { v } from "convex/values";

export default defineSchema({
  // Synced from Clerk
  users: defineTable({
    clerkId: v.string(),
    email: v.string(),
    firstName: v.string(),
    lastName: v.string(),
    phone: v.optional(v.string()),
    createdAt: v.number(),
  })
    .index("by_clerkId", ["clerkId"])
    .index("by_email", ["email"]),

  // Resy account linking
  userResyCredentials: defineTable({
    userId: v.id("users"),
    resyEmail: v.string(),
    resyAuthToken: v.string(), // encrypted
    resyPaymentMethodId: v.string(),
  }).index("by_userId", ["userId"]),

  // Stripe subscription
  subscriptions: defineTable({
    userId: v.id("users"),
    stripeCustomerId: v.string(),
    stripeSubscriptionId: v.string(),
    tier: v.union(v.literal("basic"), v.literal("premium")),
    status: v.union(
      v.literal("active"),
      v.literal("canceled"),
      v.literal("past_due")
    ),
    currentPeriodEnd: v.number(),
  })
    .index("by_userId", ["userId"])
    .index("by_stripeCustomerId", ["stripeCustomerId"])
    .index("by_stripeSubscriptionId", ["stripeSubscriptionId"]),

  // Usage tracking
  monthlyUsage: defineTable({
    userId: v.id("users"),
    month: v.string(), // "2026-01"
    reservations: v.number(),
    conciergeBookings: v.number(),
  }).index("by_userId_month", ["userId", "month"]),

  // Applications
  applications: defineTable({
    email: v.string(),
    firstName: v.string(),
    lastName: v.string(),
    referredBy: v.optional(v.string()),
    message: v.optional(v.string()),
    status: v.union(
      v.literal("pending"),
      v.literal("approved"),
      v.literal("rejected")
    ),
    createdAt: v.number(),
    approvedAt: v.optional(v.number()),
  })
    .index("by_email", ["email"])
    .index("by_status", ["status"]),

  // Reservation history
  reservationHistory: defineTable({
    userId: v.id("users"),
    venueId: v.number(),
    venueName: v.string(),
    reservationTime: v.number(),
    partySize: v.number(),
    type: v.union(v.literal("immediate"), v.literal("concierge")),
    status: v.union(
      v.literal("scheduled"),
      v.literal("success"),
      v.literal("failed"),
      v.literal("cancelled")
    ),
    createdAt: v.number(),
  })
    .index("by_userId", ["userId"])
    .index("by_userId_status", ["userId", "status"]),
});
