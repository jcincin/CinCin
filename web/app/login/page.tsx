"use client";

import { SignIn } from "@clerk/nextjs";
import { useRouter } from "next/navigation";

export default function LoginPage() {
  const router = useRouter();

  return (
    <div className="flex flex-col items-center justify-center min-h-screen bg-black text-white">
      {/* Corner brackets */}
      <div className="absolute top-8 left-8 w-16 h-16 border-l-2 border-t-2 border-amber-400" />
      <div className="absolute top-8 right-8 w-16 h-16 border-r-2 border-t-2 border-amber-400" />
      <div className="absolute bottom-8 left-8 w-16 h-16 border-l-2 border-b-2 border-amber-400" />
      <div className="absolute bottom-8 right-8 w-16 h-16 border-r-2 border-b-2 border-amber-400" />

      <h1 className="font-serif text-6xl py-8 font-bold bg-linear-to-b from-gray-200 to-gray-500 bg-clip-text text-transparent tracking-wide">
        LOGIN
      </h1>

      <div className="mt-4">
        <SignIn
          appearance={{
            elements: {
              rootBox: "mx-auto",
              card: "bg-zinc-900 border border-zinc-800",
              headerTitle: "text-white",
              headerSubtitle: "text-zinc-400",
              socialButtonsBlockButton: "bg-zinc-800 border-zinc-700 text-white hover:bg-zinc-700",
              socialButtonsBlockButtonText: "text-white",
              dividerLine: "bg-zinc-700",
              dividerText: "text-zinc-500",
              formFieldLabel: "text-zinc-400",
              formFieldInput: "bg-zinc-800 border-zinc-700 text-white",
              formButtonPrimary: "bg-amber-400 text-black hover:bg-amber-500",
              footerActionLink: "text-amber-400 hover:text-amber-500",
              identityPreviewText: "text-white",
              identityPreviewEditButton: "text-amber-400",
            },
          }}
          routing="path"
          path="/login"
          signUpUrl="/apply"
          forceRedirectUrl="/home"
        />
      </div>

      <button
        onClick={() => router.push("/")}
        className="mt-8 text-zinc-500 text-sm hover:text-zinc-300 transition-colors"
      >
        &larr; Back to home
      </button>
    </div>
  );
}
