"use client";

import { useRouter } from "next/navigation";
import { z } from "zod";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { useApplyFormStore } from "@/store/apply-form";

const applySchema = z.object({
  firstName: z.string().min(1, "First name is required"),
  lastName: z.string().min(1, "Last name is required"),
  email: z.string().email("Please enter a valid email address"),
  referredBy: z.string().optional(),
  message: z.string().optional(),
});

type ApplyFormData = z.infer<typeof applySchema>;

export default function Apply() {
  const router = useRouter();
  const {
    firstName,
    lastName,
    email,
    referredBy,
    message,
    isSubmitting,
    isSubmitted,
    error,
    setFirstName,
    setLastName,
    setEmail,
    setReferredBy,
    setMessage,
    setIsSubmitting,
    setIsSubmitted,
    setError,
  } = useApplyFormStore();

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError("");

    const formData: ApplyFormData = {
      firstName,
      lastName,
      email,
      referredBy,
      message,
    };

    const result = applySchema.safeParse(formData);

    if (!result.success) {
      const firstError = result.error.issues[0];
      setError(firstError.message);
      return;
    }

    setIsSubmitting(true);

    try {
      const res = await fetch("/api/apply", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(result.data),
      });

      const data = await res.json();

      if (!res.ok) {
        throw new Error(data.error || "Failed to submit application");
      }

      setIsSubmitted(true);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to submit application. Please try again.");
    } finally {
      setIsSubmitting(false);
    }
  };

  return (
    <div className="flex flex-col items-center justify-center min-h-screen bg-black text-white py-16">
      {/* Corner brackets */}
      <div className="absolute top-8 left-8 w-16 h-16 border-l-2 border-t-2 border-amber-400" />
      <div className="absolute top-8 right-8 w-16 h-16 border-r-2 border-t-2 border-amber-400" />
      <div className="absolute bottom-8 left-8 w-16 h-16 border-l-2 border-b-2 border-amber-400" />
      <div className="absolute bottom-8 right-8 w-16 h-16 border-r-2 border-b-2 border-amber-400" />

      <h1 className="font-serif text-6xl py-8 font-bold bg-linear-to-b from-gray-200 to-gray-500 bg-clip-text text-transparent tracking-wide">
        APPLY
      </h1>

      <p className="text-zinc-400 text-sm mb-8">Request access to our reservation service</p>

      {isSubmitted ? (
        <div className="flex flex-col items-center gap-6 max-w-xs text-center">
          <div className="p-4 bg-green-500/10 border border-green-500/30 rounded-lg text-green-400">
            Application submitted successfully! We&apos;ll be in touch soon.
          </div>
        </div>
      ) : (
        <>
          {error && (
            <div className="mb-4 p-4 bg-red-500/10 border border-red-500/30 rounded-lg text-red-400 max-w-xs text-center text-sm">
              {error}
            </div>
          )}

          <form onSubmit={handleSubmit} className="flex flex-col gap-4 w-full max-w-xs">
            <div className="flex flex-col gap-2">
              <Label htmlFor="firstName" className="text-zinc-300">
                First Name
              </Label>
              <Input
                id="firstName"
                type="text"
                placeholder="John"
                value={firstName}
                onChange={(e) => setFirstName(e.target.value)}
                className="bg-zinc-900 border-zinc-700 text-white placeholder:text-zinc-500"
              />
            </div>

            <div className="flex flex-col gap-2">
              <Label htmlFor="lastName" className="text-zinc-300">
                Last Name
              </Label>
              <Input
                id="lastName"
                type="text"
                placeholder="Doe"
                value={lastName}
                onChange={(e) => setLastName(e.target.value)}
                className="bg-zinc-900 border-zinc-700 text-white placeholder:text-zinc-500"
              />
            </div>

            <div className="flex flex-col gap-2">
              <Label htmlFor="email" className="text-zinc-300">
                Email
              </Label>
              <Input
                id="email"
                type="email"
                placeholder="john@example.com"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                className="bg-zinc-900 border-zinc-700 text-white placeholder:text-zinc-500"
              />
            </div>

            <div className="flex flex-col gap-2">
              <Label htmlFor="referredBy" className="text-zinc-300">
                Referred By
              </Label>
              <Input
                id="referredBy"
                type="text"
                placeholder="Friend's name or how you heard about us"
                value={referredBy}
                onChange={(e) => setReferredBy(e.target.value)}
                className="bg-zinc-900 border-zinc-700 text-white placeholder:text-zinc-500"
              />
            </div>

            <div className="flex flex-col gap-2">
              <Label htmlFor="message" className="text-zinc-300">
                Message
              </Label>
              <Textarea
                id="message"
                placeholder="Tell us a bit about yourself and what restaurants you're interested in..."
                value={message}
                onChange={(e) => setMessage(e.target.value)}
                rows={4}
                className="bg-zinc-900 border-zinc-700 text-white placeholder:text-zinc-500"
              />
            </div>

            <Button
              type="submit"
              disabled={isSubmitting}
              className="w-full bg-amber-400 text-black mt-4 hover:bg-amber-500 cursor-pointer disabled:opacity-50"
            >
              {isSubmitting ? "SUBMITTING..." : "SUBMIT APPLICATION"}
            </Button>
          </form>
        </>
      )}

      <button
        onClick={() => router.push("/")}
        className="mt-8 text-zinc-500 text-sm hover:text-zinc-300 transition-colors"
      >
        ← Back to home
      </button>
    </div>
  );
}
