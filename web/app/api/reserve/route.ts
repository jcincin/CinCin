import { NextRequest, NextResponse } from "next/server";
import { auth } from "@clerk/nextjs/server";
import { ConvexHttpClient } from "convex/browser";
import { api } from "@/convex/_generated/api";

const convex = new ConvexHttpClient(process.env.NEXT_PUBLIC_CONVEX_URL!);

export async function POST(request: NextRequest) {
  const { userId } = await auth();

  if (!userId) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const apiUrl = process.env.API_URL || "http://localhost:8090";
  const internalToken = process.env.INTERNAL_API_TOKEN;

  if (!internalToken) {
    return NextResponse.json(
      { error: "Server configuration error" },
      { status: 500 }
    );
  }

  try {
    const body = await request.json();

    const convexUser = await convex.query(api.users.getUserByClerkId, {
      clerkId: userId,
    });

    if (!convexUser) {
      return NextResponse.json({ error: "User not found" }, { status: 404 });
    }

    const usageType = body?.auto_schedule ? "concierge" : "immediate";
    const usageCheck = await convex.query(api.usage.checkCanBook, {
      userId: convexUser._id,
      type: usageType,
    });

    if (!usageCheck.allowed) {
      return NextResponse.json(
        { error: usageCheck.reason || "Usage limit reached" },
        { status: 403 }
      );
    }

    // Forward cookies from the client request to the backend
    const cookieHeader = request.headers.get("cookie");

    const response = await fetch(`${apiUrl}/api/reserve`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "X-Clerk-User-Id": userId,
        "X-Internal-Token": internalToken,
        ...(cookieHeader && { Cookie: cookieHeader }),
      },
      body: JSON.stringify(body),
    });

    const data = await response.json();

    // Create response and forward any cookies from the backend
    const nextResponse = NextResponse.json(data, { status: response.status });

    // Forward Set-Cookie headers from backend
    const setCookie = response.headers.get("set-cookie");
    if (setCookie) {
      nextResponse.headers.set("set-cookie", setCookie);
    }

    if (response.ok && data?.reservation_time) {
      try {
        await convex.mutation(api.usage.incrementUsage, {
          userId: convexUser._id,
          type: usageType,
        });
      } catch (usageError) {
        console.error("Usage increment error:", usageError);
      }
    }

    return nextResponse;
  } catch (error) {
    console.error("Reserve proxy error:", error);
    return NextResponse.json(
      { error: "Failed to connect to server" },
      { status: 500 }
    );
  }
}

