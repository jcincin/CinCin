import { NextRequest, NextResponse } from "next/server";
import { ConvexHttpClient } from "convex/browser";
import { api } from "@/convex/_generated/api";

let _convex: ConvexHttpClient;
function getConvex() {
  if (!_convex) _convex = new ConvexHttpClient(process.env.NEXT_PUBLIC_CONVEX_URL!);
  return _convex;
}

type UsageType = "immediate" | "concierge";

interface UsageRequest {
  clerkUserId: string;
  type: UsageType;
}

export async function POST(request: NextRequest) {
  const internalToken = process.env.INTERNAL_API_TOKEN;

  if (!internalToken) {
    return NextResponse.json(
      { error: "Server configuration error" },
      { status: 500 }
    );
  }

  const token = request.headers.get("x-internal-token");
  if (token !== internalToken) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  let body: UsageRequest;
  try {
    body = await request.json();
  } catch {
    return NextResponse.json({ error: "Invalid request format" }, { status: 400 });
  }

  if (!body?.clerkUserId || !body?.type) {
    return NextResponse.json({ error: "Missing required fields" }, { status: 400 });
  }

  if (body.type !== "immediate" && body.type !== "concierge") {
    return NextResponse.json({ error: "Invalid usage type" }, { status: 400 });
  }

  try {
    const user = await getConvex().query(api.users.getUserByClerkId, {
      clerkId: body.clerkUserId,
    });

    if (!user) {
      return NextResponse.json({ error: "User not found" }, { status: 404 });
    }

    await getConvex().mutation(api.usage.incrementUsage, {
      userId: user._id,
      type: body.type,
    });

    return NextResponse.json({ success: true });
  } catch (error) {
    console.error("Internal usage increment error:", error);
    return NextResponse.json(
      { error: "Failed to increment usage" },
      { status: 500 }
    );
  }
}
