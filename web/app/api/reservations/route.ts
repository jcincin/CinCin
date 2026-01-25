import { NextRequest, NextResponse } from "next/server";

export async function GET(request: NextRequest) {
  const apiUrl = process.env.API_URL || "http://localhost:8090";

  try {
    const cookieHeader = request.headers.get("cookie");

    const response = await fetch(`${apiUrl}/api/reservations`, {
      method: "GET",
      headers: {
        "Content-Type": "application/json",
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
