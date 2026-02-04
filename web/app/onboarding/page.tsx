"use client";

import { useUser } from "@clerk/nextjs";
import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import { Button } from "@/components/ui/button";

type Tier = "basic" | "premium";

const tiers = [
  {
    id: "basic" as Tier,
    name: "Basic",
    price: 49,
    features: [
      "5 reservations per month",
      "1 Concierge booking per month",
      "Standard support",
    ],
  },
  {
    id: "premium" as Tier,
    name: "Premium",
    price: 99,
    features: [
      "15 reservations per month",
      "5 Concierge bookings per month",
      "Priority support",
      "Early access to new features",
    ],
    recommended: true,
  },
];

export default function OnboardingPage() {
  const { isLoaded, isSignedIn, user } = useUser();
  const router = useRouter();
  const [selectedTier, setSelectedTier] = useState<Tier>("premium");
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    if (isLoaded && !isSignedIn) {
      router.push("/sign-up");
    }
  }, [isLoaded, isSignedIn, router]);

  const handleCheckout = async () => {
    setIsLoading(true);
    setError("");

    try {
      const res = await fetch("/api/stripe/checkout", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ tier: selectedTier }),
      });

      const data = await res.json();

      if (!res.ok) {
        throw new Error(data.error || "Failed to create checkout session");
      }

      // Redirect to Stripe Checkout
      window.location.href = data.url;
    } catch (err) {
      setError(
        err instanceof Error ? err.message : "Failed to start checkout"
      );
      setIsLoading(false);
    }
  };

  if (!isLoaded) {
    return (
      <div className="flex items-center justify-center min-h-screen bg-black text-white">
        <div className="animate-pulse text-amber-400">Loading...</div>
      </div>
    );
  }

  if (!isSignedIn) {
    return null;
  }

  return (
    <div className="flex flex-col items-center justify-center min-h-screen bg-black text-white py-16 px-4">
      {/* Corner brackets */}
      <div className="absolute top-8 left-8 w-16 h-16 border-l-2 border-t-2 border-amber-400" />
      <div className="absolute top-8 right-8 w-16 h-16 border-r-2 border-t-2 border-amber-400" />
      <div className="absolute bottom-8 left-8 w-16 h-16 border-l-2 border-b-2 border-amber-400" />
      <div className="absolute bottom-8 right-8 w-16 h-16 border-r-2 border-b-2 border-amber-400" />

      <h1 className="font-serif text-5xl py-6 font-bold bg-linear-to-b from-gray-200 to-gray-500 bg-clip-text text-transparent tracking-wider">
        CIN CIN
      </h1>

      <p className="text-zinc-400 text-sm mb-2">
        Welcome, {user?.firstName || "Member"}
      </p>
      <p className="text-zinc-500 text-sm mb-8">
        Choose your membership tier to continue
      </p>

      {error && (
        <div className="mb-6 p-4 bg-red-500/10 border border-red-500/30 rounded-lg text-red-400 max-w-md text-center text-sm">
          {error}
        </div>
      )}

      <div className="grid md:grid-cols-2 gap-6 max-w-2xl w-full mb-8">
        {tiers.map((tier) => (
          <button
            key={tier.id}
            onClick={() => setSelectedTier(tier.id)}
            className={`relative p-6 rounded-lg border-2 transition-all text-left cursor-pointer ${
              selectedTier === tier.id
                ? "border-amber-400 bg-amber-400/5"
                : "border-zinc-800 bg-zinc-900/50 hover:border-zinc-700"
            }`}
          >
            {tier.recommended && (
              <div className="absolute -top-3 left-1/2 -translate-x-1/2 px-3 py-1 bg-amber-400 text-black text-xs font-bold tracking-wider rounded">
                RECOMMENDED
              </div>
            )}

            <div className="flex justify-between items-start mb-4">
              <div>
                <h3 className="text-xl font-semibold text-white">
                  {tier.name}
                </h3>
                <p className="text-zinc-400 text-sm">Monthly membership</p>
              </div>
              <div className="text-right">
                <span className="text-3xl font-bold text-white">
                  ${tier.price}
                </span>
                <span className="text-zinc-500 text-sm">/mo</span>
              </div>
            </div>

            <ul className="space-y-2">
              {tier.features.map((feature, i) => (
                <li key={i} className="flex items-center text-sm text-zinc-300">
                  <span className="text-amber-400 mr-2">+</span>
                  {feature}
                </li>
              ))}
            </ul>

            {selectedTier === tier.id && (
              <div className="absolute top-4 right-4 w-5 h-5 rounded-full bg-amber-400 flex items-center justify-center">
                <svg
                  className="w-3 h-3 text-black"
                  fill="none"
                  viewBox="0 0 24 24"
                  stroke="currentColor"
                >
                  <path
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    strokeWidth={3}
                    d="M5 13l4 4L19 7"
                  />
                </svg>
              </div>
            )}
          </button>
        ))}
      </div>

      <Button
        onClick={handleCheckout}
        disabled={isLoading}
        className="bg-amber-400 text-black hover:bg-amber-500 px-12 py-6 text-lg font-semibold tracking-wide cursor-pointer disabled:opacity-50"
      >
        {isLoading ? "LOADING..." : "CONTINUE TO PAYMENT"}
      </Button>

      <p className="text-zinc-600 text-xs mt-6 text-center max-w-md">
        You will be redirected to Stripe to complete your payment securely.
        Cancel anytime from your account settings.
      </p>
    </div>
  );
}
