import { NextRequest, NextResponse } from "next/server";
import crypto from "crypto";
import { Resend } from "resend";
import { ConvexHttpClient } from "convex/browser";
import { api } from "@/convex/_generated/api";

let _resend: Resend;
function getResend() {
  if (!_resend) _resend = new Resend(process.env.RESEND_API_KEY);
  return _resend;
}

let _convex: ConvexHttpClient;
function getConvex() {
  if (!_convex) _convex = new ConvexHttpClient(process.env.NEXT_PUBLIC_CONVEX_URL!);
  return _convex;
}

function createSignedUrl(action: string, email: string, firstName: string): string {
  const secret = process.env.ADMIN_TOKEN!;
  const data = `${action}:${email}:${firstName}`;
  const sig = crypto.createHmac("sha256", secret).update(data).digest("hex");
  const appUrl = process.env.NEXT_PUBLIC_APP_URL || "http://localhost:3000";
  const params = new URLSearchParams({ action, email, firstName, sig });
  return `${appUrl}/api/admin/review?${params.toString()}`;
}

interface ApplyRequest {
  firstName: string;
  lastName: string;
  email: string;
  referredBy?: string;
  message?: string;
}

export async function POST(request: NextRequest) {
  try {
    const body: ApplyRequest = await request.json();

    // Validate required fields
    if (!body.firstName || !body.lastName || !body.email) {
      return NextResponse.json(
        { error: "Missing required fields" },
        { status: 400 }
      );
    }

    const bossEmail = process.env.BOSS_EMAIL;
    if (!bossEmail) {
      console.error("BOSS_EMAIL environment variable not set");
      return NextResponse.json(
        { error: "Server configuration error" },
        { status: 500 }
      );
    }

    // Send email to boss
    const { error } = await getResend().emails.send({
      from: "Cin Cin <applications@cincin.vip>",
      to: bossEmail,
      subject: `New Application: ${body.firstName} ${body.lastName}`,
      html: `
        <div style="font-family: Georgia, serif; max-width: 600px; margin: 0 auto; background: #0a0a0a; color: #fafafa; padding: 40px; border: 1px solid #d4af37;">
          <div style="text-align: center; margin-bottom: 30px;">
            <h1 style="color: #d4af37; font-size: 28px; letter-spacing: 4px; margin: 0;">NEW APPLICATION</h1>
            <div style="width: 60px; height: 2px; background: #d4af37; margin: 15px auto;"></div>
          </div>

          <table style="width: 100%; border-collapse: collapse;">
            <tr>
              <td style="padding: 12px 0; border-bottom: 1px solid #333; color: #888; width: 120px;">Name</td>
              <td style="padding: 12px 0; border-bottom: 1px solid #333; color: #fafafa;">${body.firstName} ${body.lastName}</td>
            </tr>
            <tr>
              <td style="padding: 12px 0; border-bottom: 1px solid #333; color: #888;">Email</td>
              <td style="padding: 12px 0; border-bottom: 1px solid #333;">
                <a href="mailto:${body.email}" style="color: #d4af37; text-decoration: none;">${body.email}</a>
              </td>
            </tr>
            ${body.referredBy ? `
            <tr>
              <td style="padding: 12px 0; border-bottom: 1px solid #333; color: #888;">Referred By</td>
              <td style="padding: 12px 0; border-bottom: 1px solid #333; color: #fafafa;">${body.referredBy}</td>
            </tr>
            ` : ''}
            ${body.message ? `
            <tr>
              <td style="padding: 12px 0; color: #888; vertical-align: top;">Message</td>
              <td style="padding: 12px 0; color: #fafafa; white-space: pre-wrap;">${body.message}</td>
            </tr>
            ` : ''}
          </table>

          <div style="margin-top: 30px; text-align: center;">
            <a href="${createSignedUrl("approve", body.email, body.firstName)}"
               style="display: inline-block; background: #d4af37; color: #0a0a0a; padding: 14px 32px; text-decoration: none; font-size: 14px; letter-spacing: 2px; font-weight: bold; margin-right: 12px;">
              APPROVE
            </a>
            <a href="${createSignedUrl("deny", body.email, body.firstName)}"
               style="display: inline-block; background: #333; color: #fafafa; padding: 14px 32px; text-decoration: none; font-size: 14px; letter-spacing: 2px; font-weight: bold; border: 1px solid #555;">
              DENY
            </a>
          </div>

          <div style="margin-top: 30px; text-align: center;">
            <p style="color: #666; font-size: 12px; margin: 0;">
              Received ${new Date().toLocaleDateString('en-US', {
                weekday: 'long',
                year: 'numeric',
                month: 'long',
                day: 'numeric',
                hour: '2-digit',
                minute: '2-digit',
                timeZone: 'America/New_York'
              })} ET
            </p>
          </div>
        </div>
      `,
    });

    if (error) {
      console.error("Resend error:", error);
      return NextResponse.json(
        { error: "Failed to send application" },
        { status: 500 }
      );
    }

    // Store application in Convex
    try {
      await getConvex().mutation(api.applications.createApplication, {
        email: body.email,
        firstName: body.firstName,
        lastName: body.lastName,
        referredBy: body.referredBy,
        message: body.message,
      });
    } catch (convexError) {
      console.error("Convex error:", convexError);
      // Don't fail the request if Convex storage fails - email was sent successfully
    }

    return NextResponse.json({ success: true });
  } catch (error) {
    console.error("Apply API error:", error);
    return NextResponse.json(
      { error: "Failed to process application" },
      { status: 500 }
    );
  }
}
