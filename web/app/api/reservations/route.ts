import { NextRequest, NextResponse } from "next/server";
import { auth } from "@clerk/nextjs/server";

export async function GET(request: NextRequest) {
  const { userId } = await auth();

  if (!userId) {
    return NextResponse.json({ error: "Unauthorized", reservations: [] }, { status: 401 });
  }

  const apiUrl = process.env.API_URL || "http://localhost:8090";
  const internalToken = process.env.INTERNAL_API_TOKEN;

  if (!internalToken) {
    return NextResponse.json(
      { error: "Server configuration error", reservations: [] },
      { status: 500 }
    );
  }

  try {
    const cookieHeader = request.headers.get("cookie");

    const response = await fetch(`${apiUrl}/api/reservations`, {
      method: "GET",
      headers: {
        "Content-Type": "application/json",
        "X-Clerk-User-Id": userId,
        "X-Internal-Token": internalToken,
        ...(cookieHeader && { Cookie: cookieHeader }),
      },
    });

    const data = await response.json();

    const nextResponse = NextResponse.json(data, { status: response.status });

    const setCookie = response.headers.get("set-cookie");
    if (setCookie) {
      nextResponse.headers.set("set-cookie", setCookie);
    }

    return nextResponse;
  } catch (error) {
    console.error("Reservations list proxy error:", error);
    return NextResponse.json(
      { error: "Failed to connect to server", reservations: [] },
      { status: 500 }
    );
  }
}
