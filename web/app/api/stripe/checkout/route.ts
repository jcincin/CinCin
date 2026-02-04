import { NextRequest, NextResponse } from "next/server";
import { auth, currentUser } from "@clerk/nextjs/server";
import Stripe from "stripe";

let _stripe: Stripe;
function getStripe() {
  if (!_stripe) _stripe = new Stripe(process.env.STRIPE_SECRET_KEY!, { apiVersion: "2025-12-15.clover" });
  return _stripe;
}

interface CheckoutRequest {
  tier: "basic" | "premium";
}

export async function POST(request: NextRequest) {
  try {
    const { userId } = await auth();
    const user = await currentUser();

    if (!userId || !user) {
      return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
    }

    const body: CheckoutRequest = await request.json();

    if (!body.tier || !["basic", "premium"].includes(body.tier)) {
      return NextResponse.json({ error: "Invalid tier" }, { status: 400 });
    }

    const priceId =
      body.tier === "basic"
        ? process.env.STRIPE_BASIC_PRICE_ID
        : process.env.STRIPE_PREMIUM_PRICE_ID;

    if (!priceId) {
      console.error(`Missing price ID for tier: ${body.tier}`);
      return NextResponse.json(
        { error: "Server configuration error" },
        { status: 500 }
      );
    }

    const appUrl = process.env.NEXT_PUBLIC_APP_URL || "http://localhost:3000";

    // Create Stripe checkout session
    const session = await getStripe().checkout.sessions.create({
      mode: "subscription",
      payment_method_types: ["card"],
      line_items: [
        {
          price: priceId,
          quantity: 1,
        },
      ],
      customer_email: user.emailAddresses[0]?.emailAddress,
      metadata: {
        clerkUserId: userId,
        tier: body.tier,
        firstName: user.firstName || "",
        lastName: user.lastName || "",
      },
      success_url: `${appUrl}/home?checkout=success`,
      cancel_url: `${appUrl}/onboarding?checkout=canceled`,
    });

    return NextResponse.json({ url: session.url });
  } catch (error) {
    console.error("Stripe checkout error:", error);
    return NextResponse.json(
      { error: "Failed to create checkout session" },
      { status: 500 }
    );
  }
}
