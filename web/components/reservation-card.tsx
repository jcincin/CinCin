"use client";

import { Card, CardContent } from "@/components/ui/card";
import { Button } from "@/components/ui/button";

export interface Reservation {
  id: string;
  venue_id: number;
  venue_name: string;
  reservation_time: string;
  party_size: number;
  run_time: string;
  created_at: string;
  table_preferences: string[];
}

interface ReservationCardProps {
  reservation: Reservation;
  onView: () => void;
  onCancel: () => void;
  isCancelling: boolean;
}

export function ReservationCard({
  reservation,
  onView,
  onCancel,
  isCancelling,
}: ReservationCardProps) {
  return (
    <Card className="bg-zinc-800/50 border-zinc-700">
      <CardContent className="py-4">
        <div className="flex items-center justify-between">
          <div className="space-y-1">
            <div className="flex items-center gap-2">
              <span className="text-white font-medium">
                {reservation.venue_name}
              </span>
              <span className="text-zinc-500 text-xs">
                #{reservation.id.slice(-8)}
              </span>
            </div>
            <div className="text-zinc-400 text-sm">
              {reservation.reservation_time} &middot; {reservation.party_size}{" "}
              {reservation.party_size === 1 ? "guest" : "guests"}
            </div>
            <div className="text-zinc-500 text-xs">
              Sniper runs: {reservation.run_time}
            </div>
          </div>
          <div className="flex gap-2">
            <Button variant="outline" size="sm" onClick={onView}>
              View
            </Button>
            <Button
              variant="destructive"
              size="sm"
              onClick={onCancel}
              disabled={isCancelling}
            >
              {isCancelling ? "..." : "Cancel"}
            </Button>
          </div>
        </div>
      </CardContent>
    </Card>
  );
}
