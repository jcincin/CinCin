import { NextResponse } from "next/server";
import { auth } from "@clerk/nextjs/server";

export async function GET() {
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
    const response = await fetch(`${apiUrl}/api/resy/status`, {
      method: "GET",
      headers: {
        "X-Clerk-User-Id": userId,
        "X-Internal-Token": internalToken,
      },
    });

    const data = await response.json();
    return NextResponse.json(data, { status: response.status });
  } catch (error) {
    console.error("Resy status proxy error:", error);
    return NextResponse.json(
      { error: "Failed to connect to server" },
      { status: 500 }
    );
  }
}
