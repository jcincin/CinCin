"use client";

import { useState } from "react";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

interface ResyLinkModalProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onLinked: () => void;
}

export function ResyLinkModal({ open, onOpenChange, onLinked }: ResyLinkModalProps) {
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState("");

  const handleLink = async (e: React.FormEvent) => {
    e.preventDefault();
    setIsLoading(true);
    setError("");

    try {
      const res = await fetch("/api/resy/link", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ email, password }),
      });

      const data = await res.json();

      if (!res.ok || data.error) {
        setError(data.error || "Failed to link Resy account");
      } else {
        onLinked();
        onOpenChange(false);
        setEmail("");
        setPassword("");
      }
    } catch {
      setError("Failed to connect to server");
    } finally {
      setIsLoading(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="bg-zinc-900 border-zinc-800 text-white sm:max-w-md">
        <DialogHeader>
          <DialogTitle className="text-amber-400">Link Resy Account</DialogTitle>
          <DialogDescription className="text-zinc-400">
            Enter your Resy credentials to link your account. Your credentials are securely stored and used only for making reservations.
          </DialogDescription>
        </DialogHeader>

        {error && (
          <div className="p-3 bg-red-500/10 border border-red-500/30 rounded-lg text-red-400 text-sm">
            {error}
          </div>
        )}

        <form onSubmit={handleLink} className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="resy-email" className="text-zinc-400">
              Resy Email
            </Label>
            <Input
              id="resy-email"
              type="email"
              placeholder="your@email.com"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              required
              className="bg-zinc-800 border-zinc-700 text-white placeholder:text-zinc-500"
            />
          </div>

          <div className="space-y-2">
            <Label htmlFor="resy-password" className="text-zinc-400">
              Resy Password
            </Label>
            <Input
              id="resy-password"
              type="password"
              placeholder="Your Resy password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              required
              className="bg-zinc-800 border-zinc-700 text-white placeholder:text-zinc-500"
            />
          </div>

          <div className="flex gap-3 pt-2">
            <Button
              type="button"
              variant="outline"
              onClick={() => onOpenChange(false)}
              className="flex-1 bg-transparent border-zinc-700 text-zinc-400 hover:bg-zinc-800 hover:text-white"
            >
              Cancel
            </Button>
            <Button
              type="submit"
              disabled={isLoading}
              className="flex-1 bg-amber-400 text-black hover:bg-amber-500 disabled:opacity-50"
            >
              {isLoading ? "Linking..." : "Link Account"}
            </Button>
          </div>
        </form>
      </DialogContent>
    </Dialog>
  );
}
