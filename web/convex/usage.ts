import { v } from "convex/values";
import { mutation, query } from "./_generated/server";

// Tier limits
const TIER_LIMITS = {
  basic: {
    reservations: 5,
    conciergeBookings: 1,
  },
  premium: {
    reservations: 15,
    conciergeBookings: 5,
  },
};

function getCurrentMonth(): string {
  const now = new Date();
  return `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, "0")}`;
}

export const getUsage = query({
  args: { userId: v.id("users") },
  handler: async (ctx, args) => {
    const month = getCurrentMonth();
    const usage = await ctx.db
      .query("monthlyUsage")
      .withIndex("by_userId_month", (q) =>
        q.eq("userId", args.userId).eq("month", month)
      )
      .first();

    return (
      usage || {
        userId: args.userId,
        month,
        reservations: 0,
        conciergeBookings: 0,
      }
    );
  },
});

export const getUsageWithLimits = query({
  args: { userId: v.id("users") },
  handler: async (ctx, args) => {
    const month = getCurrentMonth();
    const usage = await ctx.db
      .query("monthlyUsage")
      .withIndex("by_userId_month", (q) =>
        q.eq("userId", args.userId).eq("month", month)
      )
      .first();

    // Get user's subscription to determine tier
    const subscription = await ctx.db
      .query("subscriptions")
      .withIndex("by_userId", (q) => q.eq("userId", args.userId))
      .first();

    const tier = subscription?.tier || "basic";
    const limits = TIER_LIMITS[tier];

    const currentUsage = usage || {
      reservations: 0,
      conciergeBookings: 0,
    };

    return {
      month,
      tier,
      reservations: {
        used: currentUsage.reservations,
        limit: limits.reservations,
        remaining: Math.max(0, limits.reservations - currentUsage.reservations),
      },
      conciergeBookings: {
        used: currentUsage.conciergeBookings,
        limit: limits.conciergeBookings,
        remaining: Math.max(
          0,
          limits.conciergeBookings - currentUsage.conciergeBookings
        ),
      },
    };
  },
});

export const checkCanBook = query({
  args: {
    userId: v.id("users"),
    type: v.union(v.literal("immediate"), v.literal("concierge")),
  },
  handler: async (ctx, args) => {
    const month = getCurrentMonth();
    const usage = await ctx.db
      .query("monthlyUsage")
      .withIndex("by_userId_month", (q) =>
        q.eq("userId", args.userId).eq("month", month)
      )
      .first();

    const subscription = await ctx.db
      .query("subscriptions")
      .withIndex("by_userId", (q) => q.eq("userId", args.userId))
      .first();

    // Must have active subscription
    if (!subscription || subscription.status !== "active") {
      return {
        allowed: false,
        reason: "No active subscription",
      };
    }

    const tier = subscription.tier;
    const limits = TIER_LIMITS[tier];
    const currentUsage = usage || { reservations: 0, conciergeBookings: 0 };

    if (args.type === "concierge") {
      if (currentUsage.conciergeBookings >= limits.conciergeBookings) {
        return {
          allowed: false,
          reason: `Concierge booking limit reached (${limits.conciergeBookings}/month for ${tier} tier)`,
        };
      }
    }

    // Both types count against reservation limit
    if (currentUsage.reservations >= limits.reservations) {
      return {
        allowed: false,
        reason: `Reservation limit reached (${limits.reservations}/month for ${tier} tier)`,
      };
    }

    return { allowed: true };
  },
});

export const incrementUsage = mutation({
  args: {
    userId: v.id("users"),
    type: v.union(v.literal("immediate"), v.literal("concierge")),
  },
  handler: async (ctx, args) => {
    const month = getCurrentMonth();
    const usage = await ctx.db
      .query("monthlyUsage")
      .withIndex("by_userId_month", (q) =>
        q.eq("userId", args.userId).eq("month", month)
      )
      .first();

    if (usage) {
      const updates: { reservations?: number; conciergeBookings?: number } = {
        reservations: usage.reservations + 1,
      };
      if (args.type === "concierge") {
        updates.conciergeBookings = usage.conciergeBookings + 1;
      }
      await ctx.db.patch(usage._id, updates);
      return usage._id;
    }

    // Create new usage record for this month
    const usageId = await ctx.db.insert("monthlyUsage", {
      userId: args.userId,
      month,
      reservations: 1,
      conciergeBookings: args.type === "concierge" ? 1 : 0,
    });

    return usageId;
  },
});

export const resetUsageForMonth = mutation({
  args: {
    userId: v.id("users"),
    month: v.string(),
  },
  handler: async (ctx, args) => {
    const usage = await ctx.db
      .query("monthlyUsage")
      .withIndex("by_userId_month", (q) =>
        q.eq("userId", args.userId).eq("month", args.month)
      )
      .first();

    if (usage) {
      await ctx.db.patch(usage._id, {
        reservations: 0,
        conciergeBookings: 0,
      });
      return usage._id;
    }

    return null;
  },
});
