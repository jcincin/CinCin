import { NextRequest } from "next/server";
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

function verifySignature(action: string, email: string, firstName: string, sig: string): boolean {
  const secret = process.env.ADMIN_TOKEN;
  if (!secret) return false;
  const data = `${action}:${email}:${firstName}`;
  const expected = crypto.createHmac("sha256", secret).update(data).digest("hex");
  try {
    return crypto.timingSafeEqual(Buffer.from(sig, "hex"), Buffer.from(expected, "hex"));
  } catch {
    return false;
  }
}

function htmlPage(title: string, message: string, success: boolean) {
  const color = success ? "#d4af37" : "#ef4444";
  return new Response(
    `<!DOCTYPE html>
<html>
<head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>${title}</title></head>
<body style="margin:0;min-height:100vh;display:flex;align-items:center;justify-content:center;background:#0a0a0a;font-family:Georgia,serif;">
  <div style="text-align:center;padding:40px;max-width:480px;">
    <h1 style="color:${color};font-size:28px;letter-spacing:4px;margin:0 0 20px;">${title}</h1>
    <div style="width:60px;height:2px;background:${color};margin:0 auto 30px;"></div>
    <p style="color:#ccc;font-size:16px;line-height:1.6;">${message}</p>
    <p style="color:#666;font-size:12px;margin-top:40px;letter-spacing:1px;">CIN CIN &middot; RESERVATION CONCIERGE</p>
  </div>
</body>
</html>`,
    { status: 200, headers: { "Content-Type": "text/html; charset=utf-8" } }
  );
}

export async function GET(request: NextRequest) {
  const { searchParams } = new URL(request.url);
  const action = searchParams.get("action");
  const email = searchParams.get("email");
  const firstName = searchParams.get("firstName");
  const sig = searchParams.get("sig");

  if (!action || !email || !firstName || !sig) {
    return htmlPage("INVALID LINK", "This link is missing required parameters.", false);
  }

  if (action !== "approve" && action !== "deny") {
    return htmlPage("INVALID ACTION", "The requested action is not recognized.", false);
  }

  if (!verifySignature(action, email, firstName, sig)) {
    return htmlPage("UNAUTHORIZED", "This link has an invalid or expired signature.", false);
  }

  try {
    if (action === "approve") {
      const appUrl = process.env.NEXT_PUBLIC_APP_URL || "http://localhost:3000";
      const signUpUrl = `${appUrl}/sign-up?email=${encodeURIComponent(email)}`;

      const { error } = await getResend().emails.send({
        from: "Cin Cin <applications@cincin.vip>",
        to: email,
        subject: "You're In - Complete Your Cin Cin Membership",
        html: `
          <div style="font-family: Georgia, serif; max-width: 600px; margin: 0 auto; background: #0a0a0a; color: #fafafa; padding: 40px; border: 1px solid #d4af37;">
            <div style="text-align: center; margin-bottom: 30px;">
              <h1 style="color: #d4af37; font-size: 32px; letter-spacing: 6px; margin: 0;">CIN CIN</h1>
              <div style="width: 60px; height: 2px; background: #d4af37; margin: 15px auto;"></div>
            </div>

            <p style="color: #fafafa; font-size: 18px; line-height: 1.6; text-align: center; margin-bottom: 30px;">
              Dear ${firstName},
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
                CIN CIN &middot; RESERVATION CONCIERGE
              </p>
            </div>
          </div>
        `,
      });

      if (error) {
        console.error("Resend error:", error);
        return htmlPage("ERROR", "Failed to send the approval email. Please try again.", false);
      }

      try {
        await getConvex().mutation(api.applications.approveApplication, { email });
      } catch (convexError) {
        console.error("Convex error:", convexError);
      }

      return htmlPage("APPROVED", `${firstName}'s application has been approved and their invitation email has been sent.`, true);
    } else {
      try {
        await getConvex().mutation(api.applications.rejectApplication, { email });
      } catch (convexError) {
        console.error("Convex error:", convexError);
      }

      return htmlPage("DENIED", `${firstName}'s application has been denied.`, true);
    }
  } catch (error) {
    console.error("Review error:", error);
    return htmlPage("ERROR", "Something went wrong processing this request.", false);
  }
}
