import { NextRequest, NextResponse } from "next/server";
import { Resend } from "resend";
import { ConvexHttpClient } from "convex/browser";
import { api } from "@/convex/_generated/api";

const resend = new Resend(process.env.RESEND_API_KEY);
const convex = new ConvexHttpClient(process.env.NEXT_PUBLIC_CONVEX_URL!);

interface ApproveRequest {
  email: string;
  firstName: string;
}

export async function POST(request: NextRequest) {
  try {
    // Check admin token
    const authHeader = request.headers.get("authorization");
    const adminToken = process.env.ADMIN_TOKEN;

    if (!adminToken || authHeader !== `Bearer ${adminToken}`) {
      return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
    }

    const body: ApproveRequest = await request.json();

    if (!body.email || !body.firstName) {
      return NextResponse.json(
        { error: "Missing required fields" },
        { status: 400 }
      );
    }

    const appUrl = process.env.NEXT_PUBLIC_APP_URL || "http://localhost:3000";
    const signUpUrl = `${appUrl}/sign-up?email=${encodeURIComponent(body.email)}`;

    // Send approval email to applicant
    const { error } = await resend.emails.send({
      from: "Cin Cin <applications@cincin.vip>",
      to: body.email,
      subject: "You're In - Complete Your Cin Cin Membership",
      html: `
        <div style="font-family: Georgia, serif; max-width: 600px; margin: 0 auto; background: #0a0a0a; color: #fafafa; padding: 40px; border: 1px solid #d4af37;">
          <div style="text-align: center; margin-bottom: 30px;">
            <h1 style="color: #d4af37; font-size: 32px; letter-spacing: 6px; margin: 0;">CIN CIN</h1>
            <div style="width: 60px; height: 2px; background: #d4af37; margin: 15px auto;"></div>
          </div>

          <p style="color: #fafafa; font-size: 18px; line-height: 1.6; text-align: center; margin-bottom: 30px;">
            Dear ${body.firstName},
          </p>

          <p style="color: #ccc; font-size: 16px; line-height: 1.8; text-align: center; margin-bottom: 30px;">
            Your application has been approved. Welcome to Cin Cin, your exclusive gateway to the city's most coveted tables.
          </p>

          <div style="text-align: center; margin: 40px 0;">
            <a href="${signUpUrl}"
               style="display: inline-block; background: #d4af37; color: #0a0a0a; padding: 16px 40px; text-decoration: none; font-size: 14px; letter-spacing: 2px; font-weight: bold;">
              COMPLETE YOUR MEMBERSHIP
            </a>
          </div>

          <p style="color: #666; font-size: 12px; text-align: center; margin-top: 40px;">
            This invitation is exclusively for you. Please do not share this link.
          </p>

          <div style="border-top: 1px solid #333; margin-top: 40px; padding-top: 20px; text-align: center;">
            <p style="color: #666; font-size: 11px; letter-spacing: 1px; margin: 0;">
              CIN CIN · RESERVATION CONCIERGE
            </p>
          </div>
        </div>
      `,
    });

    if (error) {
      console.error("Resend error:", error);
      return NextResponse.json(
        { error: "Failed to send approval email" },
        { status: 500 }
      );
    }

    // Update application status in Convex
    try {
      await convex.mutation(api.applications.approveApplication, {
        email: body.email,
      });
    } catch (convexError) {
      console.error("Convex error:", convexError);
      // Don't fail - email was sent successfully
    }

    return NextResponse.json({ success: true });
  } catch (error) {
    console.error("Approve API error:", error);
    return NextResponse.json(
      { error: "Failed to process approval" },
      { status: 500 }
    );
  }
}
