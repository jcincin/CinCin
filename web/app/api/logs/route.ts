import { NextResponse } from "next/server";

export async function GET() {
  const apiUrl = process.env.API_URL || "http://localhost:8090";
  const internalToken = process.env.INTERNAL_API_TOKEN;

  if (!internalToken) {
    return NextResponse.json(
      { error: "Server configuration error" },
      { status: 500 }
    );
  }
  
  try {
    const response = await fetch(`${apiUrl}/api/logs`, {
      method: "GET",
      headers: {
        "Content-Type": "application/json",
        "X-Internal-Token": internalToken,
      },
    });

    const data = await response.json();
    return NextResponse.json(data, { status: response.status });
  } catch (error) {
    console.error("Logs proxy error:", error);
    return NextResponse.json([], { status: 500 });
  }
}

