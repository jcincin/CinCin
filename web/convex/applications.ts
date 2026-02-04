import { v } from "convex/values";
import { mutation, query } from "./_generated/server";

export const createApplication = mutation({
  args: {
    email: v.string(),
    firstName: v.string(),
    lastName: v.string(),
    referredBy: v.optional(v.string()),
    message: v.optional(v.string()),
  },
  handler: async (ctx, args) => {
    // Check if application already exists for this email
    const existing = await ctx.db
      .query("applications")
      .withIndex("by_email", (q) => q.eq("email", args.email))
      .first();

    if (existing) {
      // Update existing application
      await ctx.db.patch(existing._id, {
        firstName: args.firstName,
        lastName: args.lastName,
        referredBy: args.referredBy,
        message: args.message,
        status: "pending",
        createdAt: Date.now(),
      });
      return existing._id;
    }

    const applicationId = await ctx.db.insert("applications", {
      email: args.email,
      firstName: args.firstName,
      lastName: args.lastName,
      referredBy: args.referredBy,
      message: args.message,
      status: "pending",
      createdAt: Date.now(),
    });

    return applicationId;
  },
});

export const getApplicationByEmail = query({
  args: { email: v.string() },
  handler: async (ctx, args) => {
    return await ctx.db
      .query("applications")
      .withIndex("by_email", (q) => q.eq("email", args.email))
      .first();
  },
});

export const getPendingApplications = query({
  args: {},
  handler: async (ctx) => {
    return await ctx.db
      .query("applications")
      .withIndex("by_status", (q) => q.eq("status", "pending"))
      .collect();
  },
});

export const approveApplication = mutation({
  args: { email: v.string() },
  handler: async (ctx, args) => {
    const application = await ctx.db
      .query("applications")
      .withIndex("by_email", (q) => q.eq("email", args.email))
      .first();

    if (!application) {
      throw new Error("Application not found");
    }

    await ctx.db.patch(application._id, {
      status: "approved",
      approvedAt: Date.now(),
    });

    return application._id;
  },
});

export const rejectApplication = mutation({
  args: { email: v.string() },
  handler: async (ctx, args) => {
    const application = await ctx.db
      .query("applications")
      .withIndex("by_email", (q) => q.eq("email", args.email))
      .first();

    if (!application) {
      throw new Error("Application not found");
    }

    await ctx.db.patch(application._id, {
      status: "rejected",
    });

    return application._id;
  },
});
