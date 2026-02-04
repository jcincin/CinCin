import { NextRequest, NextResponse } from "next/server";
import Stripe from "stripe";
import { ConvexHttpClient } from "convex/browser";
import { api } from "@/convex/_generated/api";

const stripe = new Stripe(process.env.STRIPE_SECRET_KEY!, {
  apiVersion: "2026-01-28.clover",
});

const convex = new ConvexHttpClient(process.env.NEXT_PUBLIC_CONVEX_URL!);

export async function POST(request: NextRequest) {
  const body = await request.text();
  const signature = request.headers.get("stripe-signature");

  if (!signature) {
    return NextResponse.json({ error: "No signature" }, { status: 400 });
  }

  let event: Stripe.Event;

  try {
    event = stripe.webhooks.constructEvent(
      body,
      signature,
      process.env.STRIPE_WEBHOOK_SECRET!
    );
  } catch (err) {
    console.error("Webhook signature verification failed:", err);
    return NextResponse.json({ error: "Invalid signature" }, { status: 400 });
  }

  try {
    switch (event.type) {
      case "checkout.session.completed": {
        const session = event.data.object as Stripe.Checkout.Session;
        await handleCheckoutCompleted(session);
        break;
      }

      case "invoice.paid": {
        const invoice = event.data.object as Stripe.Invoice;
        await handleInvoicePaid(invoice);
        break;
      }

      case "customer.subscription.updated": {
        const subscription = event.data.object as Stripe.Subscription;
        await handleSubscriptionUpdated(subscription);
        break;
      }

      case "customer.subscription.deleted": {
        const subscription = event.data.object as Stripe.Subscription;
        await handleSubscriptionDeleted(subscription);
        break;
      }

      default:
        console.log(`Unhandled event type: ${event.type}`);
    }

    return NextResponse.json({ received: true });
  } catch (error) {
    console.error("Webhook handler error:", error);
    return NextResponse.json(
      { error: "Webhook handler failed" },
      { status: 500 }
    );
  }
}

async function handleCheckoutCompleted(session: Stripe.Checkout.Session) {
  const { clerkUserId, tier, firstName, lastName } = session.metadata || {};

  if (!clerkUserId || !tier) {
    console.error("Missing metadata in checkout session");
    return;
  }

  const email = session.customer_email;
  if (!email) {
    console.error("Missing customer email in checkout session");
    return;
  }

  // Create or get user in Convex
  const userId = await convex.mutation(api.users.createUser, {
    clerkId: clerkUserId,
    email,
    firstName: firstName || "",
    lastName: lastName || "",
  });

  // Get subscription details from Stripe
  const subscriptionId = session.subscription as string;
  const subscription = await stripe.subscriptions.retrieve(subscriptionId, {
    expand: ["items.data"],
  });

  // Create subscription in Convex
  await convex.mutation(api.subscriptions.createSubscription, {
    userId,
    stripeCustomerId: session.customer as string,
    stripeSubscriptionId: subscriptionId,
    tier: tier as "basic" | "premium",
    currentPeriodEnd: subscription.items.data[0].current_period_end * 1000,
  });

  console.log(`Subscription created for user ${clerkUserId}, tier: ${tier}`);
}

async function handleInvoicePaid(invoice: Stripe.Invoice) {
  // This fires at the start of each billing period
  // We could reset usage here, but it's better to track by calendar month
  console.log(`Invoice paid: ${invoice.id}`);
}

async function handleSubscriptionUpdated(subscription: Stripe.Subscription) {
  const status = mapStripeStatus(subscription.status);

  await convex.mutation(api.subscriptions.updateSubscriptionStatus, {
    stripeSubscriptionId: subscription.id,
    status,
    currentPeriodEnd: subscription.items.data[0].current_period_end * 1000,
  });

  console.log(`Subscription ${subscription.id} updated to ${status}`);
}

async function handleSubscriptionDeleted(subscription: Stripe.Subscription) {
  await convex.mutation(api.subscriptions.updateSubscriptionStatus, {
    stripeSubscriptionId: subscription.id,
    status: "canceled",
  });

  console.log(`Subscription ${subscription.id} canceled`);
}

function mapStripeStatus(
  status: Stripe.Subscription.Status
): "active" | "canceled" | "past_due" {
  switch (status) {
    case "active":
    case "trialing":
      return "active";
    case "past_due":
    case "unpaid":
      return "past_due";
    default:
      return "canceled";
  }
}
