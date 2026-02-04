"use client";

import { SignUp } from "@clerk/nextjs";
import { useSearchParams } from "next/navigation";
import { Suspense } from "react";

function SignUpContent() {
  const searchParams = useSearchParams();
  const email = searchParams.get("email");

  return (
    <div className="flex flex-col items-center justify-center min-h-screen bg-black text-white py-16">
      {/* Corner brackets */}
      <div className="absolute top-8 left-8 w-16 h-16 border-l-2 border-t-2 border-amber-400" />
      <div className="absolute top-8 right-8 w-16 h-16 border-r-2 border-t-2 border-amber-400" />
      <div className="absolute bottom-8 left-8 w-16 h-16 border-l-2 border-b-2 border-amber-400" />
      <div className="absolute bottom-8 right-8 w-16 h-16 border-r-2 border-b-2 border-amber-400" />

      <h1 className="font-serif text-5xl py-6 font-bold bg-linear-to-b from-gray-200 to-gray-500 bg-clip-text text-transparent tracking-wider">
        CIN CIN
      </h1>

      <p className="text-zinc-400 text-sm mb-8">Complete your membership</p>

      <SignUp
        initialValues={{
          emailAddress: email || undefined,
        }}
        appearance={{
          elements: {
            rootBox: "mx-auto",
            card: "bg-zinc-900 border border-zinc-800",
            headerTitle: "text-white",
            headerSubtitle: "text-zinc-400",
            socialButtonsBlockButton:
              "bg-zinc-800 border-zinc-700 text-white hover:bg-zinc-700",
            socialButtonsBlockButtonText: "text-white",
            dividerLine: "bg-zinc-700",
            dividerText: "text-zinc-500",
            formFieldLabel: "text-zinc-300",
            formFieldInput:
              "bg-zinc-800 border-zinc-700 text-white placeholder:text-zinc-500",
            formButtonPrimary:
              "bg-amber-400 text-black hover:bg-amber-500 font-semibold",
            footerActionLink: "text-amber-400 hover:text-amber-300",
            identityPreviewText: "text-white",
            identityPreviewEditButton: "text-amber-400 hover:text-amber-300",
          },
          variables: {
            colorPrimary: "#fbbf24",
            colorText: "#fafafa",
            colorTextSecondary: "#a1a1aa",
            colorBackground: "#18181b",
            colorInputBackground: "#27272a",
            colorInputText: "#fafafa",
          },
        }}
        forceRedirectUrl="/onboarding"
      />
    </div>
  );
}

export default function SignUpPage() {
  return (
    <Suspense
      fallback={
        <div className="flex flex-col items-center justify-center min-h-screen bg-black text-white">
          <div className="animate-pulse text-amber-400">Loading...</div>
        </div>
      }
    >
      <SignUpContent />
    </Suspense>
  );
}
